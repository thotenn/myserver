package widgets

// Media management widgets: Sonarr, Radarr, Lidarr, Prowlarr, Bazarr, Overseerr
// All follow the *arr API pattern.

func RegisterSonarr(r *Registry) {
	reg(r, "sonarr", "{url}/api/v3/{endpoint}?apikey={key}", map[string]EndpointMapping{
		"queue":    {Endpoint: "queue"},
		"series":   {Endpoint: "series"},
		"wanted":   {Endpoint: "wanted/missing"},
		"calendar": {Endpoint: "calendar"},
	})
}

func RegisterRadarr(r *Registry) {
	reg(r, "radarr", "{url}/api/v3/{endpoint}?apikey={key}", map[string]EndpointMapping{
		"queue":    {Endpoint: "queue"},
		"movies":   {Endpoint: "movie"},
		"wanted":   {Endpoint: "wanted/missing"},
		"calendar": {Endpoint: "calendar"},
	})
}

func RegisterLidarr(r *Registry) {
	reg(r, "lidarr", "{url}/api/v1/{endpoint}?apikey={key}", map[string]EndpointMapping{
		"queue":   {Endpoint: "queue"},
		"artists": {Endpoint: "artist"},
		"album":   {Endpoint: "album"},
	})
}

func RegisterProwlarr(r *Registry) {
	reg(r, "prowlarr", "{url}/api/v1/{endpoint}?apikey={key}", map[string]EndpointMapping{
		"stats": {Endpoint: "stats"},
	})
}

func RegisterBazarr(r *Registry) {
	reg(r, "bazarr", "{url}/api/{endpoint}", map[string]EndpointMapping{
		"episodes": {Endpoint: "episodes"},
		"movies":   {Endpoint: "movies"},
	})
}

func RegisterOverseerr(r *Registry) {
	reg(r, "overseerr", "{url}/api/v1/{endpoint}", map[string]EndpointMapping{
		"request": {Endpoint: "request?take=0"},
		"movie":   {Endpoint: "movie"},
		"tv":      {Endpoint: "tv"},
	})
}

// Media servers

func RegisterPlex(r *Registry) {
	reg(r, "plex", "{url}{endpoint}?X-Plex-Token={key}", map[string]EndpointMapping{
		"unified":   {Endpoint: "/"},
		"sessions":  {Endpoint: "/status/sessions"},
		"libraries": {Endpoint: "/library/sections"},
	})
}

func RegisterJellyfin(r *Registry) {
	reg(r, "jellyfin", "{url}/{endpoint}", map[string]EndpointMapping{
		"info":     {Endpoint: "System/Info"},
		"sessions": {Endpoint: "Sessions"},
		"items":    {Endpoint: "Items/Counts"},
	})
}

func RegisterEmby(r *Registry) {
	reg(r, "emby", "{url}/emby/{endpoint}", map[string]EndpointMapping{
		"info":     {Endpoint: "System/Info"},
		"sessions": {Endpoint: "Sessions"},
		"items":    {Endpoint: "Items/Counts"},
	})
}

func RegisterTautulli(r *Registry) {
	reg(r, "tautulli", "{url}/api/v2?apikey={key}&cmd={endpoint}", map[string]EndpointMapping{
		"activity":  {Endpoint: "get_activity"},
		"info":      {Endpoint: "get_server_info"},
		"libraries": {Endpoint: "get_libraries"},
	})
}
