package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

// AuthFile is the name of the optional authentication config file.
// It is an extension of MyServer: Homepage never reads it, so its presence
// does not break YAML compatibility with the original project.
const AuthFile = "auth.yaml"

// Authentication is opt-in and driven by the allowlist itself: an auth.yaml
// holding at least one email or domain makes login mandatory, an absent file
// (or one with an empty allowlist) leaves the dashboard public exactly as it
// was before this feature existed.
//
// The delicate part is that "empty allowlist" and "config we failed to read"
// look identical from the outside, and treating the second as the first would
// silently publish the dashboard. Every failure path below therefore keeps the
// last known good policy or locks the dashboard down; none of them degrades to
// public. See reconcileAuth for the full state table.

// AuthConfig is the parsed content of auth.yaml.
type AuthConfig struct {
	// Provider selects the identity source: "google" (default) or
	// "trustedHeader" for deployments already sitting behind an SSO proxy.
	Provider string `yaml:"provider,omitempty"`

	Google        GoogleAuthConfig        `yaml:"google,omitempty"`
	TrustedHeader TrustedHeaderAuthConfig `yaml:"trustedHeader,omitempty"`

	Allowlist AuthAllowlist `yaml:"allowlist,omitempty"`

	// AllowPublicDomains is the explicit escape hatch for listing a public
	// mail provider under allowlist.domains, which would otherwise open the
	// dashboard to anyone able to register an address there.
	AllowPublicDomains bool `yaml:"allowPublicDomains,omitempty"`

	Session AuthSessionConfig `yaml:"session,omitempty"`

	// PublicPaths are extra paths served without a session, on top of the
	// built-in ones (/static/*, /auth/*, /api/healthcheck).
	PublicPaths []string `yaml:"publicPaths,omitempty"`
}

// GoogleAuthConfig holds the Google OAuth client credentials.
type GoogleAuthConfig struct {
	ClientID     string `yaml:"clientId,omitempty"`
	ClientSecret string `yaml:"clientSecret,omitempty"`
	// RedirectURL is mandatory and explicit: deriving it from the Host header
	// would be host injection, since HostValidation does not cover "/".
	RedirectURL string `yaml:"redirectURL,omitempty"`
	// HostedDomain is an optional hint (hd=) for Google Workspace tenants.
	// It is a UX hint only — the allowlist is what actually grants access.
	HostedDomain string `yaml:"hostedDomain,omitempty"`
}

// TrustedHeaderAuthConfig configures the trustedHeader provider.
type TrustedHeaderAuthConfig struct {
	Header string `yaml:"header,omitempty"`
}

// AuthAllowlist is the set of identities allowed into the dashboard.
type AuthAllowlist struct {
	Emails  []string `yaml:"emails,omitempty"`
	Domains []string `yaml:"domains,omitempty"`
}

// IsEmpty reports whether the allowlist grants access to nobody, which is how
// the operator asks for a public dashboard.
func (a AuthAllowlist) IsEmpty() bool {
	return len(a.Emails) == 0 && len(a.Domains) == 0
}

// AuthSessionConfig configures the signed session cookie.
type AuthSessionConfig struct {
	// Secret signs the session cookie. When empty a random one is generated
	// once per process, which means sessions do not survive a restart.
	Secret     string `yaml:"secret,omitempty"`
	MaxAge     string `yaml:"maxAge,omitempty"`
	CookieName string `yaml:"cookieName,omitempty"`
	// Secure marks the cookie Secure. It defaults to true; set it to false
	// only to test over plain http on localhost.
	Secure *bool `yaml:"secure,omitempty"`
}

// Provider names.
const (
	ProviderGoogle        = "google"
	ProviderTrustedHeader = "trustedHeader"
)

const (
	defaultSessionMaxAge = 168 * time.Hour // 7 days
	defaultCookieName    = "myserver_session"
)

// AuthState is the atomically published authentication policy. Handlers and
// middleware must read it per request via Auth() so that edits to auth.yaml
// take effect without a restart.
type AuthState struct {
	// Required reports whether a valid session is needed to see anything.
	Required bool
	// Lockdown means the policy could not be determined and the dashboard
	// must answer 503 everywhere rather than risk serving content publicly.
	Lockdown bool
	// Config is the policy in force. Non-nil whenever Required is true.
	Config *AuthConfig
	// Err is the last load or validation error, if any.
	Err error
	// Degraded reports that Config is a retained last-known-good copy
	// because the current file on disk is unusable.
	Degraded bool
}

// SessionMaxAge returns the configured session lifetime, or the default.
func (c *AuthConfig) SessionMaxAge() time.Duration {
	if c == nil || c.Session.MaxAge == "" {
		return defaultSessionMaxAge
	}
	d, err := time.ParseDuration(c.Session.MaxAge)
	if err != nil || d <= 0 {
		return defaultSessionMaxAge
	}
	return d
}

// CookieName returns the configured session cookie name, or the default.
func (c *AuthConfig) CookieName() string {
	if c == nil || c.Session.CookieName == "" {
		return defaultCookieName
	}
	return c.Session.CookieName
}

// CookieSecure reports whether the session cookie carries the Secure
// attribute. It defaults to true.
func (c *AuthConfig) CookieSecure() bool {
	if c == nil || c.Session.Secure == nil {
		return true
	}
	return *c.Session.Secure
}

// ProviderName returns the configured provider, defaulting to google.
func (c *AuthConfig) ProviderName() string {
	if c == nil || c.Provider == "" {
		return ProviderGoogle
	}
	return c.Provider
}

var (
	authState atomic.Value // AuthState

	// processSessionSecret backs deployments that do not set session.secret.
	// It is generated once per process (not once per reload) so that editing
	// an unrelated YAML does not log everybody out.
	processSessionSecret     string
	processSessionSecretOnce sync.Once
)

// Auth returns the current authentication policy. It never returns nil.
func Auth() AuthState {
	if s, ok := authState.Load().(AuthState); ok {
		return s
	}
	// Before the first load, refuse to claim the dashboard is public.
	return AuthState{Lockdown: true, Err: errors.New("auth policy not loaded yet")}
}

// ResetAuthState clears the published policy (for testing). Production code
// never calls it: dropping the policy would look like "no auth configured" to
// anything reading it before the next reload.
func ResetAuthState() {
	authState.Store(AuthState{})
}

// AuthRequiredEnv is the belt-and-braces switch: with it set, a dashboard
// whose auth policy failed to load answers 503 instead of anything else.
func AuthRequiredEnv() bool {
	return os.Getenv("HOMEPAGE_AUTH_REQUIRED") == "true"
}

// SessionSecret returns the signing key for session cookies: the configured
// one, or a per-process random fallback.
func (c *AuthConfig) SessionSecret() string {
	if c != nil && c.Session.Secret != "" {
		return c.Session.Secret
	}
	processSessionSecretOnce.Do(func() {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			// crypto/rand failing is not recoverable; a predictable signing
			// key would be worse than no sessions at all.
			panic(fmt.Sprintf("generating session secret: %v", err))
		}
		processSessionSecret = hex.EncodeToString(buf)
	})
	return processSessionSecret
}

// UsesGeneratedSecret reports whether the session key is the per-process
// random fallback, so callers can warn that sessions die on restart.
func (c *AuthConfig) UsesGeneratedSecret() bool {
	return c == nil || c.Session.Secret == ""
}

// ReloadAuth re-reads auth.yaml and atomically publishes the resulting policy.
// It is called by ReloadCache, i.e. at startup and on every hot-reload.
func ReloadAuth() {
	prev := Auth()
	authState.Store(reconcileAuth(prev, readAuthFile()))
}

// authRead is the raw outcome of looking at auth.yaml on disk.
type authRead struct {
	cfg     *AuthConfig
	missing bool
	err     error
}

// readAuthFile reads and parses auth.yaml without applying any policy
// decision. Unlike the other loaders there is no skeleton: the file must stay
// absent by default, since its mere presence is what turns login on.
func readAuthFile() authRead {
	path := filepath.Join(ConfigDir(), AuthFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return authRead{missing: true}
		}
		return authRead{err: fmt.Errorf("reading %s: %w", AuthFile, err)}
	}

	var cfg AuthConfig
	if err := yaml.Unmarshal([]byte(SubstituteEnvVars(string(raw))), &cfg); err != nil {
		return authRead{err: fmt.Errorf("parsing %s: %w", AuthFile, err)}
	}
	return authRead{cfg: &cfg}
}

// reconcileAuth turns a read of auth.yaml into the policy to publish, using
// the previous policy to tell "the operator opened the dashboard on purpose"
// apart from "we could not read the config".
//
//	file absent, was not required  -> public (the default, unchanged behaviour)
//	file absent, was required      -> lockdown (deleted? or did the mount die?)
//	parsed, allowlist empty        -> public (an unambiguous, well-formed order)
//	parsed, allowlist non-empty    -> required, with this config
//	unreadable/invalid, has LKG    -> keep last known good, degraded
//	unreadable/invalid, no LKG     -> lockdown (never assume public)
func reconcileAuth(prev AuthState, read authRead) AuthState {
	switch {
	case read.missing:
		if prev.Required {
			return AuthState{
				Required: true,
				Lockdown: true,
				Config:   prev.Config,
				Degraded: true,
				Err:      fmt.Errorf("%s disappeared while authentication was active", AuthFile),
			}
		}
		return AuthState{}

	case read.err != nil:
		if prev.Required && prev.Config != nil {
			return AuthState{Required: true, Config: prev.Config, Degraded: true, Err: read.err}
		}
		// A file that exists but cannot be understood is an error, never a
		// request to publish the dashboard.
		return AuthState{Lockdown: true, Err: read.err}

	default:
		cfg := read.cfg
		if cfg.Allowlist.IsEmpty() {
			return AuthState{}
		}
		if err := ValidateAuthConfig(cfg); err != nil {
			if prev.Required && prev.Config != nil {
				return AuthState{Required: true, Config: prev.Config, Degraded: true, Err: err}
			}
			return AuthState{Lockdown: true, Err: err}
		}
		return AuthState{Required: true, Config: cfg}
	}
}

// publicMailDomains are consumer mail providers: allowing one of them as a
// domain grants access to anyone who can register an address there, which is
// never what an allowlist is meant to express.
var publicMailDomains = map[string]bool{
	"gmail.com":        true,
	"googlemail.com":   true,
	"outlook.com":      true,
	"hotmail.com":      true,
	"live.com":         true,
	"msn.com":          true,
	"yahoo.com":        true,
	"ymail.com":        true,
	"proton.me":        true,
	"protonmail.com":   true,
	"icloud.com":       true,
	"me.com":           true,
	"aol.com":          true,
	"gmx.com":          true,
	"mail.com":         true,
	"yandex.com":       true,
	"zoho.com":         true,
	"fastmail.com":     true,
	"tutanota.com":     true,
	"hotmail.co.uk":    true,
	"outlook.es":       true,
	"hotmail.es":       true,
	"yahoo.es":         true,
	"yahoo.co.uk":      true,
	"googlegroups.com": true,
}

// ValidateAuthConfig checks a non-empty auth policy for the mistakes that
// would either break login or quietly widen access.
func ValidateAuthConfig(cfg *AuthConfig) error {
	if cfg == nil {
		return errors.New("auth config is nil")
	}

	for _, email := range cfg.Allowlist.Emails {
		e := strings.TrimSpace(email)
		if e == "" {
			return errors.New("allowlist.emails contains an empty entry")
		}
		if hasUnresolvedPlaceholder(e) {
			return fmt.Errorf("allowlist.emails contains an unresolved placeholder (%s)", e)
		}
		if !strings.Contains(e, "@") {
			return fmt.Errorf("allowlist.emails entry %q is not an email address", e)
		}
	}

	for _, domain := range cfg.Allowlist.Domains {
		d := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(domain, "@")))
		if d == "" {
			return errors.New("allowlist.domains contains an empty entry")
		}
		if hasUnresolvedPlaceholder(d) {
			return fmt.Errorf("allowlist.domains contains an unresolved placeholder (%s)", d)
		}
		if publicMailDomains[d] && !cfg.AllowPublicDomains {
			return fmt.Errorf(
				"allowlist.domains lists the public mail provider %q, which would let anyone "+
					"with an address there into the dashboard; list individual emails instead, "+
					"or set allowPublicDomains: true if this is really what you want", d)
		}
	}

	switch cfg.ProviderName() {
	case ProviderGoogle:
		if err := validateGoogleAuth(cfg.Google); err != nil {
			return err
		}
	case ProviderTrustedHeader:
		if strings.TrimSpace(cfg.TrustedHeader.Header) == "" {
			return errors.New("trustedHeader.header is required when provider is trustedHeader")
		}
		if hasUnresolvedPlaceholder(cfg.TrustedHeader.Header) {
			return errors.New("trustedHeader.header contains an unresolved placeholder")
		}
	default:
		return fmt.Errorf("unknown auth provider %q (want %q or %q)",
			cfg.Provider, ProviderGoogle, ProviderTrustedHeader)
	}

	if cfg.Session.MaxAge != "" {
		if d, err := time.ParseDuration(cfg.Session.MaxAge); err != nil || d <= 0 {
			return fmt.Errorf("session.maxAge %q is not a positive duration (e.g. \"168h\")", cfg.Session.MaxAge)
		}
	}
	if hasUnresolvedPlaceholder(cfg.Session.Secret) {
		return errors.New("session.secret contains an unresolved placeholder")
	}

	for _, p := range cfg.PublicPaths {
		if !strings.HasPrefix(p, "/") {
			return fmt.Errorf("publicPaths entry %q must start with /", p)
		}
	}

	return nil
}

func validateGoogleAuth(g GoogleAuthConfig) error {
	required := []struct {
		name, value string
	}{
		{"google.clientId", g.ClientID},
		{"google.clientSecret", g.ClientSecret},
		{"google.redirectURL", g.RedirectURL},
	}
	for _, f := range required {
		v := strings.TrimSpace(f.value)
		if v == "" {
			return fmt.Errorf("%s is required when the allowlist is not empty", f.name)
		}
		if hasUnresolvedPlaceholder(v) {
			return fmt.Errorf("%s still holds an unresolved placeholder — is the environment variable set?", f.name)
		}
	}
	if !strings.HasPrefix(g.RedirectURL, "http://") && !strings.HasPrefix(g.RedirectURL, "https://") {
		return fmt.Errorf("google.redirectURL must be an absolute http(s) URL, got %q", g.RedirectURL)
	}
	return nil
}

// hasUnresolvedPlaceholder reports whether a {{HOMEPAGE_VAR_*}} or
// {{HOMEPAGE_FILE_*}} reference survived substitution, which means the
// environment variable or file behind it is missing.
func hasUnresolvedPlaceholder(s string) bool {
	return strings.Contains(s, "{{HOMEPAGE_VAR_") || strings.Contains(s, "{{HOMEPAGE_FILE_")
}
