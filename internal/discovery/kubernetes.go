package discovery

import (
	"github.com/thotenn/myserver/internal/config"
)

// KubernetesDiscoverer discovers services from Kubernetes resources.
// TODO: implement full K8s discovery when needed.
type KubernetesDiscoverer struct {
	config config.KubernetesConfig
}

// NewKubernetesDiscoverer creates a new Kubernetes discoverer.
func NewKubernetesDiscoverer(cfg config.KubernetesConfig) (*KubernetesDiscoverer, error) {
	return &KubernetesDiscoverer{config: cfg}, nil
}

// DiscoverServices discovers services from K8s resources.
// Stub: returns empty list. Full implementation requires k8s.io/client-go.
func (k *KubernetesDiscoverer) DiscoverServices() ([]config.ServiceGroup, error) {
	return nil, nil
}
