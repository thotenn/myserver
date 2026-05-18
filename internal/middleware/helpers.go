package middleware

import (
	"net"
	"net/http"
	"os"
	"strings"
)

// trustedProxyNets caches the parsed CIDRs from TRUSTED_PROXIES.
var trustedProxyNets []*net.IPNet

func init() {
	list := os.Getenv("TRUSTED_PROXIES")
	if list == "" {
		list = "127.0.0.1/8,::1/128"
	}
	for _, s := range strings.Split(list, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			continue
		}
		trustedProxyNets = append(trustedProxyNets, n)
	}
}

// isTrustedProxy reports whether the given IP address is in the trusted
// proxy list.
func isTrustedProxy(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range trustedProxyNets {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

// ClientIPFromRequest extracts the client IP. By default it trusts
// X-Forwarded-For / X-Real-IP only when the immediate peer is in the
// TRUSTED_PROXIES list (default 127.0.0.1/8 and ::1/128). Otherwise it
// uses RemoteAddr to prevent trivial spoofing from direct network access.
func ClientIPFromRequest(r *http.Request) string {
	peer := r.RemoteAddr
	host := peer
	if idx := strings.LastIndex(peer, ":"); idx >= 0 {
		host = peer[:idx]
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	}
	if isTrustedProxy(host) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if idx := strings.Index(xff, ","); idx >= 0 {
				return strings.TrimSpace(xff[:idx])
			}
			return strings.TrimSpace(xff)
		}
		if rip := r.Header.Get("X-Real-IP"); rip != "" {
			return rip
		}
	}
	return peer
}

// IsOriginAllowed returns true if the request's Origin is absent (curl,
// server-to-server) OR matches the Host header of the current request.
// This is a minimum-viable same-origin enforcement for the scripts
// endpoints; it is NOT a replacement for proper auth in front of the app.
func IsOriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	// Parse host from origin.
	o := origin
	if idx := strings.Index(o, "://"); idx >= 0 {
		o = o[idx+3:]
	}
	return strings.EqualFold(o, r.Host)
}
