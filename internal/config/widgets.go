package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// InfoWidget represents a global info widget (datetime, search, weather, etc.).
type InfoWidget struct {
	Type    string                 `yaml:"type" json:"type"`
	Options map[string]interface{} `yaml:",inline" json:"options"`
}

func loadWidgets(dir string) ([]InfoWidget, error) {
	data, err := readConfigFile(dir, "widgets.yaml")
	if err != nil {
		return nil, err
	}

	var rawList []map[string]map[string]interface{}
	if err := yaml.Unmarshal(data, &rawList); err != nil {
		return nil, fmt.Errorf("parsing widgets.yaml: %w", err)
	}

	var widgets []InfoWidget
	for _, rawItem := range rawList {
		for widgetType, opts := range rawItem {
			w := InfoWidget{
				Type:    widgetType,
				Options: make(map[string]interface{}),
			}
			for k, v := range opts {
				w.Options[k] = v
			}
			widgets = append(widgets, w)
		}
	}

	return widgets, nil
}

// sensitiveKeySubstrings is the list of substrings (case-insensitive) used
// to detect credential-bearing keys when sanitizing widget option maps for
// public API responses.
var sensitiveKeySubstrings = []string{
	"key", "apikey", "api_key",
	"token", "accesstoken", "refreshtoken", "bearertoken",
	"secret", "clientsecret", "appkey",
	"password", "pass", "passwordfield",
	"auth", "authorization", "cookie",
	"username", "user", "account",
	"hash", "salt", "credential",
}

// IsSensitiveKey reports whether a key name should be considered sensitive.
// The match is case-insensitive and substring-based so variants like
// `apiKey`, `API_KEY`, `accessToken`, `clientSecret` are all caught.
func IsSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, needle := range sensitiveKeySubstrings {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// sanitizeValue strips sensitive entries from arbitrary nested values
// (maps, slices, scalars). It returns a deep-cloned tree safe to serialize.
func sanitizeValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, v2 := range val {
			if IsSensitiveKey(k) {
				continue
			}
			out[k] = sanitizeValue(v2)
		}
		return out
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, v2 := range val {
			ks, _ := k.(string)
			if ks == "" || IsSensitiveKey(ks) {
				continue
			}
			out[ks] = sanitizeValue(v2)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, item := range val {
			out[i] = sanitizeValue(item)
		}
		return out
	default:
		return val
	}
}

// SanitizeWidgets removes sensitive fields from widgets for API responses.
// The matching is case-insensitive, substring-based and recursive into
// nested maps/slices, so credentials hidden in headers/body do not leak.
func SanitizeWidgets(widgets []InfoWidget) []InfoWidget {
	clean := make([]InfoWidget, len(widgets))
	for i, w := range widgets {
		clean[i] = InfoWidget{
			Type:    w.Type,
			Options: make(map[string]interface{}),
		}
		for k, v := range w.Options {
			if IsSensitiveKey(k) {
				continue
			}
			clean[i].Options[k] = sanitizeValue(v)
		}
	}
	return clean
}
