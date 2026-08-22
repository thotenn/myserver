package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/thotenn/myserver/internal/config"
)

// Google OAuth 2.0 endpoints. Hardcoded rather than fetched from the OIDC
// discovery document: one less network dependency at startup, and these have
// been stable for the lifetime of the API.
const (
	googleAuthEndpoint  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenEndpoint = "https://oauth2.googleapis.com/token"
)

// googleIssuers are the two spellings Google uses in the iss claim.
var googleIssuers = map[string]bool{
	"accounts.google.com":         true,
	"https://accounts.google.com": true,
}

// clockSkew tolerates a small clock difference against Google when checking
// the token expiry.
const clockSkew = 60 * time.Second

// tokenHTTPClient is used for the server-to-server token exchange. It gets a
// short timeout of its own: the request happens while a user waits on the
// callback, and the default client would hang indefinitely.
var tokenHTTPClient = &http.Client{Timeout: 15 * time.Second}

// AuthorizationURL builds the URL the browser is redirected to in order to
// start the Google login.
//
// scope is "openid email" and nothing else: an allowlist needs the address,
// not the profile or the picture, and asking for less means a smaller consent
// screen and less data to handle.
func AuthorizationURL(cfg *config.AuthConfig, state, nonce string) string {
	q := url.Values{}
	q.Set("client_id", cfg.Google.ClientID)
	q.Set("redirect_uri", cfg.Google.RedirectURL)
	q.Set("response_type", "code")
	q.Set("scope", "openid email")
	q.Set("state", state)
	q.Set("nonce", nonce)
	// select_account avoids silently reusing whichever Google account the
	// browser happens to be signed into.
	q.Set("prompt", "select_account")
	if hd := strings.TrimSpace(cfg.Google.HostedDomain); hd != "" {
		q.Set("hd", hd)
	}
	return googleAuthEndpoint + "?" + q.Encode()
}

// tokenResponse is the subset of the token endpoint reply we consume.
type tokenResponse struct {
	IDToken string `json:"id_token"`
	Error   string `json:"error"`
}

// idTokenClaims is the subset of the ID Token payload we validate.
type idTokenClaims struct {
	Issuer        string `json:"iss"`
	Audience      string `json:"aud"`
	Expiry        int64  `json:"exp"`
	Nonce         string `json:"nonce"`
	Email         string `json:"email"`
	EmailVerified any    `json:"email_verified"`
}

// ExchangeCode swaps an authorization code for a verified email address.
//
// The ID Token's signature is deliberately not verified. OIDC Core §3.1.3.7
// item 6 allows skipping it when the token is obtained directly from the
// token endpoint over a TLS channel the client validates — which is exactly
// this call: our backend POSTs to oauth2.googleapis.com over TLS and
// authenticates with the client secret. That keeps the dependency list at
// stdlib only: no JWKS fetching, no key cache, no JWT library.
//
// The claims below are NOT optional, they are what actually makes the token
// trustworthy. If this code ever accepts a token from anywhere other than the
// token endpoint (implicit flow, One Tap, a generic IdP), signature
// verification becomes mandatory again.
func ExchangeCode(ctx context.Context, cfg *config.AuthConfig, code, nonce string) (string, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", cfg.Google.ClientID)
	form.Set("client_secret", cfg.Google.ClientSecret)
	form.Set("redirect_uri", cfg.Google.RedirectURL)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenEndpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := tokenHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// tr.Error is Google's error code ("invalid_grant"), never the
		// client secret, so it is safe to keep for the internal log.
		return "", fmt.Errorf("token endpoint returned %d (%s)", resp.StatusCode, tr.Error)
	}
	if tr.IDToken == "" {
		return "", errors.New("token response carried no id_token")
	}

	return emailFromIDToken(tr.IDToken, cfg.Google.ClientID, nonce)
}

// emailFromIDToken parses an ID Token and returns the email once every claim
// checks out.
func emailFromIDToken(idToken, clientID, nonce string) (string, error) {
	claims, err := parseIDTokenClaims(idToken)
	if err != nil {
		return "", err
	}

	if !googleIssuers[claims.Issuer] {
		return "", fmt.Errorf("unexpected id_token issuer %q", claims.Issuer)
	}
	if claims.Audience != clientID {
		return "", errors.New("id_token audience does not match the configured client id")
	}
	if claims.Expiry == 0 || time.Now().After(time.Unix(claims.Expiry, 0).Add(clockSkew)) {
		return "", errors.New("id_token is expired")
	}
	if nonce == "" || claims.Nonce != nonce {
		return "", errors.New("id_token nonce does not match the login attempt")
	}
	if !isTrue(claims.EmailVerified) {
		return "", errors.New("id_token email is not verified")
	}
	email := NormalizeEmail(claims.Email)
	if email == "" || !strings.Contains(email, "@") {
		return "", errors.New("id_token carried no usable email")
	}
	return email, nil
}

// parseIDTokenClaims decodes the payload segment of a JWT.
func parseIDTokenClaims(idToken string) (*idTokenClaims, error) {
	segments := strings.Split(idToken, ".")
	if len(segments) != 3 {
		return nil, errors.New("id_token is not a well-formed JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(segments[1])
	if err != nil {
		return nil, fmt.Errorf("decoding id_token payload: %w", err)
	}
	var claims idTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("parsing id_token claims: %w", err)
	}
	return &claims, nil
}

// isTrue interprets email_verified, which Google has historically sent both
// as a JSON boolean and as the string "true".
func isTrue(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true"
	default:
		return false
	}
}
