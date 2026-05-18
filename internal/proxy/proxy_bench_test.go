package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func BenchmarkSanitizeURL(b *testing.B) {
	url := "https://example.com/api?apiKey=secret123&token=abc&user=admin"
	for i := 0; i < b.N; i++ {
		_ = SanitizeURL(url)
	}
}

func BenchmarkProxyWithMockUpstream(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer ts.Close()

	ctx := context.Background()
	SetTestSkipSSRF(true)
	defer SetTestSkipSSRF(false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Proxy(ctx, ts.URL, &Params{Doer: &DefaultDoer{Client: ts.Client()}})
	}
}

func BenchmarkCachedSSRFCheck(b *testing.B) {
	// Prime cache
	_ = checkSSRF(mustParse("https://example.com"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cachedSSRFCheck("example.com")
	}
}

func mustParse(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}
