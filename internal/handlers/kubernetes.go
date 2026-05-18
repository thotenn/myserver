package handlers

import (
	"encoding/json"
	"net/http"
)

// KubernetesStats handles GET /api/kubernetes/stats/{service}/{server}.
// Stub: returns empty response. Full implementation requires k8s.io/client-go.
func KubernetesStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{})
}

// KubernetesStatus handles GET /api/kubernetes/status/{service}/{server}.
// Stub: returns empty response.
func KubernetesStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{})
}
