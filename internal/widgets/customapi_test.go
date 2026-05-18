package widgets

import (
	"reflect"
	"testing"
)

func TestGetValue(t *testing.T) {
	data := map[string]interface{}{
		"cpu": map[string]interface{}{
			"usage": 45.5,
			"cores": 4,
		},
		"memory": []interface{}{
			map[string]interface{}{"used": 1024},
			map[string]interface{}{"used": 2048},
		},
	}

	tests := []struct {
		path     string
		expected interface{}
	}{
		{"cpu.usage", 45.5},
		{"cpu.cores", 4},
		{"memory.0.used", 1024},
		{"memory.1.used", 2048},
		{"missing", nil},
		{"cpu.missing", nil},
		{"memory.99.used", nil},
		{"", data},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := GetValue(data, tc.path)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("GetValue(%q) = %v, want %v", tc.path, got, tc.expected)
			}
		})
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		value  interface{}
		format string
		want   string
	}{
		{1024, "bytes", "1.00 KiB"},
		{1073741824, "bytes", "1.00 GiB"},
		{60, "duration", "1m0s"},
		{50.0, "percent", "50.0%"},
		{42, "number", "42"},
		{42.5, "number", "42.50"},
		{"hello", "default", "hello"},
		{nil, "bytes", ""},
		{"2023-01-15T10:30:00Z", "date", "2023-01-15 10:30"},
	}

	for _, tc := range tests {
		t.Run(tc.format, func(t *testing.T) {
			got := FormatValue(tc.value, tc.format)
			if got != tc.want {
				t.Errorf("FormatValue(%v, %q) = %q, want %q", tc.value, tc.format, got, tc.want)
			}
		})
	}
}

func TestProcessDisplay_Graph(t *testing.T) {
	data := map[string]interface{}{
		"data": []interface{}{
			map[string]interface{}{"name": "A", "val": 10},
			map[string]interface{}{"name": "B", "val": 20},
		},
	}
	mappings := map[string]interface{}{
		"items": "data",
		"name":  "name",
		"value": "val",
	}

	result, err := ProcessDisplay(data, "graph", mappings)
	if err != nil {
		t.Fatalf("ProcessDisplay error: %v", err)
	}
	graph, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	labels, _ := graph["labels"].([]string)
	values, _ := graph["values"].([]float64)
	if len(labels) != 2 || labels[0] != "A" || labels[1] != "B" {
		t.Errorf("unexpected labels: %v", labels)
	}
	if len(values) != 2 || values[0] != 10 || values[1] != 20 {
		t.Errorf("unexpected values: %v", values)
	}
}

func TestProcessDisplay_Default(t *testing.T) {
	data := map[string]interface{}{"foo": "bar"}
	result, err := ProcessDisplay(data, "", nil)
	if err != nil {
		t.Fatalf("ProcessDisplay error: %v", err)
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["foo"] != "bar" {
		t.Error("expected passthrough for default display")
	}
}
