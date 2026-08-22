package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thotenn/myserver/internal/config"
)

func testConfig() *config.AuthConfig {
	return &config.AuthConfig{
		Allowlist: config.AuthAllowlist{
			Emails:  []string{"Person@Example.com", " spaced@example.com "},
			Domains: []string{"allowed.example", "@prefixed.example"},
		},
		Google: config.GoogleAuthConfig{
			ClientID:     "client-id",
			ClientSecret: "client-secret",
			RedirectURL:  "https://dashboard.example.com/auth/google/callback",
		},
		Session: config.AuthSessionConfig{Secret: "test-secret-key", MaxAge: "1h"},
	}
}

func TestIsAllowed(t *testing.T) {
	cfg := testConfig()
	cases := []struct {
		email string
		want  bool
		why   string
	}{
		{"person@example.com", true, "listed address"},
		{"PERSON@EXAMPLE.COM", true, "matching is case-insensitive"},
		{"  person@example.com  ", true, "surrounding whitespace is ignored"},
		{"spaced@example.com", true, "whitespace in the YAML entry is ignored"},
		{"someone@allowed.example", true, "listed domain"},
		{"someone@prefixed.example", true, "domain listed with a leading @"},
		{"someone@ALLOWED.EXAMPLE", true, "domains match case-insensitively"},
		{"stranger@example.com", false, "unlisted address on a listed-looking domain"},
		{"person@other.example", false, "right local part, wrong domain"},
		{"", false, "empty address"},
		{"not-an-email", false, "no domain at all"},
		{"person@example.com.evil.test", false, "suffix must not be treated as the domain"},
		{"evil.test@allowed.example.attacker.test", false, "the domain is what follows the last @"},
	}
	for _, c := range cases {
		if got := IsAllowed(cfg, c.email); got != c.want {
			t.Errorf("IsAllowed(%q) = %v, want %v — %s", c.email, got, c.want, c.why)
		}
	}

	if IsAllowed(nil, "person@example.com") {
		t.Error("a nil config must not grant access")
	}
}

func TestSession_RoundTrip(t *testing.T) {
	cfg := testConfig()
	w := httptest.NewRecorder()
	if err := IssueSession(w, cfg, "Person@Example.com"); err != nil {
		t.Fatal(err)
	}

	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly {
		t.Error("the session cookie must be HttpOnly: custom.js is operator-supplied JavaScript on the same page")
	}
	if !cookie.Secure {
		t.Error("the session cookie must default to Secure")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Error("the session cookie should be SameSite=Lax")
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(cookie)
	session, err := ReadSession(r, cfg)
	if err != nil {
		t.Fatalf("a freshly issued cookie must verify: %v", err)
	}
	if session.Email != "person@example.com" {
		t.Errorf("email = %q, want it normalized", session.Email)
	}
}

func TestSession_RejectsTamperedCookie(t *testing.T) {
	cfg := testConfig()
	w := httptest.NewRecorder()
	if err := IssueSession(w, cfg, "person@example.com"); err != nil {
		t.Fatal(err)
	}
	original := w.Result().Cookies()[0].Value

	// Swap the email for one that is also on the allowlist, keeping the
	// original signature: this is the attack the HMAC exists to stop.
	forgedPayload := base64.RawURLEncoding.EncodeToString([]byte("someone@allowed.example")) +
		"|" + fmt.Sprint(time.Now().Add(time.Hour).Unix()) + "|" +
		base64.RawURLEncoding.EncodeToString([]byte("12345678"))
	sig := original[strings.LastIndex(original, ".")+1:]

	cases := map[string]string{
		"swapped email":     forgedPayload + "." + sig,
		"flipped signature": original[:len(original)-1] + "X",
		"no signature":      strings.Split(original, ".")[0],
		"empty":             "",
		"garbage":           "not-a-cookie",
	}
	for name, value := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.AddCookie(&http.Cookie{Name: cfg.CookieName(), Value: value})
		if _, err := ReadSession(r, cfg); err == nil {
			t.Errorf("%s: a forged cookie was accepted", name)
		}
	}
}

func TestSession_RejectsExpiredCookie(t *testing.T) {
	cfg := testConfig()
	expired, err := encodeSession(cfg.SessionSecret(), "person@example.com", time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: cfg.CookieName(), Value: expired})

	if _, err := ReadSession(r, cfg); err == nil {
		t.Error("an expired cookie must be rejected even though its signature is valid")
	}
}

func TestSession_RejectsCookieSignedWithAnotherSecret(t *testing.T) {
	cfg := testConfig()
	other := testConfig()
	other.Session.Secret = "rotated-secret"

	value, err := encodeSession(other.SessionSecret(), "person@example.com", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: cfg.CookieName(), Value: value})

	if _, err := ReadSession(r, cfg); err == nil {
		t.Error("rotating session.secret must invalidate existing cookies")
	}
}

func TestSession_SlidingRenewal(t *testing.T) {
	cfg := testConfig() // maxAge 1h

	// Fresh cookie: more than half its life left, so nothing is re-issued.
	fresh := &Session{Email: "person@example.com", ExpiresAt: time.Now().Add(50 * time.Minute)}
	w := httptest.NewRecorder()
	MaybeRenewSession(w, cfg, fresh)
	if len(w.Result().Cookies()) != 0 {
		t.Error("a fresh session should not be re-issued on every request")
	}

	// Past its half-life: renewed so an active user is not logged out.
	old := &Session{Email: "person@example.com", ExpiresAt: time.Now().Add(10 * time.Minute)}
	w = httptest.NewRecorder()
	MaybeRenewSession(w, cfg, old)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected the session to slide forward, got %d cookies", len(cookies))
	}
}

func TestClearSession(t *testing.T) {
	cfg := testConfig()
	w := httptest.NewRecorder()
	ClearSession(w, cfg)

	c := w.Result().Cookies()[0]
	if c.MaxAge >= 0 || c.Value != "" {
		t.Errorf("logout must expire the cookie, got %+v", c)
	}
}

// makeIDToken builds an unsigned JWT with the given claims. The signature
// segment is never inspected: the token's trustworthiness comes from having
// been fetched over TLS from the token endpoint (OIDC Core §3.1.3.7 item 6).
func makeIDToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func TestEmailFromIDToken(t *testing.T) {
	const clientID = "client-id"
	const nonce = "the-nonce"
	valid := map[string]any{
		"iss":            "https://accounts.google.com",
		"aud":            clientID,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"nonce":          nonce,
		"email":          "Person@Example.com",
		"email_verified": true,
	}

	email, err := emailFromIDToken(makeIDToken(t, valid), clientID, nonce)
	if err != nil {
		t.Fatalf("a well-formed token must be accepted: %v", err)
	}
	if email != "person@example.com" {
		t.Errorf("email = %q, want it normalized", email)
	}

	// email_verified also arrives as the string "true" from some Google
	// tenants; both spellings mean verified.
	stringVerified := map[string]any{}
	for k, v := range valid {
		stringVerified[k] = v
	}
	stringVerified["email_verified"] = "true"
	if _, err := emailFromIDToken(makeIDToken(t, stringVerified), clientID, nonce); err != nil {
		t.Errorf(`email_verified: "true" should be accepted: %v`, err)
	}

	rejected := map[string]map[string]any{
		"wrong issuer":         {"iss": "https://evil.example", "aud": clientID, "exp": valid["exp"], "nonce": nonce, "email": "a@b.c", "email_verified": true},
		"wrong audience":       {"iss": "accounts.google.com", "aud": "another-client", "exp": valid["exp"], "nonce": nonce, "email": "a@b.c", "email_verified": true},
		"expired":              {"iss": "accounts.google.com", "aud": clientID, "exp": time.Now().Add(-2 * time.Hour).Unix(), "nonce": nonce, "email": "a@b.c", "email_verified": true},
		"nonce mismatch":       {"iss": "accounts.google.com", "aud": clientID, "exp": valid["exp"], "nonce": "replayed", "email": "a@b.c", "email_verified": true},
		"unverified email":     {"iss": "accounts.google.com", "aud": clientID, "exp": valid["exp"], "nonce": nonce, "email": "a@b.c", "email_verified": false},
		"missing email":        {"iss": "accounts.google.com", "aud": clientID, "exp": valid["exp"], "nonce": nonce, "email_verified": true},
		"missing exp":          {"iss": "accounts.google.com", "aud": clientID, "nonce": nonce, "email": "a@b.c", "email_verified": true},
		"missing email_verifd": {"iss": "accounts.google.com", "aud": clientID, "exp": valid["exp"], "nonce": nonce, "email": "a@b.c"},
	}
	for name, claims := range rejected {
		if _, err := emailFromIDToken(makeIDToken(t, claims), clientID, nonce); err == nil {
			t.Errorf("%s: token was accepted but should not have been", name)
		}
	}

	// An empty nonce on our side must never match, or a token replayed
	// without one would sail through.
	if _, err := emailFromIDToken(makeIDToken(t, valid), clientID, ""); err == nil {
		t.Error("an empty expected nonce must not validate")
	}

	for name, token := range map[string]string{
		"not a jwt":     "abc",
		"two segments":  "header.payload",
		"bad base64":    "header.!!!.signature",
		"bad json":      "header." + base64.RawURLEncoding.EncodeToString([]byte("{nope")) + ".sig",
		"empty payload": "header..signature",
	} {
		if _, err := emailFromIDToken(token, clientID, nonce); err == nil {
			t.Errorf("%s: malformed token was accepted", name)
		}
	}
}

func TestAuthorizationURL(t *testing.T) {
	cfg := testConfig()
	cfg.Google.HostedDomain = "example.com"

	got := AuthorizationURL(cfg, "the-state", "the-nonce")

	for _, want := range []string{
		"https://accounts.google.com/o/oauth2/v2/auth?",
		"client_id=client-id",
		"response_type=code",
		"scope=openid+email",
		"state=the-state",
		"nonce=the-nonce",
		"prompt=select_account",
		"hd=example.com",
		"redirect_uri=https%3A%2F%2Fdashboard.example.com%2Fauth%2Fgoogle%2Fcallback",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("authorization URL is missing %q\ngot: %s", want, got)
		}
	}
	if strings.Contains(got, "profile") {
		t.Error("only openid and email are requested; profile would ask for data the allowlist does not need")
	}
	if strings.Contains(got, "client-secret") {
		t.Error("the client secret must never appear in a URL the browser sees")
	}
}

func TestEmailFromTrustedHeader(t *testing.T) {
	cfg := testConfig()
	cfg.Provider = config.ProviderTrustedHeader
	cfg.TrustedHeader.Header = "Cf-Access-Authenticated-User-Email"

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Cf-Access-Authenticated-User-Email", "Person@Example.com")

	// The header is only an identity when a trusted proxy put it there.
	if _, err := EmailFromTrustedHeader(r, cfg, false); err == nil {
		t.Fatal("a header from an untrusted peer must be ignored — anyone can set one")
	}

	email, err := EmailFromTrustedHeader(r, cfg, true)
	if err != nil {
		t.Fatalf("a header from a trusted proxy should be read: %v", err)
	}
	if email != "person@example.com" {
		t.Errorf("email = %q, want it normalized", email)
	}

	// Trusted peer, but the proxy asserted nothing.
	bare := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := EmailFromTrustedHeader(bare, cfg, true); err == nil {
		t.Error("a missing header is not an identity")
	}
}
