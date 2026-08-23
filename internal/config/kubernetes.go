package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// KubernetesConfig represents a Kubernetes cluster connection configuration.
type KubernetesConfig struct {
	Kubeconfig string `yaml:"kubeconfig,omitempty"`
}

func loadKubernetes(dir string) (map[string]KubernetesConfig, error) {
	data, err := readConfigFile(dir, "kubernetes.yaml")
	if err != nil {
		return nil, err
	}

	var configs map[string]KubernetesConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("parsing kubernetes.yaml: %w", err)
	}

	return configs, nil
}
