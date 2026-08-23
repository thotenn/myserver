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

func loadProxmox(dir string) (map[string]ProxmoxConfig, error) {
	data, err := readConfigFile(dir, "proxmox.yaml")
	if err != nil {
		return nil, err
	}

	var configs map[string]ProxmoxConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("parsing proxmox.yaml: %w", err)
	}

	return configs, nil
}
