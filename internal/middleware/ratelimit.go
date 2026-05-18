package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter creates a middleware that limits requests per IP using a token
// bucket. limiterForIP is called on first sight of an IP and should return a
// rate.Limiter configured for that endpoint's burst and rate.
func RateLimiter(limiterForIP func(ip string) *rate.Limiter) func(http.Handler) http.Handler {
	var (
		mu       sync.Mutex
		limiters = make(map[string]*rate.Limiter)
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			mu.Lock()
			lim, ok := limiters[ip]
			if !ok {
				lim = limiterForIP(ip)
				limiters[ip] = lim
			}
			mu.Unlock()

			if !lim.Allow() {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", strconv.Itoa(int(lim.Reserve().DelayFrom(time.Now()).Seconds())))
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"errors":["rate limit exceeded"]}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP returns the client IP string from the request.
func clientIP(r *http.Request) string {
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx >= 0 {
		ip = ip[:idx]
		ip = strings.TrimPrefix(strings.TrimSuffix(ip, "]"), "[")
	}
	return ip
}

// NewRateLimiter creates a standard rate.Limiter with the given rps and burst.
func NewRateLimiter(rps float64, burst int) *rate.Limiter {
	return rate.NewLimiter(rate.Limit(rps), burst)
}

// TrustedProxyChecker parses a comma-separated list of CIDRs and can tell if
// an IP is in the trusted set.
type TrustedProxyChecker struct {
	nets []*net.IPNet
}

// NewTrustedProxyChecker creates a checker from a comma-separated CIDR list.
// If the list is empty it defaults to 127.0.0.1/8 and ::1/128.
func NewTrustedProxyChecker(list string) *TrustedProxyChecker {
	if list == "" {
		list = "127.0.0.1/8,::1/128"
	}
	c := &TrustedProxyChecker{}
	for _, s := range strings.Split(list, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			continue
		}
		c.nets = append(c.nets, n)
	}
	return c
}

// IsTrusted reports whether the given IP address is in the trusted proxy list.
func (c *TrustedProxyChecker) IsTrusted(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range c.nets {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}
