package widgets

// Networking widgets

func RegisterPiHole(r *Registry) {
	reg(r, "pihole", "{url}/admin/api.php?{endpoint}&auth={key}", map[string]EndpointMapping{
		"stats": {Endpoint: ""},
	})
}

func RegisterAdGuard(r *Registry) {
	reg(r, "adguard", "{url}/control/{endpoint}", map[string]EndpointMapping{
		"stats":  {Endpoint: "stats"},
		"status": {Endpoint: "status"},
	})
}

func RegisterTraefik(r *Registry) {
	reg(r, "traefik", "{url}/api/{endpoint}", map[string]EndpointMapping{
		"overview": {Endpoint: "overview"},
	})
}

func RegisterCaddy(r *Registry) {
	reg(r, "caddy", "{url}/config/{endpoint}", map[string]EndpointMapping{
		"info": {Endpoint: ""},
	})
}

func RegisterNPM(r *Registry) {
	reg(r, "npm", "{url}/api/{endpoint}", map[string]EndpointMapping{
		"info": {Endpoint: ""},
	})
}

func RegisterCloudflared(r *Registry) {
	reg(r, "cloudflared", "{url}/{endpoint}", map[string]EndpointMapping{
		"info": {Endpoint: ""},
	})
}

func RegisterTailscale(r *Registry) {
	reg(r, "tailscale", "{url}/api/{endpoint}", map[string]EndpointMapping{
		"info": {Endpoint: ""},
	})
}
