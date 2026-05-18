package config

import (
	"fmt"
	"net/url"
	"strings"

	"gopkg.in/yaml.v3"
)

// ServiceGroup represents a named group of services.
type ServiceGroup struct {
	Name     string    `yaml:"name" json:"name"`
	Services []Service `yaml:"services" json:"services"`
}

// Service represents a single service entry.
type Service struct {
	Name           string            `yaml:"name" json:"name"`
	Href           string            `yaml:"href,omitempty" json:"href,omitempty"`
	Icon           string            `yaml:"icon,omitempty" json:"icon,omitempty"`
	Description    string            `yaml:"description,omitempty" json:"description,omitempty"`
	Widget         *WidgetConfig     `yaml:"widget,omitempty" json:"widget,omitempty"`
	Ping           string            `yaml:"ping,omitempty" json:"ping,omitempty"`
	SiteMonitor    string            `yaml:"siteMonitor,omitempty" json:"siteMonitor,omitempty"`
	StatusStyle    string            `yaml:"statusStyle,omitempty" json:"statusStyle,omitempty"`
	Weight         int               `yaml:"weight,omitempty" json:"weight,omitempty"`
	Tabindex       int               `yaml:"tabindex,omitempty" json:"tabindex,omitempty"`
	Type           string            `yaml:"type,omitempty" json:"type,omitempty"`
	Script         string            `yaml:"script,omitempty" json:"script,omitempty"`
	RequireConfirm bool              `yaml:"requireConfirm,omitempty" json:"requireConfirm,omitempty"`
	Server         string            `yaml:"server,omitempty" json:"server,omitempty"`
	Container      string            `yaml:"container,omitempty" json:"container,omitempty"`
	ShowStats      bool              `yaml:"showStats,omitempty" json:"showStats,omitempty"`
	Labels         map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}

// WidgetConfig represents the widget configuration for a service.
type WidgetConfig struct {
	Type     string            `yaml:"type" json:"type"`
	URL      string            `yaml:"url,omitempty" json:"url,omitempty"`
	Key      string            `yaml:"key,omitempty" json:"-"`
	Username string            `yaml:"username,omitempty" json:"-"`
	Password string            `yaml:"password,omitempty" json:"-"`
	APIKey   string            `yaml:"apiKey,omitempty" json:"-"`
	Token    string            `yaml:"token,omitempty" json:"-"`
	Secret   string            `yaml:"secret,omitempty" json:"-"`
	Headers  map[string]string `yaml:"headers,omitempty" json:"-"`
	Method   string            `yaml:"method,omitempty" json:"method,omitempty"`
	Body     interface{}       `yaml:"body,omitempty" json:"body,omitempty"`
	Mappings interface{}       `yaml:"mappings,omitempty" json:"mappings,omitempty"`
	// Display selects an alternate render mode for generic widgets
	// (currently: `dynamic-list` on the customapi widget). When set, the
	// Proxy handler content-negotiates and returns server-rendered HTML
	// for HTMX requests instead of raw JSON.
	Display string `yaml:"display,omitempty" json:"display,omitempty"`
	// Widget-specific fields stored as generic map
	Options map[string]interface{} `yaml:",inline" json:"-"`
}

// LoadServices loads and parses services.yaml.

// LoadServices returns the cached config if available, otherwise loads from disk.
func LoadServices() ([]ServiceGroup, error) {
	if c := GetCachedConfig(); c != nil {

		return c.Services, nil
	}
	return loadServices()
}
func loadServices() ([]ServiceGroup, error) {
	if err := CheckAndCopyConfig("services.yaml"); err != nil {
		return nil, err
	}

	data, err := ReadConfigFile("services.yaml")
	if err != nil {
		return nil, err
	}

	// services.yaml is a list where each item is {groupName: [{svcName: {svcFields}}]}
	var rawGroups []map[string][]map[string]Service
	if err := yaml.Unmarshal(data, &rawGroups); err != nil {
		return nil, fmt.Errorf("parsing services.yaml: %w", err)
	}

	var groups []ServiceGroup
	for _, rawGroup := range rawGroups {
		for groupName, svcList := range rawGroup {
			group := ServiceGroup{
				Name: groupName,
			}
			for _, svcMap := range svcList {
				for svcName, svc := range svcMap {
					svc.Name = svcName
					group.Services = append(group.Services, svc)
				}
			}
			groups = append(groups, group)
		}
	}

	return groups, nil
}

// SanitizeService removes sensitive fields from a service for API responses.
// It strips:
//   - Widget credential fields (Key, Username, Password, APIKey, Token, Secret, Headers)
//   - Basic-auth and sensitive query params from Widget.URL
//   - Sensitive entries (recursively) from Widget.Body and Widget.Options
//
// It preserves Widget.Method, Widget.Mappings, Widget.Body (sanitized) and
// Widget.Options (sanitized) so the frontend can render the widget.
func SanitizeService(s Service) Service {
	clean := s
	if clean.Widget != nil {
		w := *clean.Widget
		w.Key = ""
		w.Username = ""
		w.Password = ""
		w.APIKey = ""
		w.Token = ""
		w.Secret = ""
		w.Headers = nil
		w.URL = sanitizeWidgetURL(w.URL)
		w.Body = sanitizeValue(w.Body)
		if len(w.Options) > 0 {
			cleanOpts := make(map[string]interface{}, len(w.Options))
			for k, v := range w.Options {
				if IsSensitiveKey(k) {
					continue
				}
				cleanOpts[k] = sanitizeValue(v)
			}
			w.Options = cleanOpts
		}
		clean.Widget = &w
	}
	return clean
}

// sanitizeWidgetURL strips userinfo (basic-auth) and known credential
// query params from a widget URL. It returns the URL unchanged if it can't
// be parsed.
func sanitizeWidgetURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.User != nil {
		u.User = nil
	}
	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			if IsSensitiveKey(k) {
				q.Del(k)
			}
		}
		u.RawQuery = q.Encode()
	}
	return strings.TrimRight(u.String(), "?")
}
