// This file is PC-133's ingest-side half: parsing a real AWS IAM policy document
// (the JSON text Terraform's own jsonencode()/heredoc produces, already captured
// verbatim as a plain Go string in ParsedResource.Attributes) into core.PolicyDocument.
// Every real AWS JSON policy quirk this file handles was verified against the golden
// bundle's own real assume_role_policy documents (golden/aws/*.tf) before writing any
// parsing code: Action can be a bare string OR an array of strings; Statement can be a
// single object OR an array; Principal can be "*", a bare string, or an object with
// AWS/Service/Federated keys.
package ingest

import (
	"encoding/json"
	"fmt"
	"sort"

	"preflight/core"
)

// rawPolicyDocument mirrors the real AWS policy JSON shape loosely enough to accept
// every real variant — Statement is `any` because AWS allows either a single
// statement object or an array of them.
type rawPolicyDocument struct {
	Version   string `json:"Version"`
	Statement any    `json:"Statement"`
}

// parsePolicyDocument parses rawJSON (a policy document's own literal JSON text) into
// a core.PolicyDocument, preserving every field verbatim — no wildcard expansion, no
// condition evaluation, no guessed defaults. Returns an error only when rawJSON isn't
// valid JSON at all (a real, structural ingest failure, never silently swallowed).
func parsePolicyDocument(id, rawJSON string, prov core.Provenance) (core.PolicyDocument, error) {
	var raw rawPolicyDocument
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return core.PolicyDocument{}, fmt.Errorf("ingest: parse policy document %s: %w", id, err)
	}

	var rawStatements []any
	switch v := raw.Statement.(type) {
	case []any:
		rawStatements = v
	case map[string]any:
		rawStatements = []any{v}
	default:
		return core.PolicyDocument{}, fmt.Errorf("ingest: policy document %s: Statement is neither an object nor an array", id)
	}

	doc := core.PolicyDocument{ID: id, Version: raw.Version, Provenance: prov}
	for _, s := range rawStatements {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		stmt := core.PolicyStatement{
			Condition: toStringKeyedMap(m["Condition"]),
		}
		if sid, ok := m["Sid"].(string); ok {
			stmt.Sid = sid
		}
		if effect, ok := m["Effect"].(string); ok {
			stmt.Effect = effect
		}
		if principal, present := m["Principal"]; present {
			stmt.Principal = principal
		}
		stmt.Action = toStringSlice(m["Action"])
		stmt.NotAction = toStringSlice(m["NotAction"])
		stmt.Resource = toStringSlice(m["Resource"])
		stmt.NotResource = toStringSlice(m["NotResource"])
		doc.Statements = append(doc.Statements, stmt)
	}
	return doc, nil
}

// toStringSlice normalizes AWS's own real "either a bare string or an array of
// strings" shape (Action/Resource/NotAction/NotResource all allow both) — never
// guessed, only the two real shapes AWS itself documents.
func toStringSlice(v any) []string {
	switch val := v.(type) {
	case string:
		return []string{val}
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// toStringKeyedMap returns v as map[string]any when it already is one, else nil —
// Condition is always an object in real AWS policy documents; this just guards
// against a malformed document without fabricating structure.
func toStringKeyedMap(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return m
}

// mergeIAMPolicies attaches real identity and resource-based policy data onto the
// already-built node slice, by ID — the same "post-process onto already-built nodes"
// shape PC-112/113/115's own merge functions use. Three real Terraform shapes:
//
//   - aws_iam_role_policy: an INLINE identity policy, attached directly to its own
//     role attribute.
//   - aws_iam_policy + aws_iam_role_policy_attachment: a MANAGED, reusable policy
//     document, attached to a role via a separate attachment resource. An attachment
//     naming an AWS-managed policy ARN (a bare string, no resource of its own in this
//     bundle — e.g. "arn:aws:iam::aws:policy/ReadOnlyAccess") resolves to no
//     in-bundle policy document at all; a real, stated gap (nothing to merge, not a
//     guess at what that managed policy contains), never silently fabricated.
//   - aws_s3_bucket_policy: a RESOURCE-based policy, attached directly to the bucket
//     it names.
func mergeIAMPolicies(nodes []core.Node, parsed []ParsedResource) {
	byID := map[string]int{}
	for i, n := range nodes {
		byID[n.ID] = i
	}

	byKey := map[string]ParsedResource{}
	for _, r := range parsed {
		byKey[r.Key()] = r
	}
	managedPolicies := map[string]ParsedResource{}
	for _, r := range parsed {
		if r.Type == "aws_iam_policy" {
			managedPolicies[r.Key()] = r
		}
	}

	for _, r := range parsed {
		switch r.Type {
		case "aws_iam_role_policy":
			roleRefs, ok := r.AttributeReferences["role"]
			if !ok || len(roleRefs) != 1 {
				markEveryRoleUnresolved(nodes, byID, r)
				continue
			}
			attachIdentityPolicy(nodes, byID, roleRefs[0].Key(), r.Key(), r, byKey)

		case "aws_iam_role_policy_attachment":
			roleRefs, hasRole := r.AttributeReferences["role"]
			if !hasRole || len(roleRefs) != 1 {
				markEveryRoleUnresolved(nodes, byID, r)
				continue
			}
			policyRefs := r.AttributeReferences["policy_arn"]
			if len(policyRefs) == 1 {
				if managedPolicy, ok := managedPolicies[policyRefs[0].Key()]; ok {
					attachIdentityPolicy(nodes, byID, roleRefs[0].Key(), managedPolicy.Key(), managedPolicy, byKey)
					continue
				}
			}
			// An AWS-managed policy ARN (a literal string, no resource of its own), or a policy declared
			// outside this bundle: its permissions are not modelled. Recorded on the role (PC-157), never
			// read as "grants nothing".
			name := r.Key()
			if arn, ok := r.Attributes["policy_arn"].(string); ok && arn != "" {
				name = arn
			} else if len(policyRefs) == 1 {
				name = policyRefs[0].Key()
			}
			recordUnresolvedIdentityPolicy(nodes, byID, roleRefs[0].Key(), name,
				"an attached policy defined outside this bundle (for example an AWS-managed policy); its permissions are not modelled")

		case "aws_s3_bucket_policy":
			bucketRefs, ok := r.AttributeReferences["bucket"]
			if !ok || len(bucketRefs) != 1 {
				continue
			}
			idx, ok := byID[bucketRefs[0].Key()]
			if !ok {
				continue
			}
			doc, readable, _ := readPolicyAttribute(r, "policy", r.Key(), byKey)
			if !readable {
				continue // recorded on the bucket by stampResourcePolicies
			}
			nodes[idx].IAMResourcePolicy = &doc
		}
	}
}

// attachIdentityPolicy reads the policy document carried by res onto roleID. A document that cannot be
// read (not a static value, malformed JSON, absent) is recorded on the role as unresolved (PC-157): an
// unread policy must make the role's IAM evaluation not_assessable, never an implicit "grants nothing".
func attachIdentityPolicy(nodes []core.Node, byID map[string]int, roleID, policyID string, res ParsedResource, byKey map[string]ParsedResource) {
	idx, ok := byID[roleID]
	if !ok {
		return
	}
	doc, readable, why := readPolicyAttribute(res, "policy", policyID, byKey)
	if !readable {
		recordUnresolvedIdentityPolicy(nodes, byID, roleID, policyID, why)
		return
	}
	nodes[idx].IAMIdentityPolicies = append(nodes[idx].IAMIdentityPolicies, doc)
}

// UnresolvedIdentityPoliciesAttr is the RawAttributes key naming each identity policy ingest could not read.
const UnresolvedIdentityPoliciesAttr = "unresolved_identity_policies"

func recordUnresolvedIdentityPolicy(nodes []core.Node, byID map[string]int, roleID, policyID, reason string) {
	idx, ok := byID[roleID]
	if !ok {
		return
	}
	if nodes[idx].RawAttributes == nil {
		nodes[idx].RawAttributes = map[string]any{}
	}
	existing, _ := nodes[idx].RawAttributes[UnresolvedIdentityPoliciesAttr].([]string)
	existing = append(existing, policyID+": "+reason)
	sort.Strings(existing)
	nodes[idx].RawAttributes[UnresolvedIdentityPoliciesAttr] = existing
}

// markEveryRoleUnresolved is for a policy attachment whose role could not be read (PC-158): it may apply
// to any role, and it may grant anything, so no role's least-privilege result is safe.
func markEveryRoleUnresolved(nodes []core.Node, byID map[string]int, res ParsedResource) {
	if _, declared := res.AttrInfo["role"]; !declared {
		return // no role attribute at all: nothing was dropped
	}
	for _, n := range nodes {
		if n.Type == core.NodeTypeIdentity {
			recordUnresolvedIdentityPolicy(nodes, byID, n.ID, res.Key(),
				"a policy attachment names a role that could not be read, so it may apply to this role")
		}
	}
}
