package auth

import (
	"errors"
	"net/http"

	"github.com/thotenn/myserver/internal/config"
)

// ErrUntrustedPeer means a trustedHeader login was attempted from a peer that
// is not a configured reverse proxy.
var ErrUntrustedPeer = errors.New("identity header received from an untrusted peer")

// EmailFromTrustedHeader reads the identity a front proxy asserts in a header
// (Cf-Access-Authenticated-User-Email for Cloudflare Access, Remote-Email for
// Authelia, X-Forwarded-Email for oauth2-proxy, …) and hands it to the same
// allowlist as the Google provider.
//
// peerTrusted MUST come from the TRUSTED_PROXIES check, and the caller is the
// middleware because only it can see the immediate peer. Without that check
// the header is just a request field: anyone able to reach the port could set
// it and walk in. Callers pass false and this returns an error.
func EmailFromTrustedHeader(r *http.Request, cfg *config.AuthConfig, peerTrusted bool) (string, error) {
	if cfg == nil {
		return "", errors.New("no auth config")
	}
	if !peerTrusted {
		return "", ErrUntrustedPeer
	}
	email := NormalizeEmail(r.Header.Get(cfg.TrustedHeader.Header))
	if email == "" {
		return "", errors.New("identity header is absent or empty")
	}
	return email, nil
}
