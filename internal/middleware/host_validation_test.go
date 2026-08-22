package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func nextOK() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func runReq(t *testing.T, mw func(http.Handler) http.Handler, host string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/services", nil)
	req.Host = host
	rec := httptest.NewRecorder()
	mw(nextOK()).ServeHTTP(rec, req)
	return rec.Code
}

func TestHostValidation_DefaultsAllowLocalhost(t *testing.T) {
	t.Setenv("HOMEPAGE_ALLOWED_HOSTS", "")
	mw := HostValidation(3000, zap.NewNop())

	// Localhost variants always allowed.
	assert.Equal(t, http.StatusOK, runReq(t, mw, "localhost:3000"))
	assert.Equal(t, http.StatusOK, runReq(t, mw, "127.0.0.1:3000"))
	assert.Equal(t, http.StatusOK, runReq(t, mw, "[::1]:3000"))
	assert.Equal(t, http.StatusOK, runReq(t, mw, "localhost"))
	// External host rejected by default.
	assert.Equal(t, http.StatusBadRequest, runReq(t, mw, "evil.example.com"))
}

func TestHostValidation_WildcardAllowsAll(t *testing.T) {
	t.Setenv("HOMEPAGE_ALLOWED_HOSTS", "*")
	mw := HostValidation(3000, zap.NewNop())
	assert.Equal(t, http.StatusOK, runReq(t, mw, "anything.example.com"))
	assert.Equal(t, http.StatusOK, runReq(t, mw, "evil.com:9999"))
}

func TestHostValidation_ExplicitListWithPort(t *testing.T) {
	t.Setenv("HOMEPAGE_ALLOWED_HOSTS", "dashboard.example.com:3000,dashboard.example.com")
	mw := HostValidation(3000, zap.NewNop())
	assert.Equal(t, http.StatusOK, runReq(t, mw, "dashboard.example.com:3000"))
	assert.Equal(t, http.StatusOK, runReq(t, mw, "dashboard.example.com"))
	// Defaults still apply.
	assert.Equal(t, http.StatusOK, runReq(t, mw, "localhost:3000"))
	// Other host rejected.
	assert.Equal(t, http.StatusBadRequest, runReq(t, mw, "other.example.com"))
}

func TestHostValidation_CaseInsensitive(t *testing.T) {
	t.Setenv("HOMEPAGE_ALLOWED_HOSTS", "Dashboard.Example.COM")
	mw := HostValidation(3000, zap.NewNop())
	assert.Equal(t, http.StatusOK, runReq(t, mw, "dashboard.example.com"))
	assert.Equal(t, http.StatusOK, runReq(t, mw, "DASHBOARD.EXAMPLE.COM"))
}

func TestHostValidation_LogsRejection(t *testing.T) {
	t.Setenv("HOMEPAGE_ALLOWED_HOSTS", "good.example.com")
	mw := HostValidation(3000, zap.NewNop())
	assert.Equal(t, http.StatusBadRequest, runReq(t, mw, "evil.example.com"))
}
