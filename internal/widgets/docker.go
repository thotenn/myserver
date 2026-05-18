package widgets

// RegisterDocker registers the docker widget (container stats display).
func RegisterDocker(r *Registry) {
	r.Register(&simpleWidget{
		typeName: "docker",
		api:      "",
		mappings: nil, // Handled directly by Docker API
	})
}
