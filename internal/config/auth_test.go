package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// writeAuthFile drops an auth.yaml into a fresh config dir and reloads.
func writeAuthFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	SetConfigDir(dir)
	t.Cleanup(ResetConfigDir)
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, AuthFile), []byte(content), 0o600); err != nil {
			t.Fatalf("writing %s: %v", AuthFile, err)
		}
	}
	return dir
}

// resetAuthState clears the published policy between cases so that one test's
// last-known-good does not leak into the next.
func resetAuthState(t *testing.T) {
	t.Helper()
	authState.Store(AuthState{})
	t.Cleanup(func() { authState.Store(AuthState{}) })
}

const validAuth = `
allowlist:
  emails:
    - person@example.com
google:
  clientId: "id-123"
  clientSecret: "secret-456"
  redirectURL: "https://dashboard.example.com/auth/google/callback"
`

func TestAuth_MissingFileIsPublic(t *testing.T) {
	resetAuthState(t)
	writeAuthFile(t, "")
	ReloadAuth()

	state := Auth()
	if state.Required || state.Lockdown {
		t.Fatalf("no auth.yaml must leave the dashboard public, got %+v", state)
	}
}

func TestAuth_EmptyAllowlistIsPublic(t *testing.T) {
	resetAuthState(t)
	writeAuthFile(t, "allowlist:\n  emails: []\n")
	ReloadAuth()

	if state := Auth(); state.Required || state.Lockdown {
		t.Fatalf("an empty allowlist is an explicit request for a public dashboard, got %+v", state)
	}
}

func TestAuth_NonEmptyAllowlistRequiresLogin(t *testing.T) {
	resetAuthState(t)
	writeAuthFile(t, validAuth)
	ReloadAuth()

	state := Auth()
	if !state.Required {
		t.Fatalf("an allowlist with an email must require login, got %+v", state)
	}
	if state.Lockdown || state.Err != nil {
		t.Fatalf("a valid config must not degrade: %+v", state)
	}
}

// The core safety property: a typo must never publish the dashboard.
func TestAuth_BrokenYAMLKeepsLastKnownGood(t *testing.T) {
	resetAuthState(t)
	dir := writeAuthFile(t, validAuth)
	ReloadAuth()
	if !Auth().Required {
		t.Fatal("precondition: auth should be required")
	}

	// The operator saves a broken edit.
	if err := os.WriteFile(filepath.Join(dir, AuthFile),
		[]byte("allowlist:\n  emails:\n  - person@example.com\n   bad indent: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	ReloadAuth()

	state := Auth()
	if !state.Required {
		t.Fatal("a broken auth.yaml opened the dashboard — this is the bug this feature exists to prevent")
	}
	if !state.Degraded || state.Err == nil {
		t.Errorf("a retained policy must be reported as degraded, got %+v", state)
	}
	if state.Config == nil || len(state.Config.Allowlist.Emails) != 1 {
		t.Errorf("the last known good allowlist should have been kept, got %+v", state.Config)
	}
}

func TestAuth_DisappearingFileLocksDown(t *testing.T) {
	resetAuthState(t)
	dir := writeAuthFile(t, validAuth)
	ReloadAuth()
	if !Auth().Required {
		t.Fatal("precondition: auth should be required")
	}

	// The bind mount dies, or someone deletes the file.
	if err := os.Remove(filepath.Join(dir, AuthFile)); err != nil {
		t.Fatal(err)
	}
	ReloadAuth()

	state := Auth()
	if !state.Lockdown {
		t.Fatal("a vanished auth.yaml must lock down, never fall back to public")
	}
	if state.Required != true {
		t.Error("the policy should still be considered required during lockdown")
	}
}

func TestAuth_InvalidConfigWithoutPriorPolicyLocksDown(t *testing.T) {
	resetAuthState(t)
	writeAuthFile(t, "allowlist:\n  emails:\n  - a@example.com\n  bad: [")
	ReloadAuth()

	if state := Auth(); !state.Lockdown {
		t.Fatalf("an unreadable file with no fallback must lock down, got %+v", state)
	}
}

func TestAuth_ExplicitEmptyAllowlistTurnsAuthOff(t *testing.T) {
	resetAuthState(t)
	dir := writeAuthFile(t, validAuth)
	ReloadAuth()
	if !Auth().Required {
		t.Fatal("precondition: auth should be required")
	}

	// A well-formed file with nobody listed is an unambiguous order.
	if err := os.WriteFile(filepath.Join(dir, AuthFile), []byte("allowlist:\n  emails: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ReloadAuth()

	if state := Auth(); state.Required || state.Lockdown {
		t.Fatalf("clearing the allowlist should reopen the dashboard, got %+v", state)
	}
}

func TestAuth_UnresolvedPlaceholderIsRejected(t *testing.T) {
	resetAuthState(t)
	writeAuthFile(t, `
allowlist:
  emails: [person@example.com]
google:
  clientId: "{{HOMEPAGE_VAR_GOOGLE_CLIENT_ID}}"
  clientSecret: "secret"
  redirectURL: "https://dashboard.example.com/auth/google/callback"
`)
	ReloadAuth()

	state := Auth()
	if !state.Lockdown {
		t.Fatal("an unset environment variable must not start a half-configured login")
	}
	if state.Err == nil || !strings.Contains(state.Err.Error(), "clientId") {
		t.Errorf("the error should name the offending field, got %v", state.Err)
	}
}

func TestAuth_MissingGoogleCredentialsRejected(t *testing.T) {
	resetAuthState(t)
	writeAuthFile(t, "allowlist:\n  emails: [person@example.com]\n")
	ReloadAuth()

	if state := Auth(); !state.Lockdown || state.Err == nil {
		t.Fatalf("an allowlist without google credentials cannot work, got %+v", state)
	}
}

func TestValidateAuthConfig_PublicDomainGuard(t *testing.T) {
	base := func(domain string, escape bool) *AuthConfig {
		return &AuthConfig{
			Allowlist:          AuthAllowlist{Domains: []string{domain}},
			AllowPublicDomains: escape,
			Google: GoogleAuthConfig{
				ClientID:     "id",
				ClientSecret: "secret",
				RedirectURL:  "https://dashboard.example.com/auth/google/callback",
			},
		}
	}

	if err := ValidateAuthConfig(base("gmail.com", false)); err == nil {
		t.Error("gmail.com as an allowed domain lets in anyone with a Google account")
	}
	if err := ValidateAuthConfig(base("GMAIL.COM", false)); err == nil {
		t.Error("the guard must be case-insensitive")
	}
	if err := ValidateAuthConfig(base("@gmail.com", false)); err == nil {
		t.Error("the guard must see through a leading @")
	}
	if err := ValidateAuthConfig(base("gmail.com", true)); err != nil {
		t.Errorf("allowPublicDomains is the documented escape hatch: %v", err)
	}
	if err := ValidateAuthConfig(base("example.com", false)); err != nil {
		t.Errorf("a private domain must be accepted: %v", err)
	}
}

func TestValidateAuthConfig_RejectsBadValues(t *testing.T) {
	valid := GoogleAuthConfig{
		ClientID:     "id",
		ClientSecret: "secret",
		RedirectURL:  "https://dashboard.example.com/auth/google/callback",
	}
	cases := map[string]*AuthConfig{
		"relative redirect": {
			Allowlist: AuthAllowlist{Emails: []string{"a@example.com"}},
			Google:    GoogleAuthConfig{ClientID: "id", ClientSecret: "s", RedirectURL: "/auth/google/callback"},
		},
		"entry without @": {
			Allowlist: AuthAllowlist{Emails: []string{"not-an-email"}},
			Google:    valid,
		},
		"unknown provider": {
			Provider:  "github",
			Allowlist: AuthAllowlist{Emails: []string{"a@example.com"}},
			Google:    valid,
		},
		"trustedHeader without header": {
			Provider:  ProviderTrustedHeader,
			Allowlist: AuthAllowlist{Emails: []string{"a@example.com"}},
		},
		"bad maxAge": {
			Allowlist: AuthAllowlist{Emails: []string{"a@example.com"}},
			Google:    valid,
			Session:   AuthSessionConfig{MaxAge: "seven days"},
		},
		"relative publicPath": {
			Allowlist:   AuthAllowlist{Emails: []string{"a@example.com"}},
			Google:      valid,
			PublicPaths: []string{"api/config/custom.css"},
		},
	}
	for name, cfg := range cases {
		if err := ValidateAuthConfig(cfg); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

func TestAuth_SessionSecretIsStableAcrossReloads(t *testing.T) {
	resetAuthState(t)
	writeAuthFile(t, validAuth)
	ReloadAuth()

	first := Auth().Config.SessionSecret()
	ReloadAuth()
	second := Auth().Config.SessionSecret()

	if first != second {
		t.Error("regenerating the session key on reload would sign everybody out whenever any YAML changes")
	}
	if len(first) < 32 {
		t.Errorf("generated secret is too short: %d chars", len(first))
	}
}

func TestAuth_ConfigDefaults(t *testing.T) {
	var cfg *AuthConfig
	if got := cfg.CookieName(); got != defaultCookieName {
		t.Errorf("nil config cookie name = %q", got)
	}
	if !cfg.CookieSecure() {
		t.Error("cookies must default to Secure")
	}
	if cfg.ProviderName() != ProviderGoogle {
		t.Error("provider must default to google")
	}
	if cfg.SessionMaxAge() != defaultSessionMaxAge {
		t.Error("unexpected default session lifetime")
	}
}

func TestAuth_UninitialisedStateIsClosed(t *testing.T) {
	// Before the first load nothing is known, and "nothing is known" must not
	// be read as "there is no auth".
	authState = atomic.Value{}
	t.Cleanup(func() { authState.Store(AuthState{}) })
	if state := Auth(); !state.Lockdown || state.Required {
		t.Fatalf("an unloaded policy must not report a public dashboard, got %+v", state)
	}
}

func TestAuthFileIsHashed(t *testing.T) {
	resetAuthState(t)
	dir := writeAuthFile(t, validAuth)

	before, err := configHash()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, AuthFile),
		[]byte(validAuth+"\n  # touched\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := configHash()
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Error("editing auth.yaml must change the config hash so clients reload")
	}
}

// The gate reads the policy on every request while the watcher rewrites it,
// so the two must not race. Run with -race.
func TestAuth_ConcurrentReadsDuringReload(t *testing.T) {
	resetAuthState(t)
	dir := writeAuthFile(t, validAuth)
	ReloadAuth()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Readers, standing in for concurrent requests.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					state := Auth()
					// A published policy must always be self-consistent: any
					// state that demands a login has a config to enforce.
					if state.Required && state.Config == nil {
						t.Error("published a policy that requires login but carries no config")
						return
					}
				}
			}
		}()
	}

	// The watcher, rewriting the file underneath them.
	for i := 0; i < 20; i++ {
		body := validAuth
		if i%3 == 0 {
			body = "allowlist: [" // a broken save
		}
		if err := os.WriteFile(filepath.Join(dir, AuthFile), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		ReloadAuth()
	}

	close(stop)
	wg.Wait()

	// Every broken save must have been survived without opening up.
	if state := Auth(); !state.Required {
		t.Fatal("after a series of reloads including broken ones, auth must still be required")
	}
}
