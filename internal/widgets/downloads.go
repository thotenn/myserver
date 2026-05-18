package widgets

// Download client widgets

func RegisterQBittorrent(r *Registry) {
	reg(r, "qbittorrent", "{url}/api/v2/{endpoint}", map[string]EndpointMapping{
		"info":     {Endpoint: "transfer/info"},
		"torrents": {Endpoint: "torrents/info"},
	})
}

func RegisterTransmission(r *Registry) {
	reg(r, "transmission", "{url}/transmission/rpc", map[string]EndpointMapping{
		"stats": {Endpoint: ""},
	})
}

func RegisterDeluge(r *Registry) {
	reg(r, "deluge", "{url}/json", map[string]EndpointMapping{
		"stats": {Endpoint: ""},
	})
}

func RegisterSABnzbd(r *Registry) {
	reg(r, "sabnzbd", "{url}/api?mode={endpoint}&output=json&apikey={key}", map[string]EndpointMapping{
		"queue":   {Endpoint: "queue"},
		"history": {Endpoint: "history"},
	})
}
