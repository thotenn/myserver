package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Settings represents the global dashboard settings.
type Settings struct {
	Title           string                 `yaml:"title,omitempty" json:"title,omitempty"`
	Favicon         string                 `yaml:"favicon,omitempty" json:"favicon,omitempty"`
	Theme           string                 `yaml:"theme,omitempty" json:"theme,omitempty"`
	Color           string                 `yaml:"color,omitempty" json:"color,omitempty"`
	Columns         interface{}            `yaml:"columns,omitempty" json:"columns,omitempty"`
	Target          string                 `yaml:"target,omitempty" json:"target,omitempty"`
	Layout          map[string]LayoutGroup `yaml:"layout,omitempty" json:"layout,omitempty"`
	HeaderStyle     string                 `yaml:"headerStyle,omitempty" json:"headerStyle,omitempty"`
	Language        string                 `yaml:"language,omitempty" json:"language,omitempty"`
	CardBlur        bool                   `yaml:"cardBlur,omitempty" json:"cardBlur,omitempty"`
	BackgroundImage string                 `yaml:"backgroundImage,omitempty" json:"backgroundImage,omitempty"`
	Providers       map[string]string      `yaml:"providers,omitempty" json:"-"`
	QuickLaunch     *QuickLaunch           `yaml:"quicklaunch,omitempty" json:"quicklaunch,omitempty"`
	ScriptSettings  *ScriptSettings        `yaml:"scripts,omitempty" json:"scripts,omitempty"`
}

// LayoutGroup defines layout options for a service group.
type LayoutGroup struct {
	Style   string `yaml:"style,omitempty" json:"style,omitempty"`
	Columns int    `yaml:"columns,omitempty" json:"columns,omitempty"`
	Tab     string `yaml:"tab,omitempty" json:"tab,omitempty"`
}

// QuickLaunch defines quick launch search settings.
type QuickLaunch struct {
	SearchDescriptions bool `yaml:"searchDescriptions,omitempty" json:"searchDescriptions,omitempty"`
	HideInternetSearch bool `yaml:"hideInternetSearch,omitempty" json:"hideInternetSearch,omitempty"`
	HideVisitURL       bool `yaml:"hideVisitURL,omitempty" json:"hideVisitURL,omitempty"`
}

// ScriptSettings defines script feature settings.
type ScriptSettings struct {
	Enabled        bool     `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	MaxTimeout     int      `yaml:"maxTimeout,omitempty" json:"maxTimeout,omitempty"`
	DefaultTimeout int      `yaml:"defaultTimeout,omitempty" json:"defaultTimeout,omitempty"`
	MaxConcurrent  int      `yaml:"maxConcurrent,omitempty" json:"maxConcurrent,omitempty"`
	ScriptDirs     []string `yaml:"scriptDirs,omitempty" json:"scriptDirs,omitempty"`
}

func loadSettings(dir string) (*Settings, error) {
	data, err := readConfigFile(dir, "settings.yaml")
	if err != nil {
		return nil, err
	}

	var settings Settings
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parsing settings.yaml: %w", err)
	}

	// Apply defaults
	if settings.Theme == "" {
		settings.Theme = "dark"
	}
	if settings.Color == "" {
		settings.Color = "slate"
	}
	if settings.Language == "" {
		settings.Language = "en"
	}
	if settings.Target == "" {
		settings.Target = "_blank"
	}
	if settings.HeaderStyle == "" {
		settings.HeaderStyle = "underlined"
	}

	return &settings, nil
}
