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
	// PC-158: these three read where things live (containment). A node whose placement attribute ingest
	// could not read may sit inside the zone being killed, so the blast radius, and whether anything
	// survives, is unknown: not_assessable, never the radius computed without it.
	if unread := anyUnresolved(ir, AffectsPlacement); len(unread) > 0 {
		for i := range findings {
			findings[i] = degradeToNotAssessable(findings[i], "a placement input could not be read, so which resources live in this zone is unknown: "+strings.Join(unread, "; "))
		}
	}
	// PC-129: a real, route-based check (does more than one route table's own
	// default route share the SAME NAT gateway) — distinct from
	// natGatewayRedundancyFinding above (PC-28's own containment-blast-radius-based
	// coverage check, hardcoded to golden's own public subnet IDs); this one is
	// generic (any IR, any route table) and answers PC-78's real open question.
	findings = append(findings, BuildNATSharedAcrossAZsFindings(ir)...)

	for _, node := range ir.Nodes {
		if node.Type != NodeTypeManagedDatabase {
			continue
		}
		findings = append(findings, storageEncryptionFinding(node))
		findings = append(findings, rpoFeasibilityFinding(node, workload))
	}

	for _, node := range ir.Nodes {
		switch {
		case len(UnresolvedInputs(node, AffectsTierLabel)) > 0:
			// PC-158: the tags this check reads could not be read, so whether the subnet is declared
			// public is unknown. The check must not silently not run.
			findings = append(findings, degradeToNotAssessable(publicSubnetRouteMismatchFinding(node, ir),
				"the tags that declare this subnet's tier could not be read: "+strings.Join(UnresolvedInputs(node, AffectsTierLabel), "; ")))
		case declaredPublicTier(node):
			findings = append(findings, publicSubnetRouteMismatchFinding(node, ir))
		}
	}

	// PC-135: least-privilege IAM controls, evaluated purely via PC-134's own
	// evaluator (see core/iam_compliance_findings.go's own boundary-rule doc comment).
	findings = append(findings, IAMLeastPrivilegeFindings(ir)...)

	// PC-130: per-journey configuration blast surface (single SG/NACL rule changes
	// that would break it) — see core/config_fault_findings.go's own doc comment.
	findings = append(findings, BuildConfigurationBlastSurfaceFindings(ir, workload)...)

	return findings
}

// declaredPublicTier reports whether a node declares itself public via this project's
// own established tagging convention (golden/aws/network.tf: tags.Tier = "public",
// already the real, existing signal this codebase's own golden bundle uses — not a new
// convention invented for this check). Generic over every node carrying this tag, not
// hardcoded to golden subnet IDs (PC-29's own "any node of this type" discipline,
// applied here to a label rather than a NodeType).
func declaredPublicTier(node Node) bool {
	tags, ok := node.RawAttributes["tags"].(map[string]any)
	if !ok {
		return false
	}
	tier, _ := tags["Tier"].(string)
	return tier == "public"
}

// publicSubnetRouteMismatchFinding is PC-111's own acceptance criterion, verbatim: "a
// subnet labelled public with no IGW route is reported as a finding, not trusted." The
// label is a convenience; IsPublicSubnet's own real route-table lookup is the
// authority — see that function's doc comment (core/routing.go) for the AWS VPC User
// Guide citation this whole check is built on.
func publicSubnetRouteMismatchFinding(node Node, ir *IR) Finding {
	prov := NewProvenance(KindDerived, "core/routing:public-subnet-check:"+node.ID)
	isPublic, hasRouteTable := IsPublicSubnetIR(ir, node.ID)

	nodeID := node.ID
	var outcome AssessmentEnvelope
	var evidenceDesc string
	switch {
	case !hasRouteTable:
		outcome = NotAssessable[any]("subnet has no resolvable effective route table (no aws_route_table_association, and the design does not declare exactly one main route table for the subnet's VPC — AWS then uses the VPC's main route table, whose non-local routes this engine cannot know)", prov).ToEnvelope()
		evidenceDesc = "no effective route table found for this subnet"
	case isPublic:
		outcome = Assessed[any]("consistent: declared public, and a real default route to an internet gateway confirms it", prov).ToEnvelope()
		evidenceDesc = "effective route table has a 0.0.0.0/0 route to an internet gateway"
	default:
		outcome = Assessed[any]("mismatch: declared public, but no default route to an internet gateway exists — the label is not trustworthy", prov).ToEnvelope()
		evidenceDesc = "effective route table has no 0.0.0.0/0 route to an internet gateway, despite the tags.Tier=public label"
	}

	return Finding{
		ID:    "finding.routing.public-subnet-label." + node.ID,
		Title: "Declared-public subnet's route table actually routes to an internet gateway",
		Dimensions: FailureMode{
			Trigger:            "a subnet is tagged Tier=public but its effective route table has no default route to an internet gateway",
			AffectedComponents: []string{nodeID},
			Detection:          DetectionModeled,
			Impact:             NotAssessable[any]("impact dimension not evaluated by this structural check", prov).ToEnvelope(),
			Likelihood:         DeriveLikelihood(prov).ToEnvelope(),
			Detectability:      DeriveDetectability(DetectionModeled, prov).ToEnvelope(),
			Recoverability: Recoverability{
				FailoverPathExists: NotAssessable[any]("recoverability not evaluated by this structural check", prov).ToEnvelope(),
				RPOFeasible:        NotAssessable[any]("recoverability not evaluated by this structural check", prov).ToEnvelope(),
			},
		},
		Evidence: []EvidenceRef{
			{NodeID: &nodeID, Description: evidenceDesc},
		},
		Outcome: outcome,
	}
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
			Trigger: "a single NAT gateway's AZ is lost, or the gateway itself fails",
			// AffectedComponents names what this check is ABOUT (the golden public
			// subnets it evaluates NAT gateway coverage against) — the same "identify
			// what's being evaluated, not what happened to be present" semantic
			// zoneKillFinding's own AffectedComponents already uses, never the subset
			// that happened to exist in a given bundle. A real bug this replaces: when
			// NONE of goldenPublicSubnets existed in the IR (a canvas-authored or
			// Azure architecture, which never has these AWS-specific subnet IDs at
			// all), the previous "only subnets actually found" logic left this field
			// nil — violating its own required,minItems=1 schema contract in real,
			// live output. Found by a real MCP client call validating a real tool
			// response against its own schema (PC-93), not a hand-written test with a
			// matched fixture — the same class of gap this project has caught
			// everywhere else two independent code paths (a schema's own contract,
			// and the code that must satisfy it) could silently disagree.
			AffectedComponents: goldenPublicSubnets,
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

// degradeToNotAssessable keeps a finding's identity and evidence but withdraws its verdict: the input it
// depends on could not be read (PC-158).
func degradeToNotAssessable(f Finding, reason string) Finding {
	prov := NewProvenance(KindDerived, "core/unresolved:"+f.ID)
	f.Outcome = NotAssessable[any](reason, prov).ToEnvelope()
	return f
}
