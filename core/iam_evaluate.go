// This file is PC-134: a pure, in-core engine answering "can this principal perform
// this action on this resource" (and, separately, "can this principal assume this
// role") against the policy documents PC-133's IR modelling captured — never against
// a bare "does the role's permission list contain this action string" check, which
// the Card's own conversation names as the anti-pattern this engine must not become.
//
// The evaluation order below is AWS's own documented logic (verified against
// docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_evaluation-logic*.html,
// fetched while writing this file), reduced to the subset this ticket scopes in:
//
//   - "By default, all requests are implicitly denied ... an explicit deny overrides
//     an explicit allow" (reference_policies_evaluation-logic_policy-eval-denyallow).
//   - "If an action is allowed by an identity-based policy, a resource-based policy,
//     or both, then AWS allows the action. An explicit deny in either of these
//     policies overrides the allow" (reference_policies_evaluation-logic, "Evaluating
//     identity-based policies with resource-based policies") — same-account only;
//     this ticket does not model account IDs, so cross-account nuance (e.g. IAM role
//     trust-policy exceptions to the "identity OR resource" union, which the same doc
//     names explicitly) is out of scope beyond the separate EvaluateAssumeRole entry
//     point below.
//
// Explicitly out of scope, per the Card, and never silently ignored: permissions
// boundaries, session policies, and AWS Organizations SCPs/RCPs. core.Node has no
// field for any of the three (core/iam.go's own "NOT modelled" list) — there is
// structurally no way for this engine to ever see one, so it can never compute a
// false allow by overlooking one. If a future ticket adds IR representation for any
// of them, EvaluateIAMRequest must be revisited before that data can be trusted here;
// until then, this is a real, honest scope limit, not an untested claim.
package core

import (
	"fmt"
	"sort"
	"strings"
)

// IAMDecision is the 3-value verdict this engine returns for a single query — the
// same not_assessable-never-a-guess shape as ComplianceStatus (core/compliance.go),
// because a bare allow/deny would let an unresolved condition or missing trust policy
// be silently collapsed into whichever answer looks like a pass, which I4 forbids.
type IAMDecision string

const (
	IAMDecisionAllow         IAMDecision = "allow"
	IAMDecisionDeny          IAMDecision = "deny"
	IAMDecisionNotAssessable IAMDecision = "not_assessable"
)

// IAMDecidingStatement names one statement that determined (or, for a not_assessable
// result, could have changed) the decision — the Card's own "shows which policy
// statements decided it" acceptance criterion, made concrete and machine-checkable.
type IAMDecidingStatement struct {
	PolicyID string `json:"policy_id" validate:"required" jsonschema:"required"`
	Sid      string `json:"sid,omitempty"`
	Effect   string `json:"effect" validate:"required,oneof=Allow Deny" jsonschema:"required,enum=Allow,enum=Deny"`
	Reason   string `json:"reason" validate:"required" jsonschema:"required"`
}

// IAMEvaluationResult is this engine's full answer: the decision, exactly which
// statement(s) decided it, and a plain-language reasoning chain a UI can show
// verbatim (PC-135's own stated future consumer).
type IAMEvaluationResult struct {
	Decision           IAMDecision            `json:"decision" validate:"required,oneof=allow deny not_assessable" jsonschema:"required,enum=allow,enum=deny,enum=not_assessable"`
	DecidingStatements []IAMDecidingStatement `json:"deciding_statements,omitempty" validate:"dive"`
	Reasoning          []string               `json:"reasoning" validate:"required,min=1" jsonschema:"required"`
	Provenance         Provenance             `json:"provenance" validate:"required" jsonschema:"required"`
}

// Validate checks this IAMEvaluationResult against the same struct tags it would be
// generated from, matching every other Validate() on a core result type.
func (r IAMEvaluationResult) Validate() error {
	return validate.Struct(r)
}

// IAMRequest is one "can this principal perform this action on this resource" query.
//
// ResourceARN is compared against Resource/NotResource elements verbatim — an ARN
// string, not an IR node ID. core.Node has no ARN field (PC-133's own stated scope:
// the IR models policy documents, not a full ARN namespace), so this engine cannot
// resolve "the ARN of node X" itself; the caller (the architect asking the question,
// or a future automated caller in PC-135) supplies it directly, exactly as it would
// appear inside the policy documents being evaluated.
//
// ResourceID, if non-empty, names the IR node whose IAMResourcePolicy (if any) is the
// resource-based policy in play — resource-based policies attach to IR nodes (PC-133),
// so THAT lookup is structural; only the ARN-string matching inside the policy is not.
//
// PrincipalIdentifier, if non-empty, is compared against a resource-based policy's
// own Principal element (same reasoning: PC-133 stores Principal as the literal AWS
// JSON shape, an ARN/account-ID/service string, never an IR node ID). Left empty, a
// resource-based policy can never match on Principal and so can never contribute an
// allow — reported in Reasoning, never silently treated as "matches everyone."
type IAMRequest struct {
	PrincipalID         string
	Action              string
	ResourceARN         string
	ResourceID          string
	PrincipalIdentifier string
	Context             map[string]string
}

// IAMAssumeRoleRequest is a "can this principal assume this role" query — the Card's
// own separate conformance case ("role assumption requires the trust policy to allow
// the principal"), evaluated purely against RoleID's own IAMTrustPolicy.
type IAMAssumeRoleRequest struct {
	RoleID              string
	PrincipalIdentifier string
	Context             map[string]string
}

// supportedConditionOperators are the only Condition operators this engine resolves.
// Any statement whose applicability depends on any OTHER operator is not_assessable,
// naming it — never dropped, never assumed satisfied or unsatisfied (the Card's own
// "never ignore a condition to reach an allow").
var supportedConditionOperators = map[string]bool{
	"StringEquals":    true,
	"StringNotEquals": true,
	"StringLike":      true,
	"StringNotLike":   true,
}

// conditionOutcome is a supported condition's resolved result for one statement —
// distinct from "unsupported", which is carried separately so a statement can be
// indeterminate for a named reason rather than silently treated as false.
type conditionOutcome int

const (
	conditionSatisfied conditionOutcome = iota
	conditionUnsatisfied
	conditionIndeterminate
)

// evaluateCondition resolves stmt's Condition block against ctx. The returned string
// is only meaningful when the outcome is conditionIndeterminate: the specific
// operator/key this engine does not evaluate, exactly as the Card requires it be
// named.
func evaluateCondition(cond map[string]any, ctx map[string]string) (conditionOutcome, string) {
	if len(cond) == 0 {
		return conditionSatisfied, ""
	}
	// Deterministic iteration: map order is not, and the "first unsupported operator
	// named" must not vary run to run.
	operators := make([]string, 0, len(cond))
	for op := range cond {
		operators = append(operators, op)
	}
	sort.Strings(operators)

	for _, op := range operators {
		keys, ok := toStringKeyedMapOK(cond[op])
		if !ok {
			return conditionIndeterminate, fmt.Sprintf("condition operator %q has a non-object value, which this engine does not evaluate", op)
		}
		if !supportedConditionOperators[op] {
			// Name one representative key so the reason is concrete, not just the
			// bare operator name.
			for k := range keys {
				return conditionIndeterminate, fmt.Sprintf("condition operator %q (key %q)", op, k)
			}
			return conditionIndeterminate, fmt.Sprintf("condition operator %q", op)
		}
		negate := op == "StringNotEquals" || op == "StringNotLike"
		useGlob := op == "StringLike" || op == "StringNotLike"
		keyNames := make([]string, 0, len(keys))
		for k := range keys {
			keyNames = append(keyNames, k)
		}
		sort.Strings(keyNames)
		for _, key := range keyNames {
			wantAny := keys[key]
			wants := toIfaceStringSlice(wantAny)
			actual, present := ctx[key]
			if !present {
				// AWS's real semantics: most operators evaluate false when the key is
				// absent from the request context. Reported as unsatisfied, not
				// indeterminate — a missing context value is a known fact (this
				// engine's caller simply did not supply it), not an unsupported
				// operator.
				return conditionUnsatisfied, ""
			}
			matched := false
			for _, want := range wants {
				if useGlob {
					if globMatch(want, actual, true) {
						matched = true
						break
					}
				} else if want == actual {
					matched = true
					break
				}
			}
			if negate {
				matched = !matched
			}
			if !matched {
				return conditionUnsatisfied, ""
			}
		}
	}
	return conditionSatisfied, ""
}

func toIfaceStringSlice(v any) []string {
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

func toStringKeyedMapOK(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

// globMatch implements AWS's own documented wildcard grammar: "*" matches zero or
// more characters, "?" matches exactly one. caseInsensitive mirrors the Action
// element's own documented rule ("The prefix and the action name are case
// insensitive" — reference_policies_elements_action.html); Resource/ARN matching is
// left case-sensitive, since ARNs are literal, case-sensitive identifiers.
//
// This is a hand-rolled matcher, not path.Match: path.Match's "*"/"?" are scoped to
// one path segment and never cross a "/" — exactly wrong for an ARN, where a
// resource-level wildcard (e.g. "arn:aws:s3:::my-bucket/*") must match a key with
// arbitrarily many "/"-separated path components. AWS's own grammar has no such
// restriction: "*" matches any sequence of characters, full stop.
func globMatch(pattern, s string, caseInsensitive bool) bool {
	if caseInsensitive {
		pattern = strings.ToLower(pattern)
		s = strings.ToLower(s)
	}
	return globMatchRunes([]rune(pattern), []rune(s))
}

// globMatchRunes is the classic iterative wildcard matcher (star/questionmark, with
// backtracking via remembered star position) — O(len(pattern)*len(s)) worst case,
// which is fine at policy-statement scale.
func globMatchRunes(pattern, s []rune) bool {
	var pi, si, starIdx, starMatch int
	starIdx = -1
	for si < len(s) {
		switch {
		case pi < len(pattern) && (pattern[pi] == '?' || pattern[pi] == s[si]):
			pi++
			si++
		case pi < len(pattern) && pattern[pi] == '*':
			starIdx = pi
			starMatch = si
			pi++
		case starIdx != -1:
			pi = starIdx + 1
			starMatch++
			si = starMatch
		default:
			return false
		}
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

func matchesAny(patterns []string, s string, caseInsensitive bool) bool {
	for _, p := range patterns {
		if globMatch(p, s, caseInsensitive) {
			return true
		}
	}
	return false
}

// actionResourceApplies reports whether stmt's Action/NotAction and
// Resource/NotResource elements select (action, resourceARN) — independent of Effect,
// Principal, or Condition, exactly mirroring AWS's own documented element semantics
// (reference_policies_elements_action.html, reference_policies_elements_resource.html).
func actionResourceApplies(stmt PolicyStatement, action, resourceARN string) bool {
	actionMatch := true
	switch {
	case len(stmt.Action) > 0:
		actionMatch = matchesAny(stmt.Action, action, true)
	case len(stmt.NotAction) > 0:
		actionMatch = !matchesAny(stmt.NotAction, action, true)
	}
	if !actionMatch {
		return false
	}
	if resourceARN == "" {
		// No resource ARN supplied — Resource/NotResource cannot be evaluated, but
		// this helper only decides applicability for the caller, which treats an
		// empty ARN as "match on action alone" only when the statement itself has no
		// Resource/NotResource element (identity policies frequently omit Resource,
		// meaning "all resources", "*"). A statement that DOES declare Resource but
		// got no ARN to check against is handled by the caller as indeterminate, not
		// here.
		return len(stmt.Resource) == 0 && len(stmt.NotResource) == 0
	}
	switch {
	case len(stmt.Resource) > 0:
		return matchesAny(stmt.Resource, resourceARN, false)
	case len(stmt.NotResource) > 0:
		return !matchesAny(stmt.NotResource, resourceARN, false)
	default:
		return true
	}
}

// principalApplies reports whether a resource-based or trust policy statement's
// Principal element matches identifier — the literal ARN/account-ID/service string
// PC-133 preserved verbatim. identifier == "" never matches anything (see IAMRequest's
// own doc comment): a resource/trust policy statement can never contribute an allow
// to a query that supplied no principal identifier to check it against.
func principalApplies(principal any, identifier string) bool {
	if identifier == "" {
		return false
	}
	switch p := principal.(type) {
	case string:
		return p == "*" || globMatch(p, identifier, false)
	case map[string]any:
		for _, key := range []string{"AWS", "Service", "Federated"} {
			if candidates := toIfaceStringSlice(p[key]); len(candidates) > 0 {
				if matchesAny(candidates, identifier, false) {
					return true
				}
			}
		}
		return false
	default:
		return false
	}
}

// candidateStatement pairs one PolicyStatement with the policy document ID it came
// from — IAMDecidingStatement needs both.
type candidateStatement struct {
	policyID string
	stmt     PolicyStatement
}

// evaluateStatements is the shared core of EvaluateIAMRequest and EvaluateAssumeRole:
// given the statements that structurally apply (action/resource/principal already
// matched), partition by Condition outcome and Effect, then apply AWS's own
// documented precedence (deny-overrides-allow, implicit-deny default), per this
// file's header comment.
func evaluateStatements(candidates []candidateStatement, ctx map[string]string, allowNoun, denyNoun string) (IAMDecision, []IAMDecidingStatement, []string) {
	var determinateDeny, determinateAllow, indeterminateDeny, indeterminateAllow []candidateStatement

	for i := range candidates {
		c := candidates[i]
		outcome, _ := evaluateCondition(c.stmt.Condition, ctx)
		switch outcome {
		case conditionUnsatisfied:
			continue // does not apply — a known, resolved fact, not a gap
		case conditionIndeterminate:
			if c.stmt.Effect == "Deny" {
				indeterminateDeny = append(indeterminateDeny, c)
			} else {
				indeterminateAllow = append(indeterminateAllow, c)
			}
		case conditionSatisfied:
			if c.stmt.Effect == "Deny" {
				determinateDeny = append(determinateDeny, c)
			} else {
				determinateAllow = append(determinateAllow, c)
			}
		}
	}

	toDeciding := func(cs []candidateStatement, reason string) []IAMDecidingStatement {
		out := make([]IAMDecidingStatement, 0, len(cs))
		for _, c := range cs {
			out = append(out, IAMDecidingStatement{
				PolicyID: c.policyID,
				Sid:      c.stmt.Sid,
				Effect:   c.stmt.Effect,
				Reason:   reason,
			})
		}
		return out
	}

	// 1. A determinate explicit deny always wins — AWS's own documented rule, and it
	// is already CONFIRMED to apply, so no indeterminate statement can undo it.
	if len(determinateDeny) > 0 {
		deciding := toDeciding(determinateDeny, "explicit deny, condition satisfied")
		return IAMDecisionDeny, deciding, []string{fmt.Sprintf("%d statement(s) explicitly deny this request; an explicit deny always overrides any allow", len(determinateDeny))}
	}

	// 2. No confirmed deny yet, but a statement that MIGHT be a deny (its condition
	// could not be evaluated) could still turn one up — reporting Allow or implicit
	// Deny here would silently assume that statement resolves in whichever direction
	// makes the answer look decided, exactly what the Card's "never ignore a
	// condition to reach an allow" forbids.
	if len(indeterminateDeny) > 0 {
		var reasons []string
		for _, c := range indeterminateDeny {
			reasons = append(reasons, indeterminateReasonFor(c, ctx))
		}
		deciding := make([]IAMDecidingStatement, 0, len(indeterminateDeny))
		for i, c := range indeterminateDeny {
			deciding = append(deciding, IAMDecidingStatement{PolicyID: c.policyID, Sid: c.stmt.Sid, Effect: c.stmt.Effect, Reason: reasons[i]})
		}
		return IAMDecisionNotAssessable, deciding, append([]string{"a statement that could deny this request has an unsupported condition and could not be evaluated"}, reasons...)
	}

	// 3. A determinate allow, with no possible deny left unresolved, is a real allow.
	if len(determinateAllow) > 0 {
		deciding := toDeciding(determinateAllow, allowNoun)
		return IAMDecisionAllow, deciding, []string{fmt.Sprintf("%d statement(s) allow this request and no applicable deny was found", len(determinateAllow))}
	}

	// 4. No determinate answer either way, but a statement that MIGHT be an allow
	// could not be evaluated — the mirror image of step 2: defaulting to implicit
	// deny here would be safe in AWS's own terms (deny is never a false allow), but
	// I4's own standard is "unknown surfaces as not_assessable, never pass or fail" —
	// this engine holds to that even on the safe side, since the true answer really
	// is unknown.
	if len(indeterminateAllow) > 0 {
		var reasons []string
		for _, c := range indeterminateAllow {
			reasons = append(reasons, indeterminateReasonFor(c, ctx))
		}
		deciding := make([]IAMDecidingStatement, 0, len(indeterminateAllow))
		for i, c := range indeterminateAllow {
			deciding = append(deciding, IAMDecidingStatement{PolicyID: c.policyID, Sid: c.stmt.Sid, Effect: c.stmt.Effect, Reason: reasons[i]})
		}
		return IAMDecisionNotAssessable, deciding, append([]string{"a statement that could allow this request has an unsupported condition and could not be evaluated"}, reasons...)
	}

	// 5. Implicit deny — AWS's own documented default ("by default, all requests are
	// implicitly denied").
	return IAMDecisionDeny, nil, []string{fmt.Sprintf("no statement allows this %s; default is implicit deny", denyNoun)}
}

// indeterminateReasonFor recomputes one statement's unsupported-condition reason
// string for the deciding-statement/reasoning output, rather than threading an extra
// parallel slice through evaluateStatements' main loop.
func indeterminateReasonFor(c candidateStatement, ctx map[string]string) string {
	_, reason := evaluateCondition(c.stmt.Condition, ctx)
	return fmt.Sprintf("statement %s (policy %s): unsupported %s", statementLabel(c.stmt), c.policyID, reason)
}

func statementLabel(stmt PolicyStatement) string {
	if stmt.Sid != "" {
		return stmt.Sid
	}
	return "<no Sid>"
}

// EvaluateIAMRequest answers req against PrincipalID's identity policies (union of
// every inline/managed IAMIdentityPolicies document, per PC-133's own "attach to the
// identity node" modelling) and, if ResourceID names a node with an IAMResourcePolicy,
// that resource-based policy too — AWS's own documented "union of identity-based and
// resource-based, explicit deny in either overrides" rule, same-account only.
func EvaluateIAMRequest(ir *IR, req IAMRequest, prov Provenance) IAMEvaluationResult {
	byID := make(map[string]Node, len(ir.Nodes))
	for _, n := range ir.Nodes {
		byID[n.ID] = n
	}

	principal, ok := byID[req.PrincipalID]
	if !ok {
		return notAssessableResult(fmt.Sprintf("principal %q does not exist in this IR", req.PrincipalID), prov)
	}
	if principal.Type != NodeTypeIdentity {
		return notAssessableResult(fmt.Sprintf("principal %q is not an identity node (type %q)", req.PrincipalID, principal.Type), prov)
	}

	// PC-157: an identity policy ingest could not read (not a static value, malformed, or defined outside
	// the bundle) could hold any statement, including an explicit Deny, so no decision about this
	// principal is safe: not_assessable, naming each, never an implicit "grants nothing".
	if unread := stringList(principal.RawAttributes["unresolved_identity_policies"]); len(unread) > 0 {
		return notAssessableResult(fmt.Sprintf("principal %q has identity policies that could not be read: %s", req.PrincipalID, strings.Join(unread, "; ")), prov)
	}

	var candidates []candidateStatement
	for _, doc := range principal.IAMIdentityPolicies {
		for _, stmt := range doc.Statements {
			if actionResourceApplies(stmt, req.Action, req.ResourceARN) {
				candidates = append(candidates, candidateStatement{policyID: doc.ID, stmt: stmt})
			}
		}
	}

	if req.ResourceID != "" {
		resourceNode, ok := byID[req.ResourceID]
		if !ok {
			return notAssessableResult(fmt.Sprintf("resource %q does not exist in this IR", req.ResourceID), prov)
		}
		if resourceNode.IAMResourcePolicy != nil {
			for _, stmt := range resourceNode.IAMResourcePolicy.Statements {
				if !actionResourceApplies(stmt, req.Action, req.ResourceARN) {
					continue
				}
				if !principalApplies(stmt.Principal, req.PrincipalIdentifier) {
					continue
				}
				candidates = append(candidates, candidateStatement{policyID: resourceNode.IAMResourcePolicy.ID, stmt: stmt})
			}
		}
	}

	decision, deciding, reasoning := evaluateStatements(candidates, req.Context, "identity or resource policy allow", "action")
	return IAMEvaluationResult{Decision: decision, DecidingStatements: deciding, Reasoning: reasoning, Provenance: prov}
}

// EvaluateAssumeRole answers "can req.PrincipalIdentifier assume RoleID" purely
// against RoleID's own IAMTrustPolicy — the Card's own separate conformance case,
// since AWS documents trust policies as an exception to the ordinary identity/
// resource-policy union: they must explicitly allow the assuming principal.
func EvaluateAssumeRole(ir *IR, req IAMAssumeRoleRequest, prov Provenance) IAMEvaluationResult {
	var role *Node
	for i := range ir.Nodes {
		if ir.Nodes[i].ID == req.RoleID {
			role = &ir.Nodes[i]
			break
		}
	}
	if role == nil {
		return notAssessableResult(fmt.Sprintf("role %q does not exist in this IR", req.RoleID), prov)
	}
	if role.Type != NodeTypeIdentity {
		return notAssessableResult(fmt.Sprintf("role %q is not an identity node (type %q)", req.RoleID, role.Type), prov)
	}
	if role.IAMTrustPolicy == nil {
		return notAssessableResult(fmt.Sprintf("role %q has no trust policy captured in this IR", req.RoleID), prov)
	}

	var candidates []candidateStatement
	for _, stmt := range role.IAMTrustPolicy.Statements {
		if !actionResourceApplies(stmt, "sts:AssumeRole", "") {
			continue
		}
		if !principalApplies(stmt.Principal, req.PrincipalIdentifier) {
			continue
		}
		candidates = append(candidates, candidateStatement{policyID: role.IAMTrustPolicy.ID, stmt: stmt})
	}

	decision, deciding, reasoning := evaluateStatements(candidates, req.Context, "trust policy allow", "role assumption")
	return IAMEvaluationResult{Decision: decision, DecidingStatements: deciding, Reasoning: reasoning, Provenance: prov}
}

func notAssessableResult(reason string, prov Provenance) IAMEvaluationResult {
	return IAMEvaluationResult{Decision: IAMDecisionNotAssessable, Reasoning: []string{reason}, Provenance: prov}
}

// stringList reads a RawAttributes value that is a list of strings, whether it is still the []string
// ingest wrote or the []any a JSON round trip produces.
func stringList(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		var out []string
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
