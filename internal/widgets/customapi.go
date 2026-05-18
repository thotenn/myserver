package widgets

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// CustomAPIWidget is the flexible customapi widget definition.
type CustomAPIWidget struct {
	BaseWidget
}

// NewCustomAPIWidget creates a new customapi widget.
func NewCustomAPIWidget() *CustomAPIWidget {
	return &CustomAPIWidget{
		BaseWidget: BaseWidget{
			TypeName: "customapi",
			API:      "{url}",
		},
	}
}

// GetValue extracts a value from nested data using dot-separated fieldPath.
// Supports map[string]interface{}, []interface{}, and other string-keyed maps
// via reflection. Returns nil if the path cannot be resolved.
func GetValue(data interface{}, fieldPath string) interface{} {
	if fieldPath == "" {
		return data
	}
	parts := strings.Split(fieldPath, ".")
	current := data
	for _, part := range parts {
		if current == nil {
			return nil
		}
		switch v := current.(type) {
		case map[string]interface{}:
			current = v[part]
		case []interface{}:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil
			}
			current = v[idx]
		default:
			rv := reflect.ValueOf(v)
			if rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
				val := rv.MapIndex(reflect.ValueOf(part))
				if !val.IsValid() {
					return nil
				}
				current = val.Interface()
			} else {
				return nil
			}
		}
	}
	return current
}

// FormatValue formats a value according to the given format string.
// Supported formats: "number", "bytes", "duration", "percent", "date", "default".
func FormatValue(value interface{}, format string) string {
	if value == nil {
		return ""
	}
	switch format {
	case "bytes":
		return formatBytes(value)
	case "duration":
		return formatDuration(value)
	case "percent":
		return formatPercent(value)
	case "number":
		return formatNumber(value)
	case "date":
		return formatDate(value)
	default:
		return stringify(value)
	}
}

func formatBytes(v interface{}) string {
	n := toFloat64(v)
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GiB", n/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.2f MiB", n/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.2f KiB", n/(1<<10))
	default:
		return fmt.Sprintf("%.0f B", n)
	}
}

func formatDuration(v interface{}) string {
	n := toFloat64(v)
	if n == 0 {
		return "0s"
	}
	d := time.Duration(n * float64(time.Second))
	return d.Round(time.Second).String()
}

func formatPercent(v interface{}) string {
	return fmt.Sprintf("%.1f%%", toFloat64(v))
}

func formatNumber(v interface{}) string {
	f := toFloat64(v)
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', 2, 64)
}

func formatDate(v interface{}) string {
	switch t := v.(type) {
	case string:
		parsed, err := time.Parse(time.RFC3339, t)
		if err == nil {
			return parsed.Format("2006-01-02 15:04")
		}
		return t
	default:
		return stringify(v)
	}
}

func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int8:
		return float64(n)
	case int16:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case uint:
		return float64(n)
	case uint8:
		return float64(n)
	case uint16:
		return float64(n)
	case uint32:
		return float64(n)
	case uint64:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	case bool:
		if n {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func stringify(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

// ProcessDisplay dispatches raw API response to the appropriate format for
// the requested display mode. Supported modes: "text" (default), "dynamic-list",
// "graph".
func ProcessDisplay(data interface{}, display string, mappings interface{}) (interface{}, error) {
	switch strings.ToLower(display) {
	case "dynamic-list":
		// Passthrough; the HTTP handler renders HTML using mappings.
		return data, nil
	case "graph":
		return extractGraphData(data, mappings)
	default:
		return data, nil
	}
}

func extractGraphData(data, mappings interface{}) (interface{}, error) {
	m, ok := mappings.(map[string]interface{})
	if !ok {
		return data, nil
	}
	itemsKey, _ := m["items"].(string)
	nameKey, _ := m["name"].(string)
	valueKey, _ := m["value"].(string)

	if itemsKey == "" {
		itemsKey = "data"
	}
	if nameKey == "" {
		nameKey = "name"
	}
	if valueKey == "" {
		valueKey = "value"
	}

	var rawItems []interface{}
	if obj, ok := data.(map[string]interface{}); ok {
		if items, ok := obj[itemsKey].([]interface{}); ok {
			rawItems = items
		}
	}
	if rawItems == nil {
		// Try top-level array
		rawItems, _ = data.([]interface{})
	}

	labels := make([]string, 0, len(rawItems))
	values := make([]float64, 0, len(rawItems))

	for _, raw := range rawItems {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		label := stringify(GetValue(item, nameKey))
		val := toFloat64(GetValue(item, valueKey))
		labels = append(labels, label)
		values = append(values, val)
	}

	return map[string]interface{}{
		"labels": labels,
		"values": values,
	}, nil
}

// RegisterCustomAPI registers the customapi widget.
func RegisterCustomAPI(r *Registry) {
	r.Register(NewCustomAPIWidget())
}
