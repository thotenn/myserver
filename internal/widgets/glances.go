package widgets

// RegisterGlances registers the Glances system monitoring widget.
func RegisterGlances(r *Registry) {
	r.Register(&simpleWidget{
		typeName: "glances",
		api:      "{url}/api/3/{endpoint}",
		mappings: map[string]EndpointMapping{
			"info": {
				Endpoint: "info",
			},
			"cpu": {
				Endpoint: "cpu",
			},
			"memory": {
				Endpoint: "mem",
			},
			"processcount": {
				Endpoint: "processcount",
			},
			"diskio": {
				Endpoint: "diskio",
			},
			"network": {
				Endpoint: "network",
			},
			"containers": {
				Endpoint: "containers",
			},
		},
	})
}
