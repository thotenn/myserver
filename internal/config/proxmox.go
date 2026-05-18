package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ProxmoxConfig represents a Proxmox VE connection configuration.
type ProxmoxConfig struct {
	URL    string `yaml:"url"`
	Token  string `yaml:"token"`
	Secret string `yaml:"secret"`
}

// LoadProxmox loads and parses proxmox.yaml.

// LoadProxmox returns the cached config if available, otherwise loads from disk.
func LoadProxmox() (map[string]ProxmoxConfig, error) {
	if c := GetCachedConfig(); c != nil {

		return c.Proxmox, nil
	}
	return loadProxmox()
}
func loadProxmox() (map[string]ProxmoxConfig, error) {
	if err := CheckAndCopyConfig("proxmox.yaml"); err != nil {
		return nil, err
	}

	data, err := ReadConfigFile("proxmox.yaml")
	if err != nil {
		return nil, err
	}

	var configs map[string]ProxmoxConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("parsing proxmox.yaml: %w", err)
	}

	return configs, nil
}
