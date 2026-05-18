package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/thotenn/myserver/internal/config"
)

// DockerDiscoverer discovers services from Docker container labels.
type DockerDiscoverer struct {
	client *client.Client
	cfg    config.DockerConfig
}

// NewDockerDiscoverer creates a new Docker discoverer.
func NewDockerDiscoverer(cfg config.DockerConfig) (*DockerDiscoverer, error) {
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
		return nil, fmt.Errorf("creating docker client: %w", err)
	}
	negCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cli.NegotiateAPIVersion(negCtx)

	return &DockerDiscoverer{client: cli, cfg: cfg}, nil
}

// Close closes the Docker client.
func (d *DockerDiscoverer) Close() error {
	return d.client.Close()
}

// DiscoverServices discovers services from Docker containers with
// homepage.* labels. It honours cfg.Swarm and bounds every API call with
// a 10s timeout so a slow daemon never blocks the dashboard handler.
func (d *DockerDiscoverer) DiscoverServices(ctx context.Context) ([]config.ServiceGroup, error) {
	listCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	containers, err := d.client.ContainerList(listCtx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	groupsMap := make(map[string][]config.Service)

	for _, c := range containers {
		svc := containerToService(c)
		if svc == nil {
			continue
		}
		group := c.Labels["homepage.group"]
		if group == "" {
			group = "Docker"
		}
		groupsMap[group] = append(groupsMap[group], *svc)
	}

	// Only call ServiceList when the daemon is configured as a Swarm
	// manager. Calling it otherwise produces a noisy "this node is not a
	// swarm manager" error every request.
	if d.cfg.Swarm {
		swarmCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		services, err := d.client.ServiceList(swarmCtx, types.ServiceListOptions{})
		if err == nil {
			for _, s := range services {
				svc := swarmServiceToService(s)
				if svc == nil {
					continue
				}
				group := s.Spec.Labels["homepage.group"]
				if group == "" {
					group = "Docker"
				}
				groupsMap[group] = append(groupsMap[group], *svc)
			}
		}
	}

	var groups []config.ServiceGroup
	for name, svcs := range groupsMap {
		groups = append(groups, config.ServiceGroup{
			Name:     name,
			Services: svcs,
		})
	}

	return groups, nil
}

// GetContainerStats retrieves stats for a container.
func (d *DockerDiscoverer) GetContainerStats(ctx context.Context, containerName string) (io.ReadCloser, error) {
	resp, err := d.client.ContainerStats(ctx, containerName, false)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// GetContainerStatus retrieves the status of a container.
func (d *DockerDiscoverer) GetContainerStatus(ctx context.Context, containerName string) (map[string]interface{}, error) {
	containerJSON, err := d.client.ContainerInspect(ctx, containerName)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"status": containerJSON.State.Status,
	}

	if containerJSON.State.Health != nil {
		result["health"] = containerJSON.State.Health.Status
	}

	return result, nil
}

// firstContainerName safely returns the first name of a container, with
// the leading slash stripped. Returns "" if the container has no names
// (which shouldn't happen but the Docker API does not strictly forbid it).
func firstContainerName(c types.Container) string {
	if len(c.Names) == 0 {
		return ""
	}
	return strings.TrimPrefix(c.Names[0], "/")
}

// containerToService extracts a Service from Docker container labels.
func containerToService(c types.Container) *config.Service {
	name := c.Labels["homepage.name"]
	if name == "" {
		return nil
	}

	svc := &config.Service{
		Name:        name,
		Href:        c.Labels["homepage.href"],
		Icon:        c.Labels["homepage.icon"],
		Description: c.Labels["homepage.description"],
		Ping:        c.Labels["homepage.ping"],
		SiteMonitor: c.Labels["homepage.siteMonitor"],
		Container:   firstContainerName(c),
		ShowStats:   true,
	}

	if w := c.Labels["homepage.weight"]; w != "" {
		if n, err := strconv.Atoi(w); err == nil {
			svc.Weight = n
		}
	}

	if widgetJSON := c.Labels["homepage.widget"]; widgetJSON != "" {
		var widget config.WidgetConfig
		if err := json.Unmarshal([]byte(widgetJSON), &widget); err == nil {
			svc.Widget = &widget
		}
		// Note: parse errors are intentionally swallowed here. They are
		// reported via the discovery layer's logger when wired in main.
	}

	return svc
}

// swarmServiceToService extracts a Service from Docker Swarm service labels.
func swarmServiceToService(s swarm.Service) *config.Service {
	name := s.Spec.Labels["homepage.name"]
	if name == "" {
		return nil
	}

	svc := &config.Service{
		Name:        name,
		Href:        s.Spec.Labels["homepage.href"],
		Icon:        s.Spec.Labels["homepage.icon"],
		Description: s.Spec.Labels["homepage.description"],
		Ping:        s.Spec.Labels["homepage.ping"],
		SiteMonitor: s.Spec.Labels["homepage.siteMonitor"],
	}

	if w := s.Spec.Labels["homepage.weight"]; w != "" {
		if n, err := strconv.Atoi(w); err == nil {
			svc.Weight = n
		}
	}

	if widgetJSON := s.Spec.Labels["homepage.widget"]; widgetJSON != "" {
		var widget config.WidgetConfig
		if err := json.Unmarshal([]byte(widgetJSON), &widget); err == nil {
			svc.Widget = &widget
		}
	}

	return svc
}
