package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/thotenn/myserver/internal/proxy"
	"github.com/thotenn/myserver/internal/templates"
)

// OpenMeteoWidget handles GET /api/widgets/openmeteo.
// Fetches the current weather from api.open-meteo.com (no API key needed)
// and renders HTML for HTMX clients or JSON otherwise.
func OpenMeteoWidget(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lat := q.Get("latitude")
	lon := q.Get("longitude")
	label := q.Get("label")
	if lat == "" || lon == "" {
		http.Error(w, "missing latitude/longitude", http.StatusBadRequest)
		return
	}

	u := url.URL{
		Scheme: "https",
		Host:   "api.open-meteo.com",
		Path:   "/v1/forecast",
	}
	values := url.Values{}
	values.Set("latitude", lat)
	values.Set("longitude", lon)
	values.Set("current_weather", "true")
	values.Set("temperature_unit", "celsius")
	if tz := q.Get("timezone"); tz != "" {
		values.Set("timezone", tz)
	}
	u.RawQuery = values.Encode()

	ctx, cancel := r.Context(), func() {}
	defer cancel()
	_ = time.Second

	result, err := proxy.Proxy(ctx, u.String(), &proxy.Params{
		Method:          http.MethodGet,
		FollowRedirects: true,
	})
	if err != nil {
		writeWeatherResult(w, r, label, 0, "❓", err.Error())
		return
	}
	if result.Status != http.StatusOK {
		writeWeatherResult(w, r, label, 0, "❓", "upstream error")
		return
	}

	var resp struct {
		CurrentWeather struct {
			Temperature float64 `json:"temperature"`
			WeatherCode int     `json:"weathercode"`
		} `json:"current_weather"`
	}
	if err := json.Unmarshal(result.Body, &resp); err != nil {
		writeWeatherResult(w, r, label, 0, "❓", err.Error())
		return
	}

	icon := weatherCodeIcon(resp.CurrentWeather.WeatherCode)
	writeWeatherResult(w, r, label, resp.CurrentWeather.Temperature, icon, "")
}

func writeWeatherResult(w http.ResponseWriter, r *http.Request, label string, tempC float64, icon, errMsg string) {
	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.WeatherHTML(label, tempC, icon).Render(r.Context(), w)
		return
	}
	payload := map[string]interface{}{
		"label":       label,
		"temperature": tempC,
		"icon":        icon,
	}
	if errMsg != "" {
		payload["errors"] = []string{errMsg}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// weatherCodeIcon maps an Open-Meteo WMO weather interpretation code to a
// small emoji glyph. See https://open-meteo.com/en/docs
func weatherCodeIcon(code int) string {
	switch {
	case code == 0:
		return "☀️"
	case code == 1 || code == 2:
		return "⛅"
	case code == 3:
		return "☁️"
	case code == 45 || code == 48:
		return "🌫️"
	case code >= 51 && code <= 57:
		return "🌦️"
	case code >= 61 && code <= 67:
		return "🌧️"
	case code >= 71 && code <= 77:
		return "❄️"
	case code >= 80 && code <= 82:
		return "🌧️"
	case code >= 85 && code <= 86:
		return "🌨️"
	case code >= 95:
		return "⛈️"
	}
	return "❓"
}
