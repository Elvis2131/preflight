// This file is PC-159: IAM policies that reference resources in the bundle (aws_s3_bucket.data.arn) become
// assessable. The concrete ARN is only known after apply, but the engine does not need it: the reference
// identifies WHICH in-bundle resource a statement targets, which is enough to answer the least-privilege
// questions that matter (is the resource scoped or a wildcard; does this role reach that bucket). Each
// reference is bound to a placeholder, "preflight-ref:<resource key>", the document is evaluated and parsed
// like any other, and core treats a placeholder as a specific, non-wildcard target (core.SymbolicResourcePrefix).
//
// Rules, from the ticket:
//   - a reference to a resource declared in this bundle ("<type>.<name>.arn") resolves, tagged derived;
//   - a reference to anything else (a data source, a variable, a local, a resource not in the bundle, an
//     attribute other than arn) leaves the policy unreadable, naming the reference;
//   - the common object-path suffix, "${<type>.<name>.arn}/*", resolves to "objects within resource X";
//     any other text combined with a reference stays unreadable rather than guessed;
//   - a reference anywhere except a statement's Resource element stays unreadable (a principal or a
//     condition value would need the concrete ARN).
//
// Not attempted: partial evaluation of arbitrary expressions.
package ingest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"

	"preflight/core"
)

// evalSymbolicPolicy evaluates a policy expression that references in-bundle resources. It returns the JSON
// text, the resources referenced, and, when it cannot be read, why.
func evalSymbolicPolicy(expr hcl.Expression, byKey map[string]ParsedResource) (jsonText string, refs []string, problem string) {
	byType := map[string]map[string]cty.Value{}
	seen := map[string]bool{}
	for _, trav := range expr.Variables() {
		root, ok := trav[0].(hcl.TraverseRoot)
		if !ok {
			return "", nil, "an expression ingest cannot evaluate"
		}
		if reservedTraversalRoots[root.Name] {
			return "", nil, "it references " + traversalText(trav) + ", which is not a resource in this bundle"
		}
		if len(trav) != 3 {
			return "", nil, "it references " + traversalText(trav) + ", which is not a resource ARN"
		}
		name, ok1 := trav[1].(hcl.TraverseAttr)
		attr, ok2 := trav[2].(hcl.TraverseAttr)
		if !ok1 || !ok2 || attr.Name != "arn" {
			return "", nil, "it references " + traversalText(trav) + ", but only a resource's arn can stand for it symbolically"
		}
		key := root.Name + "." + name.Name
		if _, declared := byKey[key]; !declared {
			return "", nil, "it references " + key + ", which is not declared in this bundle"
		}
		if byType[root.Name] == nil {
			byType[root.Name] = map[string]cty.Value{}
		}
		byType[root.Name][name.Name] = cty.ObjectVal(map[string]cty.Value{"arn": cty.StringVal(core.SymbolicResourcePrefix + key)})
		if !seen[key] {
			seen[key] = true
			refs = append(refs, key)
		}
	}
	vars := map[string]cty.Value{}
	for typ, names := range byType {
		vars[typ] = cty.ObjectVal(names)
	}
	ctx := &hcl.EvalContext{Variables: vars, Functions: map[string]function.Function{"jsonencode": stdlib.JSONEncodeFunc}}
	val, diags := expr.Value(ctx)
	if diags.HasErrors() || !val.IsWhollyKnown() || val.IsNull() || val.Type() != cty.String {
		return "", nil, "the policy expression could not be evaluated statically"
	}
	sort.Strings(refs)
	return val.AsString(), refs, ""
}

func traversalText(trav hcl.Traversal) string {
	var b strings.Builder
	for i, step := range trav {
		switch s := step.(type) {
		case hcl.TraverseRoot:
			b.WriteString(s.Name)
		case hcl.TraverseAttr:
			b.WriteString("." + s.Name)
		default:
			if i > 0 {
				b.WriteString("[...]")
			}
		}
	}
	return b.String()
}

// checkSymbolicPlacement enforces the placement rules on a parsed document: a placeholder may appear only as a
// whole Resource element, optionally followed by "/*".
func checkSymbolicPlacement(doc core.PolicyDocument, refs []string) string {
	allowed := map[string]bool{}
	for _, ref := range refs {
		allowed[core.SymbolicResourcePrefix+ref] = true
		allowed[core.SymbolicResourcePrefix+ref+"/*"] = true
	}
	has := func(v any) bool { return strings.Contains(fmt.Sprint(v), core.SymbolicResourcePrefix) }
	for i, st := range doc.Statements {
		if has(st.Principal) || has(st.Action) || has(st.NotAction) || has(st.Condition) {
			return fmt.Sprintf("statement %d uses a resource reference outside its Resource element, which needs the concrete ARN", i)
		}
		if has(st.NotResource) {
			return fmt.Sprintf("statement %d uses a resource reference in NotResource, which is not supported", i)
		}
		for _, r := range st.Resource {
			// Compare with the references actually used: a resource name may itself contain a hyphen, so a pattern
			// alone cannot tell "<name>-backup" (text appended to the reference) from a resource named that.
			if strings.Contains(r, core.SymbolicResourcePrefix) && !allowed[r] {
				return fmt.Sprintf("statement %d combines a resource reference with text other than the /* object suffix (%q)", i, r)
			}
		}
	}
	return ""
}

// readPolicyAttribute reads a policy document attribute of res: a literal string, or an expression whose only
// non-constant parts are in-bundle resource ARNs. ok is false, with why, when it cannot be read.
func readPolicyAttribute(res ParsedResource, attr, policyID string, byKey map[string]ParsedResource) (doc core.PolicyDocument, ok bool, why string) {
	prov := core.NewProvenance(core.KindStated, "ingest/iam:"+policyID)
	if text, isLiteral := res.Attributes[attr].(string); isLiteral && text != "" {
		d, err := parsePolicyDocument(policyID, text, prov)
		if err != nil {
			return core.PolicyDocument{}, false, "the policy document is not valid IAM policy JSON"
		}
		return d, true, ""
	}
	expr, declared := res.Exprs[attr]
	if !declared {
		return core.PolicyDocument{}, false, "the " + attr + " attribute is absent"
	}
	text, refs, problem := evalSymbolicPolicy(expr, byKey)
	if problem != "" {
		return core.PolicyDocument{}, false, "the policy document is not a static value: " + problem
	}
	prov = core.NewProvenance(core.KindDerived, "ingest/iam:"+policyID).
		WithReason("resource references resolved symbolically (each stands for a specific resource declared in this bundle): " + strings.Join(refs, ", "))
	d, err := parsePolicyDocument(policyID, text, prov)
	if err != nil {
		return core.PolicyDocument{}, false, "the policy document is not valid IAM policy JSON"
	}
	if bad := checkSymbolicPlacement(d, refs); bad != "" {
		return core.PolicyDocument{}, false, bad
	}
	return d, true, ""
}
