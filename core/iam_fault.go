// This file is PC-135's second integration point, verbatim from the Card: "add
// iam_policy_change as a configuration-change fault (remove a statement, add a
// deny), re-running traces." Same copy-on-write IR-mutation discipline PC-129's
// core/nat_fault.go established (WithRouteRemoved/WithNATGatewayLost): return a COPY
// of ir with only the targeted node's own policy document changed, never mutate the
// caller's IR in place.
package core

// WithIAMStatementRemoved returns a copy of ir with the statement carrying sid
// removed from the policy document identified by policyID (an IAMIdentityPolicies[i],
// IAMTrustPolicy, or IAMResourcePolicy on some node — PC-133's own three attachment
// points). ok is false, and ir is returned unchanged, when policyID or sid cannot be
// found — the caller (core/simulate.go's resolveFaults) is expected to surface that
// as a real error, never silently apply a no-op fault.
func WithIAMStatementRemoved(ir *IR, policyID, sid string) (out *IR, ok bool) {
	return mutateIAMPolicy(ir, policyID, func(doc PolicyDocument) (PolicyDocument, bool) {
		idx := -1
		for i, s := range doc.Statements {
			if s.Sid == sid {
				idx = i
				break
			}
		}
		if idx == -1 {
			return doc, false
		}
		doc.Statements = append(append([]PolicyStatement{}, doc.Statements[:idx]...), doc.Statements[idx+1:]...)
		return doc, true
	})
}

// WithIAMDenyStatementAdded returns a copy of ir with stmt appended (Effect forced to
// "Deny" — this fault only ever injects a NEW restriction, never a new grant, which
// would make a fault injection silently more permissive than the real design) to the
// policy document identified by policyID.
func WithIAMDenyStatementAdded(ir *IR, policyID string, stmt PolicyStatement) (out *IR, ok bool) {
	stmt.Effect = "Deny"
	return mutateIAMPolicy(ir, policyID, func(doc PolicyDocument) (PolicyDocument, bool) {
		doc.Statements = append(append([]PolicyStatement{}, doc.Statements...), stmt)
		return doc, true
	})
}

// mutateIAMPolicy finds the one node/policy-document pair matching policyID across
// all three PC-133 attachment points and applies mutate to a COPY of that document,
// leaving every other node (and the original ir) untouched. Exactly one of a node's
// IAMIdentityPolicies entries, IAMTrustPolicy, or IAMResourcePolicy can ever match a
// given policyID, since PC-133's own ingest gives every policy document a unique ID
// derived from its own originating Terraform resource key.
func mutateIAMPolicy(ir *IR, policyID string, mutate func(PolicyDocument) (PolicyDocument, bool)) (*IR, bool) {
	nodes := make([]Node, len(ir.Nodes))
	copy(nodes, ir.Nodes)

	for i, n := range nodes {
		for j, doc := range n.IAMIdentityPolicies {
			if doc.ID != policyID {
				continue
			}
			mutated, ok := mutate(doc)
			if !ok {
				return ir, false
			}
			docs := make([]PolicyDocument, len(n.IAMIdentityPolicies))
			copy(docs, n.IAMIdentityPolicies)
			docs[j] = mutated
			n.IAMIdentityPolicies = docs
			nodes[i] = n
			return &IR{SchemaVersion: ir.SchemaVersion, VersionNumber: ir.VersionNumber, VersionHash: ir.VersionHash, Nodes: nodes, Edges: ir.Edges}, true
		}
		if n.IAMTrustPolicy != nil && n.IAMTrustPolicy.ID == policyID {
			mutated, ok := mutate(*n.IAMTrustPolicy)
			if !ok {
				return ir, false
			}
			n.IAMTrustPolicy = &mutated
			nodes[i] = n
			return &IR{SchemaVersion: ir.SchemaVersion, VersionNumber: ir.VersionNumber, VersionHash: ir.VersionHash, Nodes: nodes, Edges: ir.Edges}, true
		}
		if n.IAMResourcePolicy != nil && n.IAMResourcePolicy.ID == policyID {
			mutated, ok := mutate(*n.IAMResourcePolicy)
			if !ok {
				return ir, false
			}
			n.IAMResourcePolicy = &mutated
			nodes[i] = n
			return &IR{SchemaVersion: ir.SchemaVersion, VersionNumber: ir.VersionNumber, VersionHash: ir.VersionHash, Nodes: nodes, Edges: ir.Edges}, true
		}
	}
	return ir, false
}
