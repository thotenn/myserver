package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/thotenn/myserver/internal/config"
	"github.com/thotenn/myserver/internal/proxy"
)

// ProxmoxStats handles GET /api/proxmox/stats/{vmid}/{server}.
func ProxmoxStats(w http.ResponseWriter, r *http.Request) {
	vmid := chi.URLParam(r, "vmid")
	server := chi.URLParam(r, "server")

	if vmid == "" {
		http.Error(w, "missing vmid", http.StatusBadRequest)
		return
	}

	proxmoxConfigs, err := config.LoadProxmox()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cfg, ok := proxmoxConfigs[server]
	if !ok {
		http.Error(w, "proxmox server not found", http.StatusNotFound)
		return
	}

	targetURL := strings.TrimRight(cfg.URL, "/") + "/api2/json/cluster/resources?type=vm"

	params := &proxy.Params{
		Method: http.MethodGet,
		Headers: map[string]string{
			"Authorization": "PVEAPIToken=" + cfg.Token + "=" + cfg.Secret,
		},
		FollowRedirects: true,
		IgnoreTLS:       true,
	}

	result, err := proxy.Proxy(r.Context(), targetURL, params)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}

	if result.Status >= http.StatusBadRequest {
		http.Error(w, "upstream returned error", http.StatusBadGateway)
		return
	}

	var response struct {
		Data []struct {
			VMID    int     `json:"vmid"`
			Name    string  `json:"name"`
			Status  string  `json:"status"`
			Node    string  `json:"node"`
			MaxMem  int64   `json:"maxmem"`
			Mem     int64   `json:"mem"`
			MaxCPU  int     `json:"maxcpu"`
			CPU     float64 `json:"cpu"`
			MaxDisk int64   `json:"maxdisk"`
			Disk    int64   `json:"disk"`
			Uptime  int64   `json:"uptime"`
		} `json:"data"`
	}

	if err := json.Unmarshal(result.Body, &response); err != nil {
		http.Error(w, "failed to parse upstream response", http.StatusInternalServerError)
		return
	}

	vmidInt, _ := strconv.Atoi(vmid)
	for _, vm := range response.Data {
		if vm.VMID == vmidInt {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"vmid":    vm.VMID,
				"name":    vm.Name,
				"status":  vm.Status,
				"node":    vm.Node,
				"maxmem":  vm.MaxMem,
				"mem":     vm.Mem,
				"maxcpu":  vm.MaxCPU,
				"cpu":     vm.CPU,
				"maxdisk": vm.MaxDisk,
				"disk":    vm.Disk,
				"uptime":  vm.Uptime,
			})
			return
		}
	}

	http.Error(w, "vm not found", http.StatusNotFound)
}
