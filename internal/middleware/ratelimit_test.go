package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"
)

func TestRateLimiter_AllowsRequestsUnderLimit(t *testing.T) {
	handler := RateLimiter(func(ip string) *rate.Limiter {
		return NewRateLimiter(10, 10)
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	handler := RateLimiter(func(ip string) *rate.Limiter {
		return NewRateLimiter(1, 1)
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request should pass
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected first request 200, got %d", rec1.Code)
	}

	// Second request immediately should be rate limited
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec2.Code)
	}
}

func TestNewTrustedProxyChecker_Default(t *testing.T) {
	c := NewTrustedProxyChecker("")
	if !c.IsTrusted("127.0.0.1") {
		t.Error("expected 127.0.0.1 to be trusted by default")
	}
	if !c.IsTrusted("::1") {
		t.Error("expected ::1 to be trusted by default")
	}
	if c.IsTrusted("192.168.1.1") {
		t.Error("expected 192.168.1.1 to be untrusted by default")
	}
}

func TestNewTrustedProxyChecker_Custom(t *testing.T) {
	c := NewTrustedProxyChecker("192.168.0.0/16")
	if !c.IsTrusted("192.168.1.1") {
		t.Error("expected 192.168.1.1 to be trusted")
	}
	if c.IsTrusted("10.0.0.1") {
		t.Error("expected 10.0.0.1 to be untrusted")
	}
}

func TestTrustedProxyChecker_IsTrusted_Empty(t *testing.T) {
	c := NewTrustedProxyChecker("")
	if c.IsTrusted("") {
		t.Error("expected empty IP to be untrusted")
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	headers := rec.Header()
	if headers.Get("X-Frame-Options") != "DENY" {
		t.Errorf("unexpected X-Frame-Options: %s", headers.Get("X-Frame-Options"))
	}
	if headers.Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
		t.Errorf("unexpected Referrer-Policy: %s", headers.Get("Referrer-Policy"))
	}
	if headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("unexpected X-Content-Type-Options: %s", headers.Get("X-Content-Type-Options"))
	}
	if headers.Get("Content-Security-Policy") == "" {
		t.Error("expected Content-Security-Policy header")
	}
	if headers.Get("Strict-Transport-Security") != "" {
		t.Error("expected no HSTS header when HOMEPAGE_HSTS is not set")
	}
}

func TestSecurityHeaders_HSTS(t *testing.T) {
	t.Setenv("HOMEPAGE_HSTS", "true")
	// Re-create the handler so it picks up the env var at execution time
	// (SecurityHeaders reads the env var on every request).
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Error("expected HSTS header when HOMEPAGE_HSTS=true")
	}
}

func TestRateLimiter_RetryAfter(t *testing.T) {
	handler := RateLimiter(func(ip string) *rate.Limiter {
		return NewRateLimiter(1, 1)
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust the bucket
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec2.Code)
	}
	retry := rec2.Header().Get("Retry-After")
	if retry == "" {
		t.Error("expected Retry-After header on 429")
	}
}

func TestRateLimiter_DistinctIPs(t *testing.T) {
	handler := RateLimiter(func(ip string) *rate.Limiter {
		return NewRateLimiter(1, 1)
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Two different IPs should each have their own limiter.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "1.2.3.4:1234"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "5.6.7.8:5678"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec1.Code != http.StatusOK || rec2.Code != http.StatusOK {
		t.Fatalf("expected both requests to pass, got %d and %d", rec1.Code, rec2.Code)
	}
}
