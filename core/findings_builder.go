package core

import (
	"fmt"
	"strings"
)

// BuildFindings produces a set of representative findings for an IR — originally
// written for cmd/gen-golden-fixtures (PC-15/PC-19) and now shared with server.Assess
// (PC-21) so both the fixture generator and the live server run the identical logic,
// not two copies that could quietly drift — exactly the duplication risk PC-21's own
// Conversation names for the MCP/HTTP split, generalized here to the generator too.
//
// Scope is deliberately narrow, not exhaustive: two zone-kill FailureModes (PC-14/78's
// containment engine, AWS-specific — see their own hardcoded golden subnet IDs) and
// one topology-redundancy check (PC-28, also AWS-specific: golden/azure has no NAT
// gateway concept in this bundle). These stay hardcoded to AWS's own golden node IDs
// deliberately — a resource-level zone-kill engine that would generalize them is real,
// separate, explicitly-scoped future work (see golden/azure/README.md), not silently
// attempted here.
//
// The compliance checks below (storage encryption, RPO feasibility) are NOT
// hardcoded, as of PC-29: they now run against every managed_database node the IR
// contains, regardless of provider — the fix "treat any agent failure to converge as
// a signal to fix the output shape" (PC-28) demanded, extended from "one AWS node" to
// "any node of this type" once golden/azure made it obvious that hardcoding a single
// AWS node ID meant Azure got zero real findings no matter how complete its mapping
// was. See managedDatabaseLabel's own doc comment for how this stays byte-identical
// for AWS's own already-tested finding IDs while giving Azure SQL its own honest
// equivalent.
func BuildFindings(ir *IR, workload Workload) []Finding {
	var containmentEdges []DirectedEdge
	for _, e := range ir.Edges {
		if e.Type == EdgeTypeContainedIn {
			containmentEdges = append(containmentEdges, DirectedEdge{From: e.From, To: e.To})
		}
	}

	findings := []Finding{
		zoneKillFinding(containmentEdges, workload, "aws_subnet.public_a", "finding.zone-kill.public-a",
			"AZ loss: eu-west-1a public subnet"),
		zoneKillFinding(containmentEdges, workload, "aws_subnet.data_a", "finding.zone-kill.data-a",
			"AZ loss: eu-west-1a data subnet"),
		natGatewayRedundancyFinding(ir, containmentEdges),
	}

	for _, node := range ir.Nodes {
		if node.Type != NodeTypeManagedDatabase {
			continue
		}
		findings = append(findings, storageEncryptionFinding(node))
		findings = append(findings, rpoFeasibilityFinding(node, workload))
	}

	return findings
}

// managedDatabaseLabel derives the finding-ID prefix, human-readable title noun, and
// (for Evidence) the real underlying attribute name conventionally used to state
// encryption, from a managed_database node's own ID prefix.
//
// This exists so AWS's already-established, tested, and documented finding IDs
// ("finding.compliance.rds-storage-encryption.aws_db_instance.payments" — referenced
// in core/scorecard_golden_test.go, demo/pc28-live-agent-run/TRANSCRIPT.md, and prior
// Jira comments) stay byte-identical after PC-29's genericization, while a different
// provider's equivalent node gets its own honestly-labeled ID rather than either
// inheriting "rds-" (wrong for Azure SQL) or falling back to something too generic to
// be useful. The switch is provider-detection-by-naming-convention, not elegant, but
// honest and low-risk — core has carried some provider-specific knowledge since PC-15
// (this function replaces what used to be a literal hardcoded node ID here), and this
// is strictly less AWS-specific than what it replaces, not a new kind of coupling.
func managedDatabaseLabel(nodeID string) (idPrefix, titleNoun, encryptionAttr string) {
	switch {
	case strings.HasPrefix(nodeID, "aws_db_instance."):
		return "rds", "RDS", "storage_encrypted"
	case strings.HasPrefix(nodeID, "azurerm_mssql_database."):
		return "sql", "Azure SQL", "transparent_data_encryption_enabled"
	default:
		return "managed-database", "managed database", "encryption_mechanism"
	}
}

func zoneKillFinding(containmentEdges []DirectedEdge, workload Workload, killedSubnet, id, title string) Finding {
	blastRadius := ContainmentBlastRadius(containmentEdges, killedSubnet)
	prov := NewProvenance(KindDerived, "core/analyse:zone-kill:"+killedSubnet)

	return Finding{
		ID:    id,
		Title: title,
		Dimensions: FailureMode{
			Trigger:            "availability zone loss",
			AffectedComponents: []string{killedSubnet},
			BlastRadius:        blastRadius,
			Detection:          DetectionModeled,
			ExistingMitigation: "none stated",
			Impact:             DeriveImpact(len(blastRadius), workload.Criticality, prov).ToEnvelope(),
			Likelihood:         DeriveLikelihood(prov).ToEnvelope(),
			Detectability:      DeriveDetectability(DetectionModeled, prov).ToEnvelope(),
			Recoverability: Recoverability{
				FailoverPathExists: NotAssessable[any]("recoverability is evaluated per affected resource, not at the zone-kill level — see per-resource RTO/RPO findings", prov).ToEnvelope(),
				RPOFeasible:        NotAssessable[any]("recoverability is evaluated per affected resource, not at the zone-kill level", prov).ToEnvelope(),
			},
		},
		Evidence: []EvidenceRef{
			{NodeID: &killedSubnet, Description: "core.ContainmentBlastRadius over this bundle's real contained_in edges"},
		},
		Outcome: outcomeFromImpact(len(blastRadius), prov),
	}
}

func outcomeFromImpact(blastRadiusSize int, prov Provenance) AssessmentEnvelope {
	if blastRadiusSize == 0 {
		return NotAssessable[any]("zone-kill impact could not be determined — either the failure is fully contained/redundant, or the affected substrate is not yet mapped (see golden/fixtures/README.md)", prov).ToEnvelope()
	}
	return Assessed[any](fmt.Sprintf("%d component(s) affected", blastRadiusSize), prov).ToEnvelope()
}

// storageEncryptionFinding reads a managed_database node's already-canonical
// Capability.EncryptionMechanism (populated identically for AWS's storage_encrypted
// and Azure's transparent_data_encryption_enabled — see providers/aws/rds.yaml and
// providers/azure/sql.yaml's own capability mappings) rather than an AWS-specific raw
// attribute key. See StorageEncryptionCheck's own doc comment for what generalizes
// across clouds (the classification logic) and what doesn't (the specific
// compliance-framework citation, not claimed here for any provider but AWS's
// originally-verified one).
func storageEncryptionFinding(node Node) Finding {
	idPrefix, titleNoun, attrName := managedDatabaseLabel(node.ID)

	var encrypted *bool
	if node.Capability != nil && node.Capability.EncryptionMechanism != nil {
		v := *node.Capability.EncryptionMechanism == "true"
		encrypted = &v
	}

	prov := NewProvenance(KindDerived, "core/analyse:storage-encryption-check:"+node.ID)
	status, rationale := StorageEncryptionCheck(encrypted, prov)

	nodeID := node.ID
	attr := attrName

	outcome := AssessmentEnvelope{State: AssessmentStateAssessed, Value: string(status.Status), Provenance: prov}
	if status.Status == ComplianceNotAssessable {
		outcome = NotAssessable[any](rationale, prov).ToEnvelope()
	}

	return Finding{
		ID:    "finding.compliance." + idPrefix + "-storage-encryption." + node.ID,
		Title: titleNoun + " storage encryption at rest",
		Dimensions: FailureMode{
			Trigger:            "unencrypted storage exposed via snapshot, backup, or storage compromise",
			AffectedComponents: []string{nodeID},
			Detection:          DetectionModeled,
			Impact:             NotAssessable[any]("impact dimension not evaluated by this compliance check", prov).ToEnvelope(),
			Likelihood:         DeriveLikelihood(prov).ToEnvelope(),
			Detectability:      DeriveDetectability(DetectionModeled, prov).ToEnvelope(),
			Recoverability: Recoverability{
				FailoverPathExists: NotAssessable[any]("recoverability not evaluated by this compliance check", prov).ToEnvelope(),
				RPOFeasible:        NotAssessable[any]("recoverability not evaluated by this compliance check", prov).ToEnvelope(),
			},
		},
		Evidence: []EvidenceRef{
			{NodeID: &nodeID, Attribute: &attr, Description: rationale},
		},
		Outcome: outcome,
	}
}

// goldenPublicSubnets are the golden architecture's own three public subnets
// (network.tf) — hardcoded, per this function's own scope note above.
var goldenPublicSubnets = []string{"aws_subnet.public_a", "aws_subnet.public_b", "aws_subnet.public_c"}

func natGatewayRedundancyFinding(ir *IR, containmentEdges []DirectedEdge) Finding {
	nodeTypeByID := map[string]NodeType{}
	for _, n := range ir.Nodes {
		nodeTypeByID[n.ID] = n.Type
	}

	counts := map[string]int{}
	var allAffected []string
	for _, subnet := range goldenPublicSubnets {
		if _, present := nodeTypeByID[subnet]; !present {
			continue // this subnet isn't in the IR at all (out of scope for this bundle)
		}
		count := 0
		for _, id := range ContainmentBlastRadius(containmentEdges, subnet) {
			if nodeTypeByID[id] == NodeTypeNetworkBoundary && isNATGatewayID(id) {
				count++
			}
		}
		counts[subnet] = count
		allAffected = append(allAffected, subnet)
	}

	prov := NewProvenance(KindDerived, "core/analyse:nat-gateway-redundancy")
	result, rationale := NATGatewayRedundancyCheck(counts, prov)

	outcome := AssessmentEnvelope{State: AssessmentStateAssessed, Value: string(result.Status), Provenance: prov}
	if result.Status == ComplianceNotAssessable {
		outcome = NotAssessable[any](rationale, prov).ToEnvelope()
	}

	return Finding{
		ID:    "finding.compliance.nat-gateway-redundancy",
		Title: "NAT gateway redundancy across public subnets",
		Dimensions: FailureMode{
			Trigger:            "a single NAT gateway's AZ is lost, or the gateway itself fails",
			AffectedComponents: allAffected,
			Detection:          DetectionModeled,
			Impact:             NotAssessable[any]("impact dimension not evaluated by this compliance check", prov).ToEnvelope(),
			Likelihood:         DeriveLikelihood(prov).ToEnvelope(),
			Detectability:      DeriveDetectability(DetectionModeled, prov).ToEnvelope(),
			Recoverability: Recoverability{
				FailoverPathExists: NotAssessable[any]("recoverability not evaluated by this compliance check", prov).ToEnvelope(),
				RPOFeasible:        NotAssessable[any]("recoverability not evaluated by this compliance check", prov).ToEnvelope(),
			},
		},
		Evidence: []EvidenceRef{
			{Description: rationale},
		},
		Outcome: outcome,
	}
}

func isNATGatewayID(id string) bool {
	return len(id) > len("aws_nat_gateway.") && id[:len("aws_nat_gateway.")] == "aws_nat_gateway."
}

// rdsRPOFeasibilityFinding directly checks the ONE declared hard requirement
// (workload.yaml's rpo_seconds) this codebase has a real engine for — PC-14's
// core.RPOFeasibility, reusing core.DeriveNodeReplicationAndFailover to derive the
// replication mode from the node's own stated multi_az attribute. Added alongside the
// NAT-redundancy finding (PC-28): the RDS storage-encryption finding checks a PCI
// compliance dimension, not this requirement directly — conflating the two would have
// been an imprecise proxy for something this system can actually answer precisely.
// rpoFeasibilityFinding directly checks the ONE declared hard requirement
// (workload.yaml's rpo_seconds) this codebase has a real engine for — PC-14's
// RPOFeasibility, reusing DeriveNodeReplicationAndFailover (provider-agnostic since
// the PC-22 failover relocation) to derive the replication mode from whichever
// provider's node this is. Generalized (PC-29) from a single hardcoded AWS node to any
// managed_database node — see managedDatabaseLabel's own doc comment for the ID/title
// convention this preserves for AWS while giving other providers their own label.
func rpoFeasibilityFinding(node Node, workload Workload) Finding {
	idPrefix, titleNoun, _ := managedDatabaseLabel(node.ID)
	replicationMode, _ := DeriveNodeReplicationAndFailover(node)

	prov := NewProvenance(KindDerived, "core/analyse:rpo-feasibility:"+node.ID)

	rpoTarget, hasTarget := RequirementValue(workload, "rpo_seconds")
	var rpoTargetPtr *float64
	if hasTarget {
		rpoTargetPtr = &rpoTarget
	}

	result := RPOFeasibility(rpoTargetPtr, replicationMode, prov)
	status := ComplianceApplicable
	var rationale string
	if !result.IsAssessed() {
		status = ComplianceNotAssessable
		rationale, _ = result.Reason()
	} else if v, _ := result.Value(); v {
		status = ComplianceSatisfied
		rationale = "declared rpo_seconds requirement is feasible given this resource's replication mode"
	} else {
		status = ComplianceUnsatisfied
		rationale = "declared rpo_seconds requirement is NOT feasible given this resource's replication mode"
	}

	nodeID := node.ID
	outcome := AssessmentEnvelope{State: AssessmentStateAssessed, Value: string(status), Provenance: prov}
	if status == ComplianceNotAssessable {
		outcome = NotAssessable[any](rationale, prov).ToEnvelope()
	}

	return Finding{
		ID:    "finding.compliance." + idPrefix + "-rpo-feasibility." + node.ID,
		Title: titleNoun + " RPO feasibility against the declared workload requirement",
		Dimensions: FailureMode{
			Trigger:            "primary database failure with an infeasible replication mode for the declared RPO",
			AffectedComponents: []string{nodeID},
			Detection:          DetectionModeled,
			Impact:             NotAssessable[any]("impact dimension not evaluated by this compliance check", prov).ToEnvelope(),
			Likelihood:         DeriveLikelihood(prov).ToEnvelope(),
			Detectability:      DeriveDetectability(DetectionModeled, prov).ToEnvelope(),
			Recoverability: Recoverability{
				FailoverPathExists: NotAssessable[any]("see this finding's own Outcome for this same underlying check", prov).ToEnvelope(),
				RPOFeasible:        result.ToEnvelope(),
			},
		},
		Evidence: []EvidenceRef{
			{NodeID: &nodeID, Description: rationale},
		},
		Outcome: outcome,
	}
}
