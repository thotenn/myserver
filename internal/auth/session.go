package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/thotenn/myserver/internal/config"
)

// Sessions are stateless: the cookie carries the email and an expiry, signed
// with HMAC-SHA256. There is no server-side store, so there is no per-user
// revocation either — rotating session.secret invalidates every cookie at
// once, and removing an address from the allowlist evicts that person on
// their next request, which covers the case that actually matters.

const (
	// sessionParts is email | expiry | nonce, joined before signing.
	sessionParts = 3
	// renewThreshold re-issues a cookie once less than half its life is left,
	// so an active user is not logged out mid-week.
	renewFraction = 2
)

var (
	// ErrNoSession means the request carried no session cookie at all.
	ErrNoSession = errors.New("no session cookie")
	// ErrBadSession means the cookie was present but unusable: malformed,
	// tampered with, or expired.
	ErrBadSession = errors.New("invalid session cookie")
)

// Session is the identity carried by a verified cookie.
type Session struct {
	Email     string
	ExpiresAt time.Time
}

// encodeSession builds the signed cookie value for an email.
func encodeSession(secret, email string, expiresAt time.Time) (string, error) {
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generating session nonce: %w", err)
	}
	payload := strings.Join([]string{
		base64.RawURLEncoding.EncodeToString([]byte(email)),
		strconv.FormatInt(expiresAt.Unix(), 10),
		base64.RawURLEncoding.EncodeToString(nonce),
	}, "|")
	return payload + "." + sign(secret, payload), nil
}

// decodeSession verifies the signature and expiry of a cookie value.
func decodeSession(secret, value string) (*Session, error) {
	dot := strings.LastIndex(value, ".")
	if dot < 0 {
		return nil, ErrBadSession
	}
	payload, sig := value[:dot], value[dot+1:]

	// Constant-time comparison: a byte-wise early exit would leak the
	// expected signature one character at a time.
	if !hmac.Equal([]byte(sig), []byte(sign(secret, payload))) {
		return nil, ErrBadSession
	}

	fields := strings.Split(payload, "|")
	if len(fields) != sessionParts {
		return nil, ErrBadSession
	}
	rawEmail, err := base64.RawURLEncoding.DecodeString(fields[0])
	if err != nil {
		return nil, ErrBadSession
	}
	exp, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return nil, ErrBadSession
	}
	expiresAt := time.Unix(exp, 0)
	if time.Now().After(expiresAt) {
		return nil, ErrBadSession
	}
	email := NormalizeEmail(string(rawEmail))
	if email == "" {
		return nil, ErrBadSession
	}
	return &Session{Email: email, ExpiresAt: expiresAt}, nil
}

func sign(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// SessionCookiePath scopes the session cookie to the dashboard the request is
// being served under. At the root that is "/", exactly as before this existed;
// under a base path it is the prefix, so a cookie minted for
// `dashboard.example.com/team` is not sent to a sibling prefix on the same host.
func SessionCookiePath(ctx context.Context) string {
	if p := config.BasePathFrom(ctx); p != "" {
		return p
	}
	return "/"
}

// IssueSession writes a fresh session cookie for the given email.
func IssueSession(ctx context.Context, w http.ResponseWriter, cfg *config.AuthConfig, email string) error {
	maxAge := cfg.SessionMaxAge()
	expiresAt := time.Now().Add(maxAge)
	value, err := encodeSession(cfg.SessionSecret(), NormalizeEmail(email), expiresAt)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:  cfg.CookieName(),
		Value: value,
		Path:  SessionCookiePath(ctx),
		// HttpOnly is not optional here: config/custom.js is arbitrary
		// operator JavaScript injected into the dashboard, so a readable
		// session cookie would be exposed by design.
		HttpOnly: true,
		Secure:   cfg.CookieSecure(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(maxAge.Seconds()),
	})
	return nil
}

// ClearSession expires the session cookie.
func ClearSession(ctx context.Context, w http.ResponseWriter, cfg *config.AuthConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.CookieName(),
		Value:    "",
		Path:     SessionCookiePath(ctx),
		HttpOnly: true,
		Secure:   cfg.CookieSecure(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// ReadSession verifies the session cookie on a request.
func ReadSession(r *http.Request, cfg *config.AuthConfig) (*Session, error) {
	if cfg == nil {
		return nil, ErrNoSession
	}
	c, err := r.Cookie(cfg.CookieName())
	if err != nil || c.Value == "" {
		return nil, ErrNoSession
	}
	return decodeSession(cfg.SessionSecret(), c.Value)
}

// MaybeRenewSession re-issues the cookie when it is past its half-life, so an
// active session slides forward instead of expiring under the user.
func MaybeRenewSession(ctx context.Context, w http.ResponseWriter, cfg *config.AuthConfig, s *Session) {
	if s == nil {
		return
	}
	if time.Until(s.ExpiresAt) > cfg.SessionMaxAge()/renewFraction {
		return
	}
	// A failure here only costs the renewal; the current cookie stays valid.
	_ = IssueSession(ctx, w, cfg, s.Email)
}
