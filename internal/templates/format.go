package templates

import "fmt"

// FormatBytes formats a byte count as a human-readable string.
func FormatBytes(bytes float64) string {
	const (
		KB = 1024.0
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1f TB", bytes/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", bytes/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", bytes/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", bytes/KB)
	default:
		return fmt.Sprintf("%.0f B", bytes)
	}
}

// FormatPercent formats a float as a percentage string.
func FormatPercent(value float64) string {
	return fmt.Sprintf("%.1f%%", value)
}

// FormatDuration formats seconds as a human-readable duration.
func FormatDuration(seconds float64) string {
	switch {
	case seconds >= 86400:
		return fmt.Sprintf("%dd", int(seconds/86400))
	case seconds >= 3600:
		return fmt.Sprintf("%dh", int(seconds/3600))
	case seconds >= 60:
		return fmt.Sprintf("%dm", int(seconds/60))
	default:
		return fmt.Sprintf("%ds", int(seconds))
	}
}

// FormatLatency formats a millisecond latency as a short human string.
func FormatLatency(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}

// FormatStatusCode renders an HTTP status code as a string.
func FormatStatusCode(status int) string {
	if status <= 0 {
		return "ERR"
	}
	return fmt.Sprintf("%d", status)
}

// FormatTemp renders a celsius temperature as a human-readable string.
func FormatTemp(c float64) string {
	return fmt.Sprintf("%.0f°C", c)
}
