// This file is PC-136's own addition: GET /catalog/services exposes the merged
// provider registry's own real node mappings (resource_type, node_type,
// capability_level) so the canvas UI can offer the architect a real service picker
// per node — never an invented list, never a second copy of the registry data.
package server

import (
	"encoding/json"
	"net/http"
	"sort"

	"preflight/core"
	"preflight/providers"
)

// ServiceCatalogEntry is one node mapping's own real, registry-declared identity —
// deliberately excludes edge mappings (providers.ResourceMapping.IsEdgeMapping):
// an edge-only resource_type (e.g. aws_wafv2_web_acl_association) produces no IR
// node, so it is never a valid CanvasNode.ServiceID choice.
type ServiceCatalogEntry struct {
	ResourceType    string               `json:"resource_type"`
	NodeType        core.NodeType        `json:"node_type"`
	CapabilityLevel core.CapabilityLevel `json:"capability_level"`
}

// ListServiceCatalog projects registry's own node mappings into the wire shape
// above, sorted by resource_type for deterministic output (NFR-1) — registry is a Go
// map, whose iteration order is randomized by the runtime, so any caller wanting a
// stable response must sort explicitly rather than assume anything about map order.
func ListServiceCatalog(registry providers.Registry) []ServiceCatalogEntry {
	entries := make([]ServiceCatalogEntry, 0, len(registry))
	for _, mapping := range registry {
		if mapping.IsEdgeMapping() || mapping.IngestOnly {
			continue
		}
		entries = append(entries, ServiceCatalogEntry{
			ResourceType:    mapping.ResourceType,
			NodeType:        mapping.NodeType,
			CapabilityLevel: mapping.CapabilityLevel,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ResourceType < entries[j].ResourceType })
	return entries
}

// ListServiceCatalogHandler serves GET /catalog/services.
func ListServiceCatalogHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		registry, err := loadMergedRegistry()
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ListServiceCatalog(registry))
	}
}
