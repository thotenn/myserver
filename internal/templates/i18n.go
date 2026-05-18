package templates

// translations holds the i18n string maps for en and es.
var translations = map[string]map[string]string{
	"en": {
		"resources.cpu":           "CPU",
		"resources.memory":        "Memory",
		"resources.disk":          "Disk",
		"resources.network":       "Network",
		"resources.uptime":        "Uptime",
		"resources.temperature":   "Temperature",
		"resources.processes":     "Processes",
		"resources.total":         "Total",
		"resources.free":          "Free",
		"resources.used":          "Used",
		"resources.bytes":         "B",
		"resources.kilobytes":     "KB",
		"resources.megabytes":     "MB",
		"resources.gigabytes":     "GB",
		"resources.terabytes":     "TB",
		"weather.humidity":        "Humidity",
		"weather.wind":            "Wind",
		"weather.feelsLike":       "Feels like",
		"search.placeholder":      "Search...",
		"quicklaunch.placeholder": "Search or enter URL...",
		"widgets.error":           "Error loading data",
		"widgets.loading":         "Loading...",
		"docker.running":          "Running",
		"docker.stopped":          "Stopped",
		"docker.healthy":          "Healthy",
		"docker.unhealthy":        "Unhealthy",
		"scripts.execute":         "Execute",
		"scripts.again":           "again",
		"scripts.confirm":         "Are you sure you want to run",
		"scripts.running":         "Running...",
		"scripts.success":         "Completed",
		"scripts.error":           "Error",
		"scripts.timeout":         "Timeout",
		"monitor.up":              "Up",
		"monitor.down":            "Down",
		"greeting.morning":        "Good morning",
		"greeting.afternoon":      "Good afternoon",
		"greeting.evening":        "Good evening",
		"greeting.night":          "Good night",
		"time.minutes":            "minutes",
		"time.hours":              "hours",
		"time.days":               "days",
		"time.ago":                "ago",
	},
	"es": {
		"resources.cpu":           "CPU",
		"resources.memory":        "Memoria",
		"resources.disk":          "Disco",
		"resources.network":       "Red",
		"resources.uptime":        "Tiempo activo",
		"resources.temperature":   "Temperatura",
		"resources.processes":     "Procesos",
		"resources.total":         "Total",
		"resources.free":          "Libre",
		"resources.used":          "Usado",
		"resources.bytes":         "B",
		"resources.kilobytes":     "KB",
		"resources.megabytes":     "MB",
		"resources.gigabytes":     "GB",
		"resources.terabytes":     "TB",
		"weather.humidity":        "Humedad",
		"weather.wind":            "Viento",
		"weather.feelsLike":       "Sensacion termica",
		"search.placeholder":      "Buscar...",
		"quicklaunch.placeholder": "Buscar o ingresar URL...",
		"widgets.error":           "Error al cargar datos",
		"widgets.loading":         "Cargando...",
		"docker.running":          "Ejecutando",
		"docker.stopped":          "Detenido",
		"docker.healthy":          "Saludable",
		"docker.unhealthy":        "No saludable",
		"scripts.execute":         "Ejecutar",
		"scripts.again":           "de nuevo",
		"scripts.confirm":         "Seguro que quieres ejecutar",
		"scripts.running":         "Ejecutando...",
		"scripts.success":         "Completado",
		"scripts.error":           "Error",
		"scripts.timeout":         "Tiempo agotado",
		"monitor.up":              "Activo",
		"monitor.down":            "Inactivo",
		"greeting.morning":        "Buenos dias",
		"greeting.afternoon":      "Buenas tardes",
		"greeting.evening":        "Buenas tardes",
		"greeting.night":          "Buenas noches",
		"time.minutes":            "minutos",
		"time.hours":              "horas",
		"time.days":               "dias",
		"time.ago":                "atras",
	},
}

// T returns the translated string for the given key in the given language.
func T(lang, key string) string {
	if m, ok := translations[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	if m, ok := translations["en"]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}
