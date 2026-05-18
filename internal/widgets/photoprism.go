package widgets

// RegisterPhotoPrism registers the PhotoPrism photo management widget.
func RegisterPhotoPrism(r *Registry) {
	r.Register(&simpleWidget{
		typeName: "photoprism",
		api:      "{url}/api/v1/{endpoint}",
		mappings: map[string]EndpointMapping{
			"photos": {
				Endpoint: "photos?count=true",
			},
			"albums": {
				Endpoint: "albums?count=true",
			},
		},
	})
}
