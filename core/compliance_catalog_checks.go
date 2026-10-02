// This file is PC-121's shared architectural checks, the deterministic evidence behind every
// assessable or partially assessable control in the PCI DSS and SOC 2 catalogs. Each reuses an
// engine that already exists (routing, security groups, IAM evaluation, storage encryption,
// the IR's own capability facts); no evaluation logic is invented for compliance.
//
// A control a diagram can only partly evidence is classified partially_assessable, and its
// architectural finding never reads "satisfied": it becomes ComplianceApplicable ("the
// architecture supports this; the rest needs evidence a diagram cannot show"). A fact the IR does
// not model produces not_assessable with that reason, never a pass or a fail (I4).
package core

import "strings"

// ctl is one control's fixed metadata, so a check can build its results without repeating it.
type ctl struct {
	id        string
	framework ComplianceFramework
	req       string
	title     string
	class     ControlClassification
}

func (c ctl) result(nodeID string, status ComplianceStatus, rationale string, prov Provenance) ComplianceControlResult {
	return ComplianceControlResult{
		ControlID: c.id, Framework: c.framework, RequirementID: c.req, Title: c.title,
		Classification: c.class, NodeID: nodeID,
		Result:    ComplianceResult{Status: status, Provenance: prov},
		Rationale: rationale,
	}
}

// supported is the status a finding takes when the architecture is fine: satisfied for an
// assessable control, applicable (never satisfied) for a partially assessable one.
func (c ctl) supported() ComplianceStatus {
	if c.class == ClassificationPartial {
		return ComplianceApplicable
	}
	return ComplianceSatisfied
}

const partialNote = "; the architectural aspect is supported, but the full requirement also needs evidence a diagram cannot show"

func (c ctl) supportedRationale(r string) string {
	if c.class == ClassificationPartial {
		return r + partialNote
	}
	return r
}

func databaseNodes(ir *IR) []Node {
	var out []Node
	for _, n := range ir.Nodes {
		if n.Type == NodeTypeManagedDatabase {
			out = append(out, n)
		}
	}
	return out
}

func identityNodes(ir *IR) []Node {
	var out []Node
	for _, n := range ir.Nodes {
		if n.Type == NodeTypeIdentity {
			out = append(out, n)
		}
	}
	return out
}

// checkDatabaseNotPubliclyRoutable reuses databaseNotPubliclyRoutableResult (PC-111's route
// evaluation) verbatim for each managed database.
func checkDatabaseNotPubliclyRoutable(ir *IR, c ctl, prov Provenance) []ComplianceControlResult {
	dbs := databaseNodes(ir)
	if len(dbs) == 0 {
		return []ComplianceControlResult{c.result("", ComplianceNotAssessable, "this IR has no managed_database node to evaluate", prov)}
	}
	var out []ComplianceControlResult
	for _, db := range dbs {
		r := databaseNotPubliclyRoutableResult(ir, db, c.id, c.framework, c.req, c.title, prov)
		r.Classification = c.class
		if r.Result.Status == ComplianceSatisfied {
			r.Result.Status = c.supported()
			r.Rationale = c.supportedRationale(r.Rationale)
		}
		out = append(out, r)
	}
	return out
}

func ruleOpenToWorld(r SGRule) bool {
	for _, cidr := range r.CIDRs {
		if cidr == "0.0.0.0/0" || cidr == "::/0" {
			return true
		}
	}
	return false
}

// checkDatabaseSGIngress asks whether any security group attached to a database admits traffic
// from anywhere. World-open ingress to a data store is never "only what is necessary", so it is a
// definite failure; its absence supports, but cannot prove, that inbound is limited to what is
// necessary (that needs the documented business need).
func checkDatabaseSGIngress(ir *IR, c ctl, prov Provenance) []ComplianceControlResult {
	return perDatabaseSG(ir, c, prov, "ingress", func(r SGRule) bool { return ruleOpenToWorld(r) },
		"admits traffic from anywhere (0.0.0.0/0)", "no attached security group admits traffic from anywhere")
}

// checkDatabaseSGEgress asks whether any attached security group lets the database send any
// protocol to anywhere.
func checkDatabaseSGEgress(ir *IR, c ctl, prov Provenance) []ComplianceControlResult {
	return perDatabaseSG(ir, c, prov, "egress", func(r SGRule) bool { return ruleOpenToWorld(r) && r.Protocol == "-1" },
		"permits all protocols to anywhere (0.0.0.0/0)", "no attached security group permits all protocols to anywhere")
}

func perDatabaseSG(ir *IR, c ctl, prov Provenance, direction string, violates func(SGRule) bool, violationText, okText string) []ComplianceControlResult {
	dbs := databaseNodes(ir)
	if len(dbs) == 0 {
		return []ComplianceControlResult{c.result("", ComplianceNotAssessable, "this IR has no managed_database node to evaluate", prov)}
	}
	var out []ComplianceControlResult
	for _, db := range dbs {
		profile := SecurityGroupProfile(ir.Nodes, ir.Edges, db.ID)
		if len(profile.SGIDs) == 0 {
			out = append(out, c.result(db.ID, ComplianceNotAssessable, "no security group is attached to this resource, so its "+direction+" posture is unknown, never assumed", prov))
			continue
		}
		var bad []string
		for _, r := range profile.Rules {
			if r.Direction == direction && violates(r) {
				bad = append(bad, "["+r.Protocol+" "+strings.Join(r.CIDRs, ",")+"]")
			}
		}
		if len(bad) > 0 {
			out = append(out, c.result(db.ID, ComplianceUnsatisfied, "an attached security group ("+strings.Join(profile.SGIDs, ", ")+") "+violationText+": "+strings.Join(bad, " "), prov))
			continue
		}
		out = append(out, c.result(db.ID, c.supported(), c.supportedRationale(okText+" ("+strings.Join(profile.SGIDs, ", ")+")"), prov))
	}
	return out
}

// checkStoredDataProtection maps storage-level encryption onto a stored-data-protection control.
// Storage encryption is disk-level protection. PCI DSS 3.5.1.2 allows disk-level encryption to
// render account data unreadable only on removable media, or on non-removable media when the data is
// ALSO rendered unreadable another way, and SOC 2 CC6.1 leaves encryption to the entity's own risk
// strategy. So encrypted storage is supporting evidence (applicable), never a pass, and unencrypted
// storage is not a failure either: field-level protection of the data, which this model cannot see,
// would still be what counts. CIS (a different standard) judges the same fact on its own terms.
func checkStoredDataProtection(ir *IR, c ctl, prov Provenance, whyNotAFailure string) []ComplianceControlResult {
	dbs := databaseNodes(ir)
	if len(dbs) == 0 {
		return []ComplianceControlResult{c.result("", ComplianceNotAssessable, "this IR has no managed_database node to evaluate", prov)}
	}
	var out []ComplianceControlResult
	for _, db := range dbs {
		var encrypted *bool
		if db.Capability != nil && db.Capability.EncryptionMechanism != nil {
			v := *db.Capability.EncryptionMechanism == "true"
			encrypted = &v
		}
		res, rationale := StorageEncryptionCheck(encrypted, prov)
		switch res.Status {
		case ComplianceSatisfied:
			out = append(out, c.result(db.ID, ComplianceApplicable, rationale+"; "+whyNotAFailure+" — storage encryption supports this but does not satisfy it alone", prov))
		case ComplianceUnsatisfied:
			out = append(out, c.result(db.ID, ComplianceNotAssessable, "no storage-level encryption is declared ("+rationale+"); "+whyNotAFailure+" — protection applied inside the data itself is not visible in this model", prov))
		default:
			out = append(out, c.result(db.ID, ComplianceNotAssessable, rationale, prov))
		}
	}
	return out
}

// checkTransportNotModelled is the honest result for transmission-protection controls: TLS
// settings of listeners and endpoints are not part of the IR, so nothing is claimed.
func checkTransportNotModelled(c ctl, prov Provenance) []ComplianceControlResult {
	return []ComplianceControlResult{c.result("", ComplianceNotAssessable,
		"the TLS and protocol settings of listeners and endpoints are not part of the architecture model, so transmission protection is neither confirmed nor refuted", prov)}
}

// checkPublicFacingBehindWAF asks, for every internet-facing load balancer, whether a web
// application firewall is attached to it. An attached firewall supports the control (it must
// also be configured, current and logged: process evidence). The absence of one is NOT a failure:
// a firewall in front of the load balancer (a content-delivery or API layer) is outside the model,
// so the result is not_assessable and says so.
func checkPublicFacingBehindWAF(ir *IR, c ctl, prov Provenance) []ComplianceControlResult {
	byID := map[string]Node{}
	for _, n := range ir.Nodes {
		byID[n.ID] = n
	}
	var out []ComplianceControlResult
	for _, n := range ir.Nodes {
		if n.Type != NodeTypeLoadBalancer {
			continue
		}
		if internal, _ := n.RawAttributes["internal"].(bool); internal {
			continue // not internet-facing: not a public-facing application
		}
		attached := ""
		for _, e := range ir.Edges {
			if e.Type != EdgeTypeDependsOn || e.From != n.ID {
				continue
			}
			if t, ok := byID[e.To]; ok && t.RawAttributes["network_role"] == "web_acl" {
				attached = t.ID
				break
			}
		}
		if attached != "" {
			out = append(out, c.result(n.ID, c.supported(), c.supportedRationale("this internet-facing load balancer has a web application firewall attached ("+attached+")"), prov))
			continue
		}
		out = append(out, c.result(n.ID, ComplianceNotAssessable,
			"no web application firewall is attached to this internet-facing load balancer; one placed in front of it (a content-delivery or API layer) is not modelled, so absence is not treated as a failure", prov))
	}
	if len(out) == 0 {
		return []ComplianceControlResult{c.result("", ComplianceNotAssessable, "this IR has no internet-facing load balancer to evaluate", prov)}
	}
	return out
}

// checkLeastPrivilege reuses PC-135's IAM evaluation verbatim: does any identity policy grant
// every action on every resource. A grant is a definite failure; its absence supports least
// privilege but cannot show that access is assigned by job function (process evidence).
func checkLeastPrivilege(ir *IR, c ctl, prov Provenance) []ComplianceControlResult {
	roles := identityNodes(ir)
	if len(roles) == 0 {
		return []ComplianceControlResult{c.result("", ComplianceNotAssessable, "this IR has no identity node to evaluate", prov)}
	}
	var out []ComplianceControlResult
	for _, role := range roles {
		evalProv := NewProvenance(KindDerived, "core/iam_evaluate:"+c.id+":"+role.ID)
		res := EvaluateIAMRequest(ir, IAMRequest{PrincipalID: role.ID, Action: "*", ResourceARN: "*"}, evalProv)
		status, rationale := iamDecisionToCompliance(res,
			"no identity policy statement grants every action on every resource",
			"an identity policy statement grants every action on every resource — full administrative access")
		if status == ComplianceSatisfied {
			status = c.supported()
			rationale = c.supportedRationale(rationale)
		}
		out = append(out, c.result(role.ID, status, rationale, prov))
	}
	return out
}

// checkRecoveryInfrastructure reports, per managed database, whether the design declares
// redundant recovery infrastructure (a synchronous standby). Its presence supports the availability
// criterion; its absence is not a failure, because recovery arrangements outside the model (backups,
// rebuild automation) are not visible here.
func checkRecoveryInfrastructure(ir *IR, c ctl, prov Provenance) []ComplianceControlResult {
	dbs := databaseNodes(ir)
	if len(dbs) == 0 {
		return []ComplianceControlResult{c.result("", ComplianceNotAssessable, "this IR has no managed_database node to evaluate", prov)}
	}
	var out []ComplianceControlResult
	for _, db := range dbs {
		standby := db.Capability != nil && db.Capability.MultiAZImplementation != nil && *db.Capability.MultiAZImplementation == "true"
		switch {
		case standby:
			out = append(out, c.result(db.ID, c.supported(), c.supportedRationale("this database declares a synchronous standby in another availability zone"), prov))
		case db.Capability == nil || db.Capability.MultiAZImplementation == nil:
			out = append(out, c.result(db.ID, ComplianceNotAssessable, "whether this database has a standby is not declared", prov))
		default:
			out = append(out, c.result(db.ID, ComplianceNotAssessable, "this database declares no standby; recovery arrangements outside the model (backups, rebuild automation) are not visible, so absence is not treated as a failure", prov))
		}
	}
	return out
}
