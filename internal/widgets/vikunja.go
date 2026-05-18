package widgets

// RegisterVikunja registers the Vikunja task management widget.
func RegisterVikunja(r *Registry) {
	r.Register(&simpleWidget{
		typeName: "vikunja",
		api:      "{url}/api/v1/{endpoint}",
		mappings: map[string]EndpointMapping{
			"tasks": {
				Endpoint: "tasks/all",
			},
			"projects": {
				Endpoint: "projects",
			},
		},
	})
}
