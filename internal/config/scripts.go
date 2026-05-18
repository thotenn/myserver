package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ScriptsFile represents the structure of scripts.yaml.
type ScriptsFile struct {
	Scripts map[string]ScriptEntry `yaml:"scripts"`
}

// ScriptEntry represents a single script definition in scripts.yaml.
type ScriptEntry struct {
	Command         string            `yaml:"command"`
	Description     string            `yaml:"description,omitempty"`
	Args            []string          `yaml:"args,omitempty"`
	Env             map[string]string `yaml:"env,omitempty"`
	Timeout         int               `yaml:"timeout,omitempty"`
	RequireConfirm  bool              `yaml:"requireConfirm,omitempty"`
	Icon            string            `yaml:"icon,omitempty"`
	AllowConcurrent bool              `yaml:"allowConcurrent,omitempty"`
	LogOutput       bool              `yaml:"logOutput,omitempty"`
}

// LoadScriptsFile loads and parses scripts.yaml.

// LoadScriptsFile returns the cached config if available, otherwise loads from disk.
func LoadScriptsFile() (*ScriptsFile, error) {
	if c := GetCachedConfig(); c != nil {

		return c.Scripts, nil
	}
	return loadScriptsFile()
}
func loadScriptsFile() (*ScriptsFile, error) {
	if err := CheckAndCopyConfig("scripts.yaml"); err != nil {
		return nil, err
	}

	data, err := ReadConfigFile("scripts.yaml")
	if err != nil {
		return nil, err
	}

	var sf ScriptsFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parsing scripts.yaml: %w", err)
	}

	return &sf, nil
}
