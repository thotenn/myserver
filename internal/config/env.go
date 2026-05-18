package config

import (
	"os"
	"regexp"
	"strings"
)

var (
	envVarRe  = regexp.MustCompile(`\{\{HOMEPAGE_VAR_([^}]+)\}\}`)
	fileVarRe = regexp.MustCompile(`\{\{HOMEPAGE_FILE_([^}]+)\}\}`)
)

// SubstituteEnvVars replaces {{HOMEPAGE_VAR_*}} and {{HOMEPAGE_FILE_*}} placeholders.
// If a HOMEPAGE_VAR_* or HOMEPAGE_FILE_* reference cannot be resolved
// (undefined env var, missing/unreadable file) the placeholder is kept
// literally, so the error is visible in the rendered YAML instead of
// silently substituting an empty string.
func SubstituteEnvVars(input string) string {
	// Replace HOMEPAGE_VAR_* placeholders
	result := envVarRe.ReplaceAllStringFunc(input, func(match string) string {
		varName := envVarRe.FindStringSubmatch(match)[1]
		val, ok := os.LookupEnv("HOMEPAGE_VAR_" + varName)
		if !ok {
			return match
		}
		return val
	})

	// Replace HOMEPAGE_FILE_* placeholders (preserve file content verbatim,
	// no TrimSpace, matching Node.js Homepage behaviour).
	result = fileVarRe.ReplaceAllStringFunc(result, func(match string) string {
		varName := fileVarRe.FindStringSubmatch(match)[1]
		filePath := os.Getenv("HOMEPAGE_FILE_" + varName)
		if filePath == "" {
			return match
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return match
		}
		return string(data)
	})

	return result
}

// RawAllowedHosts returns the raw HOMEPAGE_ALLOWED_HOSTS value (empty string
// if unset, "*" if wildcard). Middlewares should prefer this over the parsed
// slice so they can distinguish the three modes (unset, wildcard, explicit).
func RawAllowedHosts() string {
	return os.Getenv("HOMEPAGE_ALLOWED_HOSTS")
}

// AllowedHosts returns the list of allowed hosts from HOMEPAGE_ALLOWED_HOSTS.
func AllowedHosts() []string {
	hosts := os.Getenv("HOMEPAGE_ALLOWED_HOSTS")
	if hosts == "" {
		return nil
	}
	if hosts == "*" {
		return nil // nil means allow all
	}
	parts := strings.Split(hosts, ",")
	for i, h := range parts {
		parts[i] = strings.TrimSpace(h)
	}
	return parts
}

// ProxyDisableIPv6 returns whether IPv6 should be disabled for proxy requests.
func ProxyDisableIPv6() bool {
	return os.Getenv("HOMEPAGE_PROXY_DISABLE_IPV6") == "true"
}

// ScriptsEnabled returns whether the scripts feature is enabled.
func ScriptsEnabled() bool {
	return os.Getenv("HOMEPAGE_SCRIPTS_ENABLED") == "true"
}

// AllowPrivateHosts opts out of the SSRF policy that blocks private/loopback
// IPs from the proxy. Required for self-hosted setups where many widgets
// point to internal services (e.g. http://plex:32400 inside a docker network).
func AllowPrivateHosts() bool {
	v := os.Getenv("HOMEPAGE_ALLOW_PRIVATE_HOSTS")
	// Default to true: this is a self-hosted dashboard, the dominant case
	// is internal-network widgets. Operators who deploy this on a multi-
	// tenant boundary should set HOMEPAGE_ALLOW_PRIVATE_HOSTS=false to
	// re-enable strict SSRF protection.
	if v == "" {
		return true
	}
	return v == "true"
}
