package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/thotenn/myserver/internal/templates"
)

// ResourcesWidget handles GET /api/widgets/resources.
// Reads /proc/stat, /proc/meminfo, /proc/uptime and statfs to produce a
// list of ResourceBarData suitable for the frontend. Falls back to
// runtime.NumCPU / Go-level metrics on non-Linux hosts.
func ResourcesWidget(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	label := q.Get("label")

	var bars []templates.ResourceBarData

	if q.Get("cpu") == "true" {
		cpuPct := readCPUPercent()
		name := label
		if name == "" {
			name = "CPU"
		}
		bars = append(bars, templates.ResourceBarData{
			Label:    name,
			Percent:  cpuPct,
			Value:    templates.FormatPercent(cpuPct),
			BarColor: cpuBarColor(cpuPct),
		})
		if q.Get("cputemp") == "true" {
			if t := readCPUTemp(); t > 0 {
				bars = append(bars, templates.ResourceBarData{
					Label:    "TEMP",
					Percent:  clamp(t, 0, 100),
					Value:    templates.FormatTemp(t),
					BarColor: tempBarColor(t),
				})
			}
		}
		if q.Get("uptime") == "true" {
			if up := readUptime(); up > 0 {
				bars = append(bars, templates.ResourceBarData{
					Label:    "UP",
					Percent:  0,
					Value:    templates.FormatDuration(up),
					BarColor: "bg-theme-400",
				})
			}
		}
	}

	if q.Get("memory") == "true" {
		used, total := readMemory()
		pct := 0.0
		if total > 0 {
			pct = float64(used) / float64(total) * 100
		}
		name := label
		if name == "" {
			name = "RAM"
		}
		bars = append(bars, templates.ResourceBarData{
			Label:    name,
			Percent:  pct,
			Value:    fmt.Sprintf("%s/%s", templates.FormatBytes(float64(used)), templates.FormatBytes(float64(total))),
			BarColor: memBarColor(pct),
		})
	}

	if disk := q.Get("disk"); disk != "" {
		used, total := readDisk(disk)
		pct := 0.0
		if total > 0 {
			pct = float64(used) / float64(total) * 100
		}
		name := label
		if name == "" {
			name = "DISK"
		}
		bars = append(bars, templates.ResourceBarData{
			Label:    name,
			Percent:  pct,
			Value:    fmt.Sprintf("%s/%s", templates.FormatBytes(float64(used)), templates.FormatBytes(float64(total))),
			BarColor: diskBarColor(pct),
		})
	}

	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.ResourcesHTML(bars).Render(r.Context(), w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"bars": bars})
}

// ---------------------------------------------------------------------
// /proc readers (linux-only, return zeros on other platforms)
// ---------------------------------------------------------------------

// lastCPUSample keeps the previous cpu jiffies snapshot so we can compute
// the delta-based utilisation without blocking the handler.
var lastCPUSample struct {
	total, idle uint64
	at          time.Time
}

func readCPUPercent() float64 {
	if runtime.GOOS != "linux" {
		return 0
	}
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return 0
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0
	}
	var values []uint64
	for _, v := range fields[1:] {
		n, _ := strconv.ParseUint(v, 10, 64)
		values = append(values, n)
	}
	if len(values) < 4 {
		return 0
	}
	idle := values[3]
	var total uint64
	for _, v := range values {
		total += v
	}
	prevTotal := lastCPUSample.total
	prevIdle := lastCPUSample.idle
	lastCPUSample.total = total
	lastCPUSample.idle = idle
	lastCPUSample.at = time.Now()
	if prevTotal == 0 {
		return 0
	}
	totalDelta := total - prevTotal
	idleDelta := idle - prevIdle
	if totalDelta == 0 {
		return 0
	}
	used := float64(totalDelta-idleDelta) / float64(totalDelta) * 100
	return used
}

func readCPUTemp() float64 {
	if runtime.GOOS != "linux" {
		return 0
	}
	// Try the common thermal_zone0 sensor
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0
	}
	return float64(n) / 1000.0
}

func readUptime() float64 {
	if runtime.GOOS != "linux" {
		return 0
	}
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0
	}
	v, _ := strconv.ParseFloat(fields[0], 64)
	return v
}

func readMemory() (used, total uint64) {
	if runtime.GOOS != "linux" {
		return 0, 0
	}
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var memTotal, memAvailable uint64
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			memTotal = parseMemKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			memAvailable = parseMemKB(line)
		}
	}
	total = memTotal
	if memAvailable > 0 && memTotal >= memAvailable {
		used = memTotal - memAvailable
	}
	return used, total
}

func parseMemKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	n, _ := strconv.ParseUint(fields[1], 10, 64)
	return n * 1024 // kB → bytes
}

func readDisk(path string) (used, total uint64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0
	}
	total = stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	if total > free {
		used = total - free
	}
	return used, total
}

// ---------------------------------------------------------------------
// Bar colours
// ---------------------------------------------------------------------

func cpuBarColor(pct float64) string {
	if pct > 85 {
		return "bg-red-500"
	}
	if pct > 60 {
		return "bg-yellow-500"
	}
	return "bg-blue-500"
}

func memBarColor(pct float64) string {
	if pct > 90 {
		return "bg-red-500"
	}
	if pct > 75 {
		return "bg-yellow-500"
	}
	return "bg-green-500"
}

func diskBarColor(pct float64) string {
	if pct > 90 {
		return "bg-red-500"
	}
	if pct > 75 {
		return "bg-yellow-500"
	}
	return "bg-green-500"
}

func tempBarColor(t float64) string {
	if t > 80 {
		return "bg-red-500"
	}
	if t > 60 {
		return "bg-yellow-500"
	}
	return "bg-green-500"
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
