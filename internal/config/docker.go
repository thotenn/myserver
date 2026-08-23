package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// DockerConfig represents a Docker server connection configuration.
type DockerConfig struct {
	Socket  string            `yaml:"socket,omitempty"`
	Host    string            `yaml:"host,omitempty"`
	Port    int               `yaml:"port,omitempty"`
	TLS     *TLSConfig        `yaml:"tls,omitempty"`
	Swarm   bool              `yaml:"swarm,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

// TLSConfig represents TLS certificate configuration.
type TLSConfig struct {
	CA   string `yaml:"ca,omitempty"`
	Cert string `yaml:"cert,omitempty"`
	Key  string `yaml:"key,omitempty"`
}

func loadDocker(dir string) (map[string]DockerConfig, error) {
	data, err := readConfigFile(dir, "docker.yaml")
	if err != nil {
		return nil, err
	}

	var configs map[string]DockerConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("parsing docker.yaml: %w", err)
	}

	return configs, nil
}
