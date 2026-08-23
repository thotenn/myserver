package templates

import (
	"context"
	"strings"
	"testing"

	"github.com/thotenn/myserver/internal/config"
)

func renderGroup(t *testing.T, layout *config.LayoutGroup) string {
	t.Helper()
	group := config.ServiceGroup{
		Name:     "Apps",
		Services: []config.Service{{Name: "Vikunja"}},
	}
	var sb strings.Builder
	if err := ServiceGroup(group, layout, "es", false).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

// The column count must reach the browser as a custom property, never as an
// inline `grid-template-columns`. An inline style beats every media query, so
// the second form forced the desktop column count onto phones: four ~85px
// cards whose name, status line and CPU/MEM/RX/TX row all overflowed the card.
func TestServiceGroup_ColumnsTravelAsCustomProperty(t *testing.T) {
	html := renderGroup(t, &config.LayoutGroup{Columns: 4})

	if !strings.Contains(html, `style="--service-cols: repeat(4, minmax(0, 1fr));"`) {
		t.Errorf("expected the column count as an inline --service-cols property, got:\n%s", html)
	}
	if strings.Contains(html, "grid-template-columns") {
		t.Errorf("inline grid-template-columns is back; it overrides the responsive breakpoints:\n%s", html)
	}
	if !strings.Contains(html, `class="service-grid"`) {
		t.Errorf("the grid must carry .service-grid, which is where the breakpoints live:\n%s", html)
	}
}

// A group with no layout entry emits no inline style at all, so the fallback
// baked into `.service-grid` at the `lg` breakpoint applies.
func TestServiceGroup_NoLayoutEmitsNoColumns(t *testing.T) {
	for name, layout := range map[string]*config.LayoutGroup{
		"nil layout":  nil,
		"zero column": {Columns: 0},
	} {
		t.Run(name, func(t *testing.T) {
			html := renderGroup(t, layout)
			if !strings.Contains(html, `style=""`) {
				t.Errorf("expected an empty style attribute, got:\n%s", html)
			}
			if strings.Contains(html, "--service-cols") {
				t.Errorf("no layout entry must not emit a column count:\n%s", html)
			}
		})
	}
}
