package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// BookmarkGroup represents a named group of bookmarks.
type BookmarkGroup struct {
	Name      string     `yaml:"name" json:"name"`
	Bookmarks []Bookmark `yaml:"bookmarks" json:"bookmarks"`
}

// Bookmark represents a single bookmark entry.
type Bookmark struct {
	Name string `yaml:"name" json:"name"`
	Abbr string `yaml:"abbr,omitempty" json:"abbr,omitempty"`
	Href string `yaml:"href" json:"href"`
	Icon string `yaml:"icon,omitempty" json:"icon,omitempty"`
}

func loadBookmarks(dir string) ([]BookmarkGroup, error) {
	data, err := readConfigFile(dir, "bookmarks.yaml")
	if err != nil {
		return nil, err
	}

	// bookmarks.yaml is a list where each item is {groupName: [{bmName: {bmFields}}]}
	var rawGroups []map[string][]map[string]Bookmark
	if err := yaml.Unmarshal(data, &rawGroups); err != nil {
		return nil, fmt.Errorf("parsing bookmarks.yaml: %w", err)
	}

	var groups []BookmarkGroup
	for _, rawGroup := range rawGroups {
		for groupName, bmList := range rawGroup {
			group := BookmarkGroup{
				Name: groupName,
			}
			for _, bmMap := range bmList {
				for bmName, bm := range bmMap {
					bm.Name = bmName
					group.Bookmarks = append(group.Bookmarks, bm)
				}
			}
			groups = append(groups, group)
		}
	}

	return groups, nil
}
