package widgets

// RegisterSpeedtest registers the Speedtest widget.
func RegisterSpeedtest(r *Registry) {
	r.Register(&simpleWidget{
		typeName: "speedtest",
		api:      "{url}",
		mappings: map[string]EndpointMapping{
			"status": {
				Endpoint: "",
			},
		},
	})
}
