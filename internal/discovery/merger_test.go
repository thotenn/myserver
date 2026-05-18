package discovery

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thotenn/myserver/internal/config"
)

func TestMergeServices(t *testing.T) {
	configGroups := []config.ServiceGroup{
		{
			Name: "Apps",
			Services: []config.Service{
				{Name: "Wiki", Href: "https://wiki.example.com", Icon: "mdi:wiki", Weight: 10},
				{Name: "Blog", Href: "https://blog.example.com", Icon: "mdi:post", Weight: 20},
			},
		},
		{
			Name: "Infra",
			Services: []config.Service{
				{Name: "Router", Href: "https://router.local", Weight: 5},
			},
		},
	}

	dockerGroups := []config.ServiceGroup{
		{
			Name: "Apps",
			Services: []config.Service{
				{Name: "Wiki", Container: "wiki-container", ShowStats: true},
				{Name: "NewService", Container: "new-container", ShowStats: true},
			},
		},
	}

	layout := map[string]config.LayoutGroup{
		"Infra": {Columns: 2},
		"Apps":  {Columns: 3},
	}

	merged := MergeServices(configGroups, dockerGroups, layout)

	require.Len(t, merged, 2)

	// Layout uses alphabetical ordering for the map keys (deterministic),
	// so "Apps" comes before "Infra".
	apps := findGroupByName(merged, "Apps")
	infra := findGroupByName(merged, "Infra")
	require.NotNil(t, apps)
	require.NotNil(t, infra)
	require.Len(t, apps.Services, 3) // Wiki (merged) + Blog + NewService
	require.Len(t, infra.Services, 1)
	assert.Equal(t, "Router", infra.Services[0].Name)

	// Wiki should have config fields merged with Docker container name
	wiki := findServiceByName(apps.Services, "Wiki")
	require.NotNil(t, wiki)
	assert.Equal(t, "https://wiki.example.com", wiki.Href)
	assert.Equal(t, "mdi:wiki", wiki.Icon)
	assert.Equal(t, "wiki-container", wiki.Container)
	assert.Equal(t, 10, wiki.Weight)

	// NewService from Docker only
	newSvc := findServiceByName(apps.Services, "NewService")
	require.NotNil(t, newSvc)
	assert.Equal(t, "new-container", newSvc.Container)

	// Services sorted by weight (0 first, then 10, then 20)
	assert.Equal(t, "NewService", apps.Services[0].Name) // weight 0 (default)
	assert.Equal(t, "Wiki", apps.Services[1].Name)       // weight 10
	assert.Equal(t, "Blog", apps.Services[2].Name)       // weight 20
}

func findGroupByName(groups []config.ServiceGroup, name string) *config.ServiceGroup {
	for i := range groups {
		if groups[i].Name == name {
			return &groups[i]
		}
	}
	return nil
}

func TestMergeServicesEmptyDocker(t *testing.T) {
	configGroups := []config.ServiceGroup{
		{
			Name: "Apps",
			Services: []config.Service{
				{Name: "Wiki", Href: "https://wiki.example.com"},
			},
		},
	}

	merged := MergeServices(configGroups, nil, nil)
	require.Len(t, merged, 1)
	assert.Equal(t, "Apps", merged[0].Name)
	require.Len(t, merged[0].Services, 1)
	assert.Equal(t, "Wiki", merged[0].Services[0].Name)
}

func TestSortServices(t *testing.T) {
	services := []config.Service{
		{Name: "Charlie", Weight: 30},
		{Name: "Alpha", Weight: 10},
		{Name: "Bravo", Weight: 20},
		{Name: "Delta", Weight: 10},
	}

	sortServices(services)

	assert.Equal(t, "Alpha", services[0].Name)   // weight 10, name Alpha
	assert.Equal(t, "Delta", services[1].Name)   // weight 10, name Delta
	assert.Equal(t, "Bravo", services[2].Name)   // weight 20
	assert.Equal(t, "Charlie", services[3].Name) // weight 30
}

func TestContainerToService(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected *config.Service
	}{
		{
			name:     "no homepage labels",
			labels:   map[string]string{},
			expected: nil,
		},
		{
			name: "minimal service",
			labels: map[string]string{
				"homepage.name": "MyService",
			},
			expected: &config.Service{
				Name:      "MyService",
				ShowStats: true,
			},
		},
		{
			name: "full service",
			labels: map[string]string{
				"homepage.name":        "FullService",
				"homepage.href":        "https://example.com",
				"homepage.icon":        "mdi:web",
				"homepage.description": "A full service",
				"homepage.group":       "MyGroup",
				"homepage.ping":        "https://example.com/ping",
				"homepage.weight":      "50",
			},
			expected: &config.Service{
				Name:        "FullService",
				Href:        "https://example.com",
				Icon:        "mdi:web",
				Description: "A full service",
				Ping:        "https://example.com/ping",
				Weight:      50,
				ShowStats:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := types.Container{Labels: tt.labels, Names: []string{"/test-container"}}
			result := containerToService(c)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.Name, result.Name)
				assert.Equal(t, tt.expected.Href, result.Href)
				assert.Equal(t, tt.expected.Icon, result.Icon)
				assert.Equal(t, tt.expected.Description, result.Description)
				assert.Equal(t, tt.expected.Weight, result.Weight)
				assert.Equal(t, "test-container", result.Container)
			}
		})
	}
}
