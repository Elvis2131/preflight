// This file exports a LitmusChaos ChaosEngine manifest for a zone-kill Finding —
// PC-23's own Conversation: "Litmus manifest format ... has real specifics worth a
// short spike before committing to the exporter's output shape — don't guess the
// schema." The shape below is verified against LitmusChaos's own real documentation
// (litmuschaos.github.io/litmus/experiments/categories/nodes/node-drain/, verified
// 2026-09-21): apiVersion, kind, and the experiments[].spec.components.env shape are
// copied from that page's own example, not invented. node-drain (draining the node(s)
// so their pods reschedule elsewhere) is the real, documented Litmus experiment
// closest to "AZ loss" at the Kubernetes level — the golden architecture's own
// container_workload node type (EKS) is exactly what this drains.
package exporters

import (
	"fmt"
	"strings"

	"preflight/core"
)

// ExportLitmus derives a LitmusChaos ChaosEngine manifest from a zone-kill Finding —
// drains every node in the Finding's own affected availability zone (selected via
// Kubernetes' own well-known node label, kubernetes.io/docs/reference/labels-
// annotations-taints/, topology.kubernetes.io/zone — verified 2026-09-21, not
// guessed), rather than a single named node, since which specific nodes exist in a
// given AZ at experiment time is not something this exporter can know in advance.
func ExportLitmus(finding core.Finding, ir *core.IR, prov core.Provenance) (ExportedExperiment, error) {
	az, err := findingAvailabilityZone(finding, ir)
	if err != nil {
		return ExportedExperiment{}, err
	}

	engineName := "preflight-" + sanitizeK8sName(finding.ID)

	var b strings.Builder
	fmt.Fprintf(&b, "# Derived from Preflight finding %s (%s) — drains every node in %s.\n", finding.ID, finding.Title, az)
	fmt.Fprintf(&b, "# chaosServiceAccount (node-drain-sa) and the litmus namespace/RBAC must exist\n")
	fmt.Fprintf(&b, "# in the target cluster before this runs — see LitmusChaos's own installation docs.\n")
	fmt.Fprintf(&b, "apiVersion: litmuschaos.io/v1alpha1\n")
	fmt.Fprintf(&b, "kind: ChaosEngine\n")
	fmt.Fprintf(&b, "metadata:\n")
	fmt.Fprintf(&b, "  name: %s\n", engineName)
	fmt.Fprintf(&b, "  namespace: litmus\n")
	fmt.Fprintf(&b, "spec:\n")
	fmt.Fprintf(&b, "  engineState: \"active\"\n")
	fmt.Fprintf(&b, "  annotationCheck: \"false\"\n")
	fmt.Fprintf(&b, "  chaosServiceAccount: node-drain-sa\n")
	fmt.Fprintf(&b, "  experiments:\n")
	fmt.Fprintf(&b, "  - name: node-drain\n")
	fmt.Fprintf(&b, "    spec:\n")
	fmt.Fprintf(&b, "      components:\n")
	fmt.Fprintf(&b, "        env:\n")
	fmt.Fprintf(&b, "        - name: NODE_LABEL\n")
	fmt.Fprintf(&b, "          value: 'topology.kubernetes.io/zone=%s'\n", az)
	fmt.Fprintf(&b, "        - name: TOTAL_CHAOS_DURATION\n")
	fmt.Fprintf(&b, "          value: '60'\n")

	return ExportedExperiment{
		Format:          "litmus",
		SourceFindingID: finding.ID,
		Content:         b.String(),
		Provenance:      prov,
	}, nil
}

// sanitizeK8sName produces a Kubernetes-object-name-safe suffix from a finding ID
// (Kubernetes names must be lowercase RFC 1123 labels — no dots). finding.zone-kill.
// public-a already uses hyphens, but the dots need converting; this is intentionally
// minimal (this codebase's own finding IDs are already a small, known, controlled
// vocabulary — see core/findings_builder.go — not arbitrary user input needing full
// RFC 1123 sanitization).
func sanitizeK8sName(id string) string {
	return strings.ReplaceAll(id, ".", "-")
}
