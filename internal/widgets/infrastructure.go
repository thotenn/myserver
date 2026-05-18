package widgets

// Infrastructure widgets

func RegisterProxmox(r *Registry) {
	reg(r, "proxmox", "{url}/api2/json/{endpoint}", map[string]EndpointMapping{
		"version": {Endpoint: "version"},
		"nodes":   {Endpoint: "nodes"},
	})
}

func RegisterArgoCD(r *Registry) {
	reg(r, "argocd", "{url}/api/v1/{endpoint}", map[string]EndpointMapping{
		"applications": {Endpoint: "applications"},
	})
}
