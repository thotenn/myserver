package widgets

// Productivity widgets

func RegisterNextcloud(r *Registry) {
	reg(r, "nextcloud", "{url}/ocs/v2.php/apps/serverinfo/api/v1/info?format=json", map[string]EndpointMapping{
		"info": {Endpoint: ""},
	})
}

func RegisterTrilium(r *Registry) {
	reg(r, "trilium", "{url}/api/{endpoint}", map[string]EndpointMapping{
		"info": {Endpoint: "app-info"},
	})
}

func RegisterPaperlessNGX(r *Registry) {
	reg(r, "paperlessngx", "{url}/api/{endpoint}", map[string]EndpointMapping{
		"stats": {Endpoint: "statistics/"},
	})
}
