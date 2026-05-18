package widgets

// Info widgets — global widgets that appear at the top of the dashboard.
// These don't connect to external services (except weather/stocks).

func RegisterDateTime(r *Registry) {
	r.Register(&simpleWidget{
		typeName: "datetime",
		api:      "", // Rendered client-side
	})
}

func RegisterGreeting(r *Registry) {
	r.Register(&simpleWidget{
		typeName: "greeting",
		api:      "", // Rendered client-side
	})
}

func RegisterSearch(r *Registry) {
	r.Register(&simpleWidget{
		typeName: "search",
		api:      "", // Client-side search widget
	})
}

func RegisterWeather(r *Registry) {
	r.Register(&simpleWidget{
		typeName: "weather",
		api:      "https://api.openweathermap.org/data/2.5/{endpoint}?lat={latitude}&lon={longitude}&appid={apiKey}&units=metric",
		mappings: map[string]EndpointMapping{
			"current":  {Endpoint: "weather"},
			"forecast": {Endpoint: "forecast"},
		},
	})
}

func RegisterOpenMeteo(r *Registry) {
	r.Register(&simpleWidget{
		typeName: "openmeteo",
		api:      "https://api.open-meteo.com/v1/{endpoint}?latitude={latitude}&longitude={longitude}&current_weather=true&timezone=auto",
		mappings: map[string]EndpointMapping{
			"current": {Endpoint: "forecast"},
		},
	})
}

func RegisterStocks(r *Registry) {
	r.Register(&simpleWidget{
		typeName: "stocks",
		api:      "",
		mappings: nil, // Uses external stock API configured in settings
	})
}

func RegisterKubernetesInfo(r *Registry) {
	r.Register(&simpleWidget{
		typeName: "kubernetes",
		api:      "", // Uses k8s client directly
	})
}

func RegisterLonghorn(r *Registry) {
	r.Register(&simpleWidget{
		typeName: "longhorn",
		api:      "{url}/v1/{endpoint}",
		mappings: map[string]EndpointMapping{
			"nodes":   {Endpoint: "nodes"},
			"volumes": {Endpoint: "volumes"},
		},
	})
}
