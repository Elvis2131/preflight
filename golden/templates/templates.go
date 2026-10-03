// Package templates is PC-108's reference-architecture template library. A template is
// DATA — a checked-in canvas document, an inline workload, and a UI-only layout — loaded
// through the exact same canvas → IR pipeline as a hand-drawn design: there is no
// template-specific code path anywhere (server.AssessCanvas never learns a template was
// involved). The expected IR, findings and a failure-mode result sit beside each input,
// derived from a real run by cmd/gen-template-fixtures (never hand-written, same rule as
// PC-15's golden fixtures) and byte-compared by this package's test.
//
// Templates may include explicit architecture intent for services the registry does
// not yet model. Those identities stay unresolved for capability checks; their metadata
// and design guide distinguish the blueprint from simulated behaviour.
package templates

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"

	"preflight/core"
)

//go:embed */*.json
var files embed.FS

// Meta is a template's listing entry.
type Meta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Layout is UI-only node placement (id → x,y). It is deliberately NOT part of the frozen
// canvas contract: canvas.schema.json carries no coordinates, and a template's layout
// must never become a verdict input.
type Layout map[string]struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	// Width/Height are set only for a container (VPC/subnet) drawn larger than the
	// workspace's default container size.
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
	// Optional positions for the clean service view. The full infrastructure layout
	// and this service layout are presentation only, never assessment inputs.
	ServiceX *float64 `json:"service_x,omitempty"`
	ServiceY *float64 `json:"service_y,omitempty"`
}

// Template is one loadable template: what the workspace needs to put it on the canvas.
type Template struct {
	Meta     Meta                `json:"meta"`
	Canvas   core.CanvasDocument `json:"canvas"`
	Workload core.Workload       `json:"workload"`
	Layout   Layout              `json:"layout"`
}

// IDs returns every template ID, sorted (NFR-1).
func IDs() ([]string, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// Raw returns one of a template's checked-in files (canvas, workload, layout, meta, ir,
// findings, failure) exactly as stored — the bytes the fixture test compares.
func Raw(id, name string) ([]byte, error) {
	return files.ReadFile(id + "/" + name + ".json")
}

func read(id, name string, into any) error {
	b, err := Raw(id, name)
	if err != nil {
		return fmt.Errorf("template %q: %w", id, err)
	}
	if err := json.Unmarshal(b, into); err != nil {
		return fmt.Errorf("template %q %s.json: %w", id, name, err)
	}
	return nil
}

// Load reads one template's input files.
func Load(id string) (Template, error) {
	var t Template
	if err := read(id, "meta", &t.Meta); err != nil {
		return t, err
	}
	if err := read(id, "canvas", &t.Canvas); err != nil {
		return t, err
	}
	if err := read(id, "workload", &t.Workload); err != nil {
		return t, err
	}
	if err := read(id, "layout", &t.Layout); err != nil {
		return t, err
	}
	return t, nil
}

// List returns every template's Meta, sorted by ID.
func List() ([]Meta, error) {
	ids, err := IDs()
	if err != nil {
		return nil, err
	}
	out := make([]Meta, 0, len(ids))
	for _, id := range ids {
		var m Meta
		if err := read(id, "meta", &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}
