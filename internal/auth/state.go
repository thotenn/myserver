package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
)

// The OAuth state cookie is signed with a key that belongs to the PROCESS, not
// to a dashboard, and that is forced by what the cookie is for.
//
// With a shared Google callback, the login that started under one dashboard
// comes back to another's route: the callback has to open the cookie before it
// knows which dashboard the flow belongs to, so it cannot use that dashboard's
// key. A per-process key is exactly right here — the cookie lives ten minutes,
// and losing every in-flight login on a restart means "log in again", not a
// lost session.
//
// The signature is what lets the slug inside the payload be trusted. It is the
// slug that decides which allowlist the login is validated against, so it must
// not be something the caller can rewrite in their own browser.
var (
	stateKey     []byte
	stateKeyOnce sync.Once
)

func oauthStateKey() []byte {
	stateKeyOnce.Do(func() {
		stateKey = make([]byte, 32)
		if _, err := rand.Read(stateKey); err != nil {
			// crypto/rand failing is not recoverable, and a predictable key
			// here would make the signature decorative.
			panic(fmt.Sprintf("generating oauth state key: %v", err))
		}
	})
	return stateKey
}

// SealState encodes and signs an OAuth state payload for transport in a cookie.
func SealState(payload []byte) string {
	body := base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + sign(string(oauthStateKey()), body)
}

// OpenState verifies a sealed payload and returns it. A value that was
// tampered with, truncated or signed by another process is rejected.
func OpenState(value string) ([]byte, bool) {
	dot := strings.LastIndex(value, ".")
	if dot < 0 {
		return nil, false
	}
	body, sig := value[:dot], value[dot+1:]
	// Constant-time: a byte-wise early exit would leak the expected signature
	// one character at a time.
	if !hmac.Equal([]byte(sig), []byte(sign(string(oauthStateKey()), body))) {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return nil, false
	}
	return payload, true
}
