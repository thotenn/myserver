package proxy

import (
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/thotenn/myserver/internal/config"
)

// ssrfCache caches DNS lookup results for SSRF checks to avoid repeated
// resolver round-trips on every proxied request.
type ssrfCacheEntry struct {
	err       error
	expiresAt time.Time
}

var (
	ssrfCache    = make(map[string]ssrfCacheEntry)
	ssrfCacheMu  sync.RWMutex
	ssrfCacheTTL = 5 * time.Minute
	// testSkipSSRF is set by tests to bypass SSRF checks when using
	// httptest.Server (which always binds to loopback).
	testSkipSSRF bool
)

// SetTestSkipSSRF allows tests to bypass SSRF checks. NEVER use in production.
func SetTestSkipSSRF(v bool) {
	testSkipSSRF = v
}

// cachedSSRFCheck returns a cached SSRF result for hostname if available.
func cachedSSRFCheck(hostname string) (error, bool) {
	ssrfCacheMu.RLock()
	defer ssrfCacheMu.RUnlock()
	entry, ok := ssrfCache[hostname]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.err, true
}

// setSSRFCache stores the SSRF result for hostname.
func setSSRFCache(hostname string, err error) {
	ssrfCacheMu.Lock()
	defer ssrfCacheMu.Unlock()
	ssrfCache[hostname] = ssrfCacheEntry{err: err, expiresAt: time.Now().Add(ssrfCacheTTL)}
}

// MaxResponseBodyBytes caps the response body we read from upstream so a
// pathological backend can't OOM the dashboard.
const MaxResponseBodyBytes int64 = 10 * 1024 * 1024 // 10 MiB

// ErrSSRFBlocked is returned when an upstream URL resolves to a private,
// loopback, link-local, or metadata IP and has not been explicitly opted in.
var ErrSSRFBlocked = errors.New("upstream host blocked by SSRF policy")

// ErrInvalidURL is returned when the rawURL is unparseable or uses a
// non-http(s) scheme.
var ErrInvalidURL = errors.New("invalid upstream URL")

// Params contains the parameters for an HTTP proxy request.
type Params struct {
	Method          string
	Headers         map[string]string
	Body            io.Reader
	CookieJar       http.CookieJar
	FollowRedirects bool
	IgnoreTLS       bool
	// Doer overrides the default HTTP client. Used by tests.
	Doer Doer
}

// Result contains the response from a proxied HTTP request.
type Result struct {
	Status          int
	ContentType     string
	Body            []byte
	ResponseHeaders http.Header
}

// transports is a small pool of *http.Transport keyed by the (insecure,
// disableIPv6) tuple. Sharing transports across requests enables connection
// pooling, keep-alive, and DNS caching at the OS level. Without this every
// request paid TLS+TCP+DNS overhead.
var (
	transportsMu sync.Mutex
	transports   = map[bool]*http.Transport{}
)

func sharedTransport(insecure bool) *http.Transport {
	transportsMu.Lock()
	defer transportsMu.Unlock()
	if t, ok := transports[insecure]; ok {
		return t
	}
	t := &http.Transport{
		DialContext:           dialContextFunc(),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecure,
		},
	}
	transports[insecure] = t
	return t
}

// Doer is the HTTP client boundary used by Proxy. The default
// implementation is DefaultDoer; tests may supply a stub.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// DefaultDoer implements Doer with a real http.Client configured from Params.
type DefaultDoer struct {
	Client *http.Client
}

func (d *DefaultDoer) Do(req *http.Request) (*http.Response, error) {
	return d.Client.Do(req)
}

// Proxy performs an HTTP request and returns the result.
// It validates the URL scheme, blocks SSRF targets, decompresses gzip/zlib
// bodies, sanitizes errors so credentials never leak to the caller, and
// caps the body read at MaxResponseBodyBytes.
//
// Special scheme support:
//   - file://  reads a local file directly.  The path is resolved relative
//     to the config directory when it does not start with "/".
func Proxy(ctx context.Context, rawURL string, params *Params) (*Result, error) {
	if params == nil {
		params = &Params{}
	}
	if params.Method == "" {
		params.Method = http.MethodGet
	}

	// file:// scheme — read local JSON data directly without HTTP round-trip.
	if strings.HasPrefix(rawURL, "file://") {
		path := strings.TrimPrefix(rawURL, "file://")
		if !strings.HasPrefix(path, "/") {
			path = filepath.Join(config.ConfigDir(), path)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading file %s: %w", path, err)
		}
		return &Result{
			Status:      http.StatusOK,
			ContentType: "application/json",
			Body:        body,
			ResponseHeaders: http.Header{
				"Content-Type": []string{"application/json"},
			},
		}, nil
	}

	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("%w: scheme %q not allowed", ErrInvalidURL, parsedURL.Scheme)
	}
	if parsedURL.Host == "" {
		return nil, fmt.Errorf("%w: missing host", ErrInvalidURL)
	}

	req, err := http.NewRequestWithContext(ctx, params.Method, rawURL, params.Body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	for k, v := range params.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Homepage/0.1")
	}

	// SSRF check on the initial URL.
	if err := checkSSRF(parsedURL); err != nil {
		return nil, err
	}

	doer := params.Doer
	if doer == nil {
		transport := sharedTransport(params.IgnoreTLS)
		client := &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		}
		// Default to following redirects (with bounded count and SSRF re-check
		// on each hop). The original Node.js Homepage follows redirects.
		maxRedirects := 10
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if !params.FollowRedirects {
				return http.ErrUseLastResponse
			}
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			if err := checkSSRF(req.URL); err != nil {
				return err
			}
			return nil
		}
		if params.CookieJar != nil {
			client.Jar = params.CookieJar
		}
		doer = &DefaultDoer{Client: client}
	}

	resp, err := doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %s", scrubError(err))
	}
	defer resp.Body.Close()

	// Decompress body if needed. If decompression init fails we abort with
	// a clean error rather than returning the raw bytes (which would be
	// undecodable garbage to the caller).
	var reader io.Reader = resp.Body
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		gr, gerr := gzip.NewReader(resp.Body)
		if gerr != nil {
			return nil, fmt.Errorf("decompressing gzip: %v", gerr)
		}
		defer gr.Close()
		reader = gr
	case "deflate":
		zr, zerr := zlib.NewReader(resp.Body)
		if zerr != nil {
			return nil, fmt.Errorf("decompressing deflate: %v", zerr)
		}
		defer zr.Close()
		reader = zr
	}

	body, err := io.ReadAll(io.LimitReader(reader, MaxResponseBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading response body: %v", err)
	}
	if int64(len(body)) > MaxResponseBodyBytes {
		body = body[:MaxResponseBodyBytes]
	}

	return &Result{
		Status:          resp.StatusCode,
		ContentType:     resp.Header.Get("Content-Type"),
		Body:            body,
		ResponseHeaders: resp.Header,
	}, nil
}

// scrubError returns a string version of err with any URL inside scrubbed
// of credentials. It handles *url.Error explicitly because Go's standard
// library wraps the full URL (including query string) into Error.URL.
func scrubError(err error) string {
	if err == nil {
		return ""
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Sprintf("%s %s: %s", ue.Op, SanitizeURL(ue.URL), ue.Err)
	}
	return SanitizeURL(err.Error())
}

// checkSSRF rejects URLs whose host resolves to a private/loopback/
// link-local/multicast/cloud-metadata IP. Without this any widget URL,
// even one provided via Docker labels, could be abused to reach the
// internal network.
func checkSSRF(u *url.URL) error {
	if testSkipSSRF {
		return nil
	}
	host := u.Hostname()
	if host == "" {
		return ErrSSRFBlocked
	}
	if err, ok := cachedSSRFCheck(host); ok {
		return err
	}
	// Resolve all addresses for the host. We use the Go resolver here so
	// the lookup honours /etc/hosts (PaaS gateways often resolve as private
	// names but their IPs are still in the docker network).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		// If we can't resolve we let the dialer fail with a network error
		// instead of blocking — that surfaces a clearer message to ops.
		setSSRFCache(host, nil)
		return nil
	}
	for _, ip := range ips {
		if isBlockedIP(ip.IP) {
			setSSRFCache(host, ErrSSRFBlocked)
			return ErrSSRFBlocked
		}
	}
	setSSRFCache(host, nil)
	return nil
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip.IsLoopback() {
		return !config.AllowPrivateHosts()
	}
	// Cloud metadata IPs (AWS/GCP/Azure)
	if ip.Equal(net.IPv4(169, 254, 169, 254)) || ip.Equal(net.IPv4(100, 100, 100, 200)) {
		return true
	}
	if ip.IsPrivate() {
		// RFC1918 (v4) and fc00::/7 (v6) are blocked. Container networks live
		// here too, so users who need to proxy to internal services should
		// rely on docker dns name. The dialer will still try them via the
		// dialer configured below — but we block here.
		// NOTE: this is opinionated. If you need to proxy to a private
		// host (e.g. Plex on the LAN), set HOMEPAGE_ALLOW_PRIVATE_HOSTS=true.
		return !config.AllowPrivateHosts()
	}
	return false
}

// dialContextFunc returns a custom dial function that respects IPv6 disable setting.
func dialContextFunc() func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	if config.ProxyDisableIPv6() {
		return func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp4", addr)
		}
	}

	return dialer.DialContext
}

// NewCookieJar creates a new cookie jar for session management.
func NewCookieJar() (http.CookieJar, error) {
	return cookiejar.New(nil)
}

// sensitiveURLParams is a substring list of query parameter names that
// should be redacted in any URL we log or return to clients.
var sensitiveURLParams = []string{
	"apikey", "api_key", "token", "access_token", "auth", "key",
	"password", "passwd", "pass", "secret", "client_secret", "appkey",
	"account", "user", "username", "credential",
}

// SanitizeURL removes sensitive query parameters from a URL string.
// Matching is case-insensitive and substring-based.
func SanitizeURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if parsed.User != nil {
		parsed.User = url.UserPassword("[REDACTED]", "[REDACTED]")
	}
	q := parsed.Query()
	changed := false
	for k := range q {
		lower := strings.ToLower(k)
		for _, needle := range sensitiveURLParams {
			if strings.Contains(lower, needle) {
				q.Set(k, "[REDACTED]")
				changed = true
				break
			}
		}
	}
	if changed || parsed.User != nil {
		parsed.RawQuery = q.Encode()
		return parsed.String()
	}
	return rawURL
}

// FormatAPICall replaces {key} placeholders in a URL template with values from args.
func FormatAPICall(template string, args map[string]string) string {
	result := template
	for k, v := range args {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
	}
	return result
}

// IsJSON checks if a content type is JSON.
func IsJSON(contentType string) bool {
	return strings.Contains(contentType, "application/json")
}
