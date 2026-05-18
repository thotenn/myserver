package widgets

// RegisterResources registers the system resources widget (CPU, RAM, disk, network, uptime).
// This widget reads from the local system, not from an external API.
func RegisterResources(r *Registry) {
	r.Register(&simpleWidget{
		typeName: "resources",
		api:      "", // No external API, uses local systeminformation
		mappings: nil,
	})
}
