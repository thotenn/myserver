package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// KubernetesConfig represents a Kubernetes cluster connection configuration.
type KubernetesConfig struct {
	Kubeconfig string `yaml:"kubeconfig,omitempty"`
}

// LoadKubernetes loads and parses kubernetes.yaml.

// LoadKubernetes returns the cached config if available, otherwise loads from disk.
func LoadKubernetes() (map[string]KubernetesConfig, error) {
	if c := GetCachedConfig(); c != nil {

		return c.Kubernetes, nil
	}
	return loadKubernetes()
}
func loadKubernetes() (map[string]KubernetesConfig, error) {
	if err := CheckAndCopyConfig("kubernetes.yaml"); err != nil {
		return nil, err
	}

	data, err := ReadConfigFile("kubernetes.yaml")
	if err != nil {
		return nil, err
	}

	var configs map[string]KubernetesConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("parsing kubernetes.yaml: %w", err)
	}

	return configs, nil
}
