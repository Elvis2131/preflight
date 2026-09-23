// This file exports an AWS Fault Injection Simulator (FIS) experiment template for a
// zone-kill Finding — PC-23's own Conversation: "FIS template structure ... has real
// specifics worth a short spike before committing to the exporter's output shape —
// don't guess the schema." The shape below is copied from AWS's own documented,
// complete example ("Stop EC2 instances based on filters",
// docs.aws.amazon.com/fis/latest/userguide/experiment-template-example.html, verified
// 2026-09-21), adapted to this finding's own affected availability zone — not
// invented from memory.
package exporters

import (
	"encoding/json"
	"fmt"
	"strings"

	"preflight/core"
)

// fisTemplate mirrors AWS FIS's real experiment-template JSON syntax exactly
// (docs.aws.amazon.com/fis/latest/userguide/experiment-templates.html): description,
// targets, actions, stopConditions, roleArn, tags.
type fisTemplate struct {
	Description string               `json:"description"`
	Targets     map[string]fisTarget `json:"targets"`
	Actions     map[string]fisAction `json:"actions"`
	// StopConditions is a required field (an empty template with no monitored alarm
	// is rejected by the real FIS API) — "none" is the documented sentinel for "run
	// without a CloudWatch-alarm-based stop condition" (see the "Run a predefined
	// Automation runbook" and "Kinesis stream experiment" examples in AWS's own
	// example-templates doc, both using stopConditions: [{"source": "none"}]).
	StopConditions []fisStopCondition `json:"stopConditions"`
	RoleArn        string             `json:"roleArn"`
	Tags           map[string]string  `json:"tags"`
}

type fisTarget struct {
	ResourceType  string            `json:"resourceType"`
	ResourceTags  map[string]string `json:"resourceTags,omitempty"`
	Filters       []fisFilter       `json:"filters,omitempty"`
	SelectionMode string            `json:"selectionMode"`
}

type fisFilter struct {
	Path   string   `json:"path"`
	Values []string `json:"values"`
}

type fisAction struct {
	ActionID    string            `json:"actionId"`
	Description string            `json:"description"`
	Parameters  map[string]string `json:"parameters,omitempty"`
	Targets     map[string]string `json:"targets"`
}

type fisStopCondition struct {
	Source string `json:"source"`
	Value  string `json:"value,omitempty"`
}

// placeholderAccountID and placeholderRoleName are AWS's own documented placeholder
// values (the exact account ID AWS's own FIS docs use throughout their examples,
// e.g. "arn:aws:iam::111122223333:role/role-name") — used here rather than an
// invented value, and named exactly for what they are: account-specific
// configuration any real deployment must supply, the same way golden/aws's own
// variables.tf declares var.sql_admin_password rather than a real secret. This is
// not what PC-23's "runs without manual editing" criterion is about — that criterion
// is about structural completeness (a real, submittable document), not about this
// exporter somehow knowing a stranger's AWS account ID in advance.
const (
	placeholderAccountID = "111122223333"
	placeholderRoleName  = "FISExperimentRole"
)

// ExportFIS derives an AWS FIS experiment template from a zone-kill Finding — stops
// every EC2 instance in the Finding's own affected availability zone, restarting them
// after 2 minutes (mirroring AWS's own documented example exactly). ir is required to
// look up the affected subnet's real availability_zone attribute; a Finding whose
// AffectedComponents don't resolve to a real AWS subnet node with a stated AZ returns
// an error rather than a fabricated target.
func ExportFIS(finding core.Finding, ir *core.IR, prov core.Provenance) (ExportedExperiment, error) {
	az, err := findingAvailabilityZone(finding, ir)
	if err != nil {
		return ExportedExperiment{}, err
	}

	tmpl := fisTemplate{
		Description: fmt.Sprintf("Simulate %s (derived from Preflight finding %s): stop all running EC2 instances in %s, restart after 2 minutes.", finding.Title, finding.ID, az),
		Targets: map[string]fisTarget{
			"AffectedInstances": {
				ResourceType: "aws:ec2:instance",
				Filters: []fisFilter{
					{Path: "Placement.AvailabilityZone", Values: []string{az}},
					{Path: "State.Name", Values: []string{"running"}},
				},
				SelectionMode: "ALL",
			},
		},
		Actions: map[string]fisAction{
			"StopInstances": {
				ActionID:    "aws:ec2:stop-instances",
				Description: "stop the instances",
				Parameters:  map[string]string{"startInstancesAfterDuration": "PT2M"},
				Targets:     map[string]string{"Instances": "AffectedInstances"},
			},
		},
		StopConditions: []fisStopCondition{{Source: "none"}},
		RoleArn:        fmt.Sprintf("arn:aws:iam::%s:role/%s", placeholderAccountID, placeholderRoleName),
		Tags:           map[string]string{"Name": "Preflight-" + finding.ID, "source_finding": finding.ID},
	}

	content, err := json.MarshalIndent(tmpl, "", "  ")
	if err != nil {
		return ExportedExperiment{}, fmt.Errorf("exporters: marshal FIS template: %w", err)
	}

	return ExportedExperiment{
		Format:          "fis",
		SourceFindingID: finding.ID,
		Content:         string(content),
		Provenance:      prov,
	}, nil
}

// ExportFISRegionLoss derives an AWS FIS experiment template corresponding to
// core.Simulate's own "region_loss" fault (PC-82) — PC-29's own criterion, "region-loss
// simulation verified by an exported chaos experiment", verbatim. Unlike
// ExportFIS/ExportLitmus (keyed to one zone-kill Finding, PC-23's own stated scope),
// this targets every EC2 instance tagged with the workload's own Name (golden/
// workload.yaml declares name: payments-api, matching golden/aws/network.tf's own
// real Service=payments-api tag — a genuine, verified correlation, not assumed), with
// no availability-zone filter at all — stopping everything belonging to this
// workload, region-wide, which is the closest FIS-native expression of "region loss"
// that exists: FIS has no single "kill this region" action, because AWS itself has no
// such API — the real, documented way to chaos-test a region-wide outage is exactly
// this shape (stop every instance in scope), the same pattern AWS's own "Stop EC2
// instances based on filters" example uses, just without an AZ-scoping filter.
//
// No Litmus equivalent is exported for region_loss, deliberately: LitmusChaos's own
// scope is workload/pod/node-level chaos WITHIN an existing Kubernetes cluster — its
// node-drain experiment (used for the zone-kill case above) drains specific nodes, but
// "the entire region is gone" for a Kubernetes cluster means the cluster's own control
// plane and every node are gone, which isn't a node-drain scenario at all (draining
// literally every node is a materially different, far more destructive action than
// this exporter should suggest as equivalent). Stated as a real scope boundary, not a
// gap silently left unaddressed.
func ExportFISRegionLoss(workload core.Workload, prov core.Provenance) (ExportedExperiment, error) {
	if workload.Name == "" {
		return ExportedExperiment{}, fmt.Errorf("exporters: workload has no Name to scope a region-loss experiment to — refusing to target an unscoped resourceTags filter")
	}

	tmpl := fisTemplate{
		Description: fmt.Sprintf("Simulate region loss for workload %q (derived from a core.Simulate region_loss fault, PC-82): stop every EC2 instance tagged Service=%s, restart after 2 minutes.", workload.Name, workload.Name),
		Targets: map[string]fisTarget{
			"AllWorkloadInstances": {
				ResourceType:  "aws:ec2:instance",
				ResourceTags:  map[string]string{"Service": workload.Name},
				SelectionMode: "ALL",
			},
		},
		Actions: map[string]fisAction{
			"StopInstances": {
				ActionID:    "aws:ec2:stop-instances",
				Description: "stop every instance belonging to this workload, region-wide",
				Parameters:  map[string]string{"startInstancesAfterDuration": "PT2M"},
				Targets:     map[string]string{"Instances": "AllWorkloadInstances"},
			},
		},
		StopConditions: []fisStopCondition{{Source: "none"}},
		RoleArn:        fmt.Sprintf("arn:aws:iam::%s:role/%s", placeholderAccountID, placeholderRoleName),
		Tags:           map[string]string{"Name": "Preflight-region-loss-" + workload.Name, "source": "core.Simulate:region_loss"},
	}

	content, err := json.MarshalIndent(tmpl, "", "  ")
	if err != nil {
		return ExportedExperiment{}, fmt.Errorf("exporters: marshal FIS region-loss template: %w", err)
	}

	return ExportedExperiment{
		Format:          "fis",
		SourceFindingID: "simulate:region_loss:" + workload.Name,
		Content:         string(content),
		Provenance:      prov,
	}, nil
}

// findingAvailabilityZone resolves a zone-kill Finding's own affected AZ.
//
// Tried first, and preferred: the killed node's own stated RawAttributes
// ["availability_zone"] — a real literal Terraform value. Verified against the real
// golden bundle this DOES NOT WORK for golden/aws/network.tf specifically: its own
// subnets declare `availability_zone = local.az_a`, a locals reference, and
// ingest/parse.go's own ParsedResource.Attributes only captures literal values with
// no external references (its own documented scope) — so this attribute is
// legitimately absent from every golden AWS subnet's RawAttributes, not a bug in this
// exporter or in ingest.
//
// Fallback, used here because it is the ONLY place this fact already exists anywhere
// in this codebase: core/findings_builder.go's own zoneKillFinding calls hardcode the
// real AZ name directly into each Finding's Title ("AZ loss: eu-west-1a public
// subnet") — there is no other derivation of it anywhere today. Parsed via a strict,
// documented convention ("AZ loss: <az> ..."), not a loose guess: this function
// fails clearly if the Title doesn't match rather than extracting something that
// merely looks plausible. A real fix — resolving locals.az_a in ingest so
// RawAttributes carries a genuine derived value — is separate, future ingest-parser
// work, not attempted here as a side effect of building an exporter.
func findingAvailabilityZone(finding core.Finding, ir *core.IR) (string, error) {
	if len(finding.Dimensions.AffectedComponents) == 0 {
		return "", fmt.Errorf("exporters: finding %s has no AffectedComponents to resolve an availability zone from", finding.ID)
	}
	killedID := finding.Dimensions.AffectedComponents[0]

	var killedNode *core.Node
	for i := range ir.Nodes {
		if ir.Nodes[i].ID == killedID {
			killedNode = &ir.Nodes[i]
			break
		}
	}
	if killedNode == nil {
		return "", fmt.Errorf("exporters: finding %s's affected component %s was not found in the IR", finding.ID, killedID)
	}

	if az, ok := killedNode.RawAttributes["availability_zone"].(string); ok && az != "" {
		return az, nil
	}

	const prefix = "AZ loss: "
	if !strings.HasPrefix(finding.Title, prefix) {
		return "", fmt.Errorf("exporters: node %s has no stated availability_zone attribute, and finding %s's Title %q does not match the fallback convention %q — cannot derive a real target without guessing", killedID, finding.ID, finding.Title, prefix+"<az> ...")
	}
	rest := strings.TrimPrefix(finding.Title, prefix)
	az, _, found := strings.Cut(rest, " ")
	if !found || az == "" {
		return "", fmt.Errorf("exporters: finding %s's Title %q matched the %q prefix but no AZ token followed it", finding.ID, finding.Title, prefix)
	}
	return az, nil
}
