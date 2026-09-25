// This file is PC-27: a FINOS CALM (Common Architecture Language Model) export of an
// assessed IR. See exporters/calm_schema/README.md for exactly where the vendored,
// official schema this export validates against came from — the same "don't guess the
// schema" discipline exporters/fis.go's own doc comment already established for AWS
// FIS, applied here to CALM's real, published document shape (calm_schema/core.json's
// nodes/relationships/metadata).
//
// MAPPING, recorded here rather than left implicit:
//   - core.Node -> a CALM node: UniqueID = Node.ID, NodeType = Node.Type's own string
//     value directly (CALM's node-type-definition is explicitly "any enum value OR any
//     string" — calm_schema/core.json's own def — so Preflight's real type vocabulary
//     is carried through verbatim rather than forced into CALM's own, different,
//     smaller example enum), Description = Node.Provenance.Source (a real file:line
//     citation already in the IR, not an invented human-readable blurb).
//   - a core.Edge whose Type is contained_in -> a CALM "deployed-in" relationship
//     (container: edge.To, nodes: [edge.From]) — contained_in IS physical placement
//     (a resource contained in a subnet, a subnet in a VPC), which is exactly what
//     CALM's deployed-in relationship-type names, unlike composed-of (structural
//     decomposition of one system into parts, a different claim this export never
//     makes).
//   - every other core.Edge type (depends_on, routes_to, reads/writes,
//     authenticates_via, replicates_to) -> a CALM "connects" relationship (source:
//     edge.From, destination: edge.To), with the relationship's own Description set to
//     Preflight's real EdgeType string — CALM's "connects" has no equivalent
//     sub-vocabulary of its own, so the specific edge kind is preserved as text rather
//     than silently collapsed into one undifferentiated "connects".
//
// LOSSY, ONE-WAY, STATED IN THE OUTPUT ITSELF (PC-27's second acceptance criterion,
// verbatim: not buried in a design doc): the exported document's own top-level
// "metadata" object carries a "notice" string saying plainly that Provenance's Kind/
// Reason/EvidenceTier/Rung, ResolutionState, and CapabilityModel are not represented
// here in round-trippable form — a compliance stakeholder opening this JSON file sees
// that fact directly, not by reading this Go source or a separate document. Best-
// effort, non-normative traces of Provenance.Source/Kind and ResolutionState ARE still
// attached per-node/per-relationship (CALM's own "metadata" field on both), since
// including that extra context costs nothing and does not contradict the stated
// one-way/lossy fact — it is not a promise that re-importing this file recovers them.
//
// NO CHANGES TO CORE IR (PC-27's third acceptance criterion, verbatim): this file reads
// core.IR/core.Node/core.Edge exactly as PC-11 already defined them; nothing in core/
// was added or altered to support this exporter.
package exporters

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v5"

	"preflight/core"
)

//go:embed calm_schema/core.json calm_schema/interface.json calm_schema/control.json calm_schema/flow.json
var calmSchemaFS embed.FS

// calmCoreSchemaID is the vendored schema's own $id — see calm_schema/README.md.
const calmCoreSchemaID = "https://calm.finos.org/release/1.2/meta/core.json"

// calmSchemaFiles maps each vendored file's own declared $id to its embedded path, so
// the compiler resolves core.json's relative $refs ("interface.json#/...",
// "control.json#/...", "flow.json#/...") against these local copies instead of
// reaching out to calm.finos.org over the network at validation time.
var calmSchemaFiles = map[string]string{
	calmCoreSchemaID: "calm_schema/core.json",
	"https://calm.finos.org/release/1.2/meta/interface.json": "calm_schema/interface.json",
	"https://calm.finos.org/release/1.2/meta/control.json":   "calm_schema/control.json",
	"https://calm.finos.org/release/1.2/meta/flow.json":      "calm_schema/flow.json",
}

type calmDocument struct {
	Nodes         []calmNode         `json:"nodes"`
	Relationships []calmRelationship `json:"relationships"`
	Metadata      map[string]any     `json:"metadata"`
}

type calmNode struct {
	UniqueID    string         `json:"unique-id"`
	NodeType    string         `json:"node-type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type calmRelationship struct {
	UniqueID         string               `json:"unique-id"`
	Description      string               `json:"description,omitempty"`
	RelationshipType calmRelationshipType `json:"relationship-type"`
	Metadata         map[string]any       `json:"metadata,omitempty"`
}

// calmRelationshipType carries exactly one of Connects/DeployedIn — CALM's own schema
// requires exactly one of its five relationship-type variants; this exporter only ever
// produces these two (see this file's own MAPPING doc comment above for why the other
// three — interacts, composed-of, options — have no Preflight edge type they'd
// honestly correspond to).
type calmRelationshipType struct {
	Connects   *calmConnects   `json:"connects,omitempty"`
	DeployedIn *calmDeployedIn `json:"deployed-in,omitempty"`
}

type calmConnects struct {
	Source      calmNodeInterface `json:"source"`
	Destination calmNodeInterface `json:"destination"`
}

type calmNodeInterface struct {
	Node string `json:"node"`
}

type calmDeployedIn struct {
	Container string   `json:"container"`
	Nodes     []string `json:"nodes"`
}

// calmLossNotice is PC-27's second acceptance criterion, verbatim, made literal: this
// exact string is written into every export's own metadata.notice field — see this
// file's own package doc comment for why it belongs in the output, not only here.
const calmLossNotice = "This is a one-way, lossy export of a Preflight IR into the FINOS CALM format. " +
	"Preflight's Provenance (kind/reason/evidence tier/rung), ResolutionState (known/inferred/unresolved), " +
	"and CapabilityModel are NOT round-trippable from this document — re-importing this file back into " +
	"Preflight would not recover them. Per-node/per-relationship \"metadata\" fields below carry a best-effort, " +
	"non-normative trace of provenance/resolution for human/tooling context only, not a round-trip guarantee."

// ExportCALM derives a FINOS CALM architecture document from an assessed IR. The
// returned bytes are indented JSON that has already been checked against the vendored,
// official CALM schema (calm_schema/core.json) before being returned — a mapping bug
// here should fail loudly at export time, not silently produce an export a downstream
// CALM-compatible tool would then reject.
func ExportCALM(ir *core.IR) ([]byte, error) {
	doc := calmDocument{
		// Non-nil from the start (matching core/simulate.go's own established
		// discipline: a Go nil slice marshals to JSON `null`, not `[]`, which the
		// vendored official CALM schema's own "type": "array" check on nodes/
		// relationships correctly rejects — caught for real by
		// TestExportCALM_StatesLossyNoticeInOutput's single-node, zero-edge IR before
		// this fix, not merely anticipated).
		Nodes:         make([]calmNode, 0, len(ir.Nodes)),
		Relationships: make([]calmRelationship, 0, len(ir.Edges)),
		Metadata: map[string]any{
			"notice":         calmLossNotice,
			"generator":      "Preflight",
			"schema_version": ir.SchemaVersion,
			"version_number": ir.VersionNumber,
			"version_hash":   ir.VersionHash,
		},
	}

	for _, n := range ir.Nodes {
		doc.Nodes = append(doc.Nodes, calmNode{
			UniqueID:    n.ID,
			NodeType:    string(n.Type),
			Name:        n.ID,
			Description: n.Provenance.Source,
			Metadata: map[string]any{
				"resolution":      string(n.Resolution),
				"provenance_kind": string(n.Provenance.Kind),
			},
		})
	}

	for _, e := range ir.Edges {
		rel := calmRelationship{
			UniqueID: e.ID,
			Metadata: map[string]any{
				"resolution":      string(e.Resolution),
				"provenance_kind": string(e.Provenance.Kind),
			},
		}
		if e.Type == core.EdgeTypeContainedIn {
			rel.RelationshipType.DeployedIn = &calmDeployedIn{Container: e.To, Nodes: []string{e.From}}
		} else {
			rel.Description = string(e.Type)
			rel.RelationshipType.Connects = &calmConnects{
				Source:      calmNodeInterface{Node: e.From},
				Destination: calmNodeInterface{Node: e.To},
			}
		}
		doc.Relationships = append(doc.Relationships, rel)
	}

	content, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("exporters: marshal CALM document: %w", err)
	}

	if err := ValidateCALM(content); err != nil {
		return nil, fmt.Errorf("exporters: CALM export failed schema validation against its own vendored schema (this is a bug in ExportCALM's mapping, not a caller error): %w", err)
	}

	return content, nil
}

// ValidateCALM checks calmJSON against the vendored, official CALM core.json document
// schema — see calm_schema/README.md for exactly where it came from. Exported (not
// just used internally by ExportCALM) so CI can run this same check as its own step
// against a deliberately malformed document, per PC-27's first acceptance criterion:
// "deliberately test CI validation ... with a malformed export to confirm it actually
// catches problems, not just that it passes on the happy path" — see
// TestValidateCALM_RejectsMalformedExport.
func ValidateCALM(calmJSON []byte) error {
	c := jsonschema.NewCompiler()
	for id, path := range calmSchemaFiles {
		data, err := calmSchemaFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("exporters: read vendored CALM schema %s: %w", path, err)
		}
		if err := c.AddResource(id, bytes.NewReader(data)); err != nil {
			return fmt.Errorf("exporters: add CALM schema resource %s: %w", id, err)
		}
	}
	sch, err := c.Compile(calmCoreSchemaID)
	if err != nil {
		return fmt.Errorf("exporters: compile vendored CALM schema: %w", err)
	}

	var doc any
	if err := json.Unmarshal(calmJSON, &doc); err != nil {
		return fmt.Errorf("exporters: CALM export is not valid JSON: %w", err)
	}
	return sch.Validate(doc)
}
