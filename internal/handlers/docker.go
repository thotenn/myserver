package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/go-chi/chi/v5"
	"github.com/thotenn/myserver/internal/config"
	"github.com/thotenn/myserver/internal/templates"
)

// dockerClientPool caches Docker clients keyed by server name to avoid
// creating and closing a client on every request.
var (
	dockerClientPool   = make(map[string]*client.Client)
	dockerClientPoolMu sync.RWMutex
)

// InvalidateDockerClients closes all pooled Docker clients and clears the
// pool. Called on config change so stale connections are recreated.
func InvalidateDockerClients() {
	dockerClientPoolMu.Lock()
	defer dockerClientPoolMu.Unlock()
	for _, cli := range dockerClientPool {
		_ = cli.Close()
	}
	clear(dockerClientPool)
}

// withDockerClient acquires a Docker client for server and runs fn. If the
// client cannot be obtained, it writes a generic 502 and returns false.
func withDockerClient(w http.ResponseWriter, server string, fn func(*client.Client) bool) bool {
	cli, err := getDockerClient(server)
	if err != nil {
		writeUpstreamError(w, "docker client unavailable")
		return false
	}
	return fn(cli)
}

// DockerStats handles GET /api/docker/stats/{container}/{server}.
// Renders HTML when called by HTMX (HX-Request header), JSON otherwise.
func DockerStats(w http.ResponseWriter, r *http.Request) {
	containerName := chi.URLParam(r, "container")
	server := chi.URLParam(r, "server")

	if containerName == "" {
		http.Error(w, "missing container name", http.StatusBadRequest)
		return
	}

	ok := withDockerClient(w, server, func(cli *client.Client) bool {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		resp, err := cli.ContainerStats(ctx, containerName, false)
		if err != nil {
			if errdefs.IsNotFound(err) {
				// Return 200 so HTMX swaps — see rationale in DockerStatus.
				if isHTMXRequest(r) {
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					_ = templates.DockerStatsPlaceholder().Render(r.Context(), w)
					return true
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "notfound"})
				return true
			}
			writeUpstreamError(w, "stats unavailable")
			return true
		}
		if resp.Body == nil {
			writeUpstreamError(w, "empty stats body")
			return true
		}
		defer resp.Body.Close()

		// Bound the read to avoid OOM if the daemon misbehaves.
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			writeUpstreamError(w, "stats read failed")
			return true
		}

		// NOTE on JSON tags: the Docker engine reports system-wide CPU time as
		// `system_cpu_usage` (NOT `system_usage`). An earlier version of this
		// handler used the wrong tag, so SystemUsage always decoded to 0 and
		// every container reported 0.0% CPU.
		var stats struct {
			CPUStats struct {
				CPUUsage struct {
					TotalUsage  uint64   `json:"total_usage"`
					PercpuUsage []uint64 `json:"percpu_usage"`
				} `json:"cpu_usage"`
				SystemUsage uint64 `json:"system_cpu_usage"`
				OnlineCPUs  uint32 `json:"online_cpus"`
			} `json:"cpu_stats"`
			PreCPUStats struct {
				CPUUsage struct {
					TotalUsage uint64 `json:"total_usage"`
				} `json:"cpu_usage"`
				SystemUsage uint64 `json:"system_cpu_usage"`
			} `json:"precpu_stats"`
			MemoryStats struct {
				Usage uint64            `json:"usage"`
				Limit uint64            `json:"limit"`
				Stats map[string]uint64 `json:"stats"`
			} `json:"memory_stats"`
			Networks map[string]struct {
				RxBytes uint64 `json:"rx_bytes"`
				TxBytes uint64 `json:"tx_bytes"`
			} `json:"networks"`
		}

		if err := json.Unmarshal(body, &stats); err != nil {
			writeUpstreamError(w, "stats parse failed")
			return true
		}

		// Calculate CPU percentage. CGroup v2 provides OnlineCPUs; fall back to
		// the legacy PercpuUsage slice when OnlineCPUs is zero.
		cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)
		cpuCount := float64(stats.CPUStats.OnlineCPUs)
		if cpuCount == 0 {
			cpuCount = float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
		}
		if cpuCount == 0 {
			cpuCount = 1
		}
		cpuPercent := 0.0
		if systemDelta > 0 && cpuDelta > 0 {
			cpuPercent = (cpuDelta / systemDelta) * cpuCount * 100.0
		}

		// Match `docker stats`: strip the inactive page-cache from memory usage.
		// Without this the value is dominated by file cache that doesn't change
		// with the container's workload and looks frozen in the UI.
		memUsage := stats.MemoryStats.Usage
		if cache, ok := stats.MemoryStats.Stats["inactive_file"]; ok && cache < memUsage {
			memUsage -= cache
		} else if cache, ok := stats.MemoryStats.Stats["total_inactive_file"]; ok && cache < memUsage {
			memUsage -= cache
		}

		var rx, tx float64
		for _, nw := range stats.Networks {
			rx += float64(nw.RxBytes)
			tx += float64(nw.TxBytes)
		}

		if isHTMXRequest(r) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = templates.DockerStatsHTML(cpuPercent, memUsage, stats.MemoryStats.Limit, rx, tx).Render(r.Context(), w)
			return true
		}

		result := map[string]interface{}{
			"stats": map[string]interface{}{
				"cpu": cpuPercent,
				"memory": map[string]interface{}{
					"usage": float64(memUsage),
					"limit": float64(stats.MemoryStats.Limit),
				},
				"network": map[string]interface{}{
					"rx": rx,
					"tx": tx,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
		return true
	})
	_ = ok
}

// DockerStatus handles GET /api/docker/status/{container}/{server}.
func DockerStatus(w http.ResponseWriter, r *http.Request) {
	containerName := chi.URLParam(r, "container")
	server := chi.URLParam(r, "server")

	if containerName == "" {
		http.Error(w, "missing container name", http.StatusBadRequest)
		return
	}

	ok := withDockerClient(w, server, func(cli *client.Client) bool {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		containerJSON, err := cli.ContainerInspect(ctx, containerName)
		if err != nil {
			if errdefs.IsNotFound(err) {
				// Return 200 with a "notfound" status so HTMX swaps the
				// placeholder cleanly. A 404 makes HTMX skip the swap, which
				// leaves a previously-rendered "running" badge visible forever
				// after the real container is removed or renamed.
				if isHTMXRequest(r) {
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					_ = templates.DockerStatusHTML("notfound", "").Render(r.Context(), w)
					return true
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "notfound"})
				return true
			}
			writeUpstreamError(w, "inspect failed")
			return true
		}

		status := containerJSON.State.Status
		health := ""
		if containerJSON.State.Health != nil {
			health = containerJSON.State.Health.Status
		}

		if isHTMXRequest(r) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = templates.DockerStatusHTML(status, health).Render(r.Context(), w)
			return true
		}

		result := map[string]interface{}{"status": status}
		if health != "" {
			result["health"] = health
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
		return true
	})
	_ = ok
}

// isHTMXRequest reports whether the request was issued by HTMX.
func isHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// writeUpstreamError writes a 502 with a generic body, hiding internal
// detail. Use for unrecoverable upstream errors so the client doesn't see
// stack traces or filesystem paths.
func writeUpstreamError(w http.ResponseWriter, msg string) {
	http.Error(w, msg, http.StatusBadGateway)
}

// getDockerClient returns a pooled Docker client for the given server name.
// Clients are reused across requests and only recreated when the pool is
// invalidated.
func getDockerClient(server string) (*client.Client, error) {
	dockerClientPoolMu.RLock()
	cli, ok := dockerClientPool[server]
	dockerClientPoolMu.RUnlock()
	if ok {
		return cli, nil
	}

	// The root dashboard's, explicitly: the docker client pool talks to the
	// host's daemon and none of the /api/docker routes is registered for a
	// client dashboard, so there is no per-request dashboard to read here.
	dockerConfigs, err := config.Dashboards().Root().Docker()
	if err != nil {
		return nil, fmt.Errorf("loading docker config: %w", err)
	}

	var cfg config.DockerConfig
	if server == "" {
		if len(dockerConfigs) > 0 {
			for _, c := range dockerConfigs {
				cfg = c
				break
			}
		} else {
			return client.NewClientWithOpts(client.FromEnv)
		}
	} else {
		var found bool
		cfg, found = dockerConfigs[server]
		if !found {
			return nil, errors.New("docker server not configured")
		}
	}

	cli, err = createDockerClient(cfg)
	if err != nil {
		return nil, err
	}

	dockerClientPoolMu.Lock()
	dockerClientPool[server] = cli
	dockerClientPoolMu.Unlock()
	return cli, nil
}

func createDockerClient(cfg config.DockerConfig) (*client.Client, error) {
	var opts []client.Opt

	if cfg.Socket != "" {
		opts = append(opts, client.WithHost("unix://"+cfg.Socket))
	} else if cfg.Host != "" {
		host := cfg.Host
		if cfg.Port > 0 {
			host = fmt.Sprintf("%s:%d", host, cfg.Port)
		}
		proto := "tcp"
		if cfg.TLS != nil {
			proto = "https"
		}
		opts = append(opts, client.WithHost(fmt.Sprintf("%s://%s", proto, host)))
	} else {
		opts = append(opts, client.FromEnv)
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}
	cli.NegotiateAPIVersion(context.Background())
	return cli, nil
}
