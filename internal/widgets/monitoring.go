package widgets

// Monitoring widgets

func RegisterPortainer(r *Registry) {
	reg(r, "portainer", "{url}/api/{endpoint}", map[string]EndpointMapping{
		"info":      {Endpoint: "status"},
		"stacks":    {Endpoint: "stacks"},
		"endpoints": {Endpoint: "endpoints"},
	})
}

func RegisterUptimeKuma(r *Registry) {
	reg(r, "uptimekuma", "{url}/api/{endpoint}", map[string]EndpointMapping{
		"status": {Endpoint: "status-page"},
	})
}

func RegisterNetdata(r *Registry) {
	reg(r, "netdata", "{url}/api/v1/{endpoint}", map[string]EndpointMapping{
		"info":    {Endpoint: "info"},
		"alarms":  {Endpoint: "alarms"},
		"metrics": {Endpoint: "allmetrics"},
	})
}

func RegisterPrometheus(r *Registry) {
	reg(r, "prometheus", "{url}/api/v1/{endpoint}", map[string]EndpointMapping{
		"query": {Endpoint: "query"},
	})
}

func RegisterGrafana(r *Registry) {
	reg(r, "grafana", "{url}/api/{endpoint}", map[string]EndpointMapping{
		"stats": {Endpoint: "org"},
	})
}
