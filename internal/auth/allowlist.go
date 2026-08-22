// Package auth implements the optional email allowlist and the identity
// providers that feed it. It depends on internal/config for the policy and on
// nothing else in the tree, so the dependency direction stays one-way.
package auth

import (
	"strings"

	"github.com/thotenn/myserver/internal/config"
)

// NormalizeEmail lower-cases and trims an address so that comparisons are
// case-insensitive.
//
// It deliberately does NOT apply Gmail's dot- and plus-folding: to Google
// j.perez@gmail.com and jperez@gmail.com are the same mailbox, but silently
// treating them as equal here would surprise an operator reading the YAML,
// and treating them as different is the safer of the two surprises. List the
// address exactly as it is spelled.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// emailDomain returns the lower-cased domain part of an address, or "" when
// the input is not shaped like an email.
func emailDomain(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return email[at+1:]
}

// IsAllowed reports whether an authenticated email may enter the dashboard.
//
// It is called on every request, not just at login, so removing somebody from
// auth.yaml evicts them on their next request instead of when their cookie
// happens to expire.
func IsAllowed(cfg *config.AuthConfig, email string) bool {
	if cfg == nil {
		return false
	}
	normalized := NormalizeEmail(email)
	if normalized == "" {
		return false
	}

	for _, allowed := range cfg.Allowlist.Emails {
		if NormalizeEmail(allowed) == normalized {
			return true
		}
	}

	domain := emailDomain(normalized)
	if domain == "" {
		return false
	}
	for _, allowed := range cfg.Allowlist.Domains {
		// Accept both "example.com" and "@example.com" in the YAML.
		d := NormalizeEmail(strings.TrimPrefix(strings.TrimSpace(allowed), "@"))
		if d != "" && d == domain {
			return true
		}
	}
	return false
}
