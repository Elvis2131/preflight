// Package ingest is the HCL parser: the defined v1 subset (CLAUDE.md §15 —
// resource/data blocks, module calls, variables/locals, static references; count,
// for_each, dynamic blocks and complex interpolation are out of scope for v1, surfaced
// as unresolved rather than a parse error), plus the Minimum Viable Graph check.
//
// Source note: PC-12 cites PRD §5.1 and Design §2.2, neither of which was available
// when this was written. Built from PRD §4/§15 (already known via CLAUDE.md §8/§20)
// plus PC-12's own Card/Conversation, which states the MVG formula directly.
package ingest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

// reservedTraversalRoots are traversal roots that are never a resource reference —
// Terraform's own reserved identifiers (variables, locals, module outputs, data
// sources, and the count/for_each/self meta-arguments). v1 does not build edges from
// these (§15's scope: "static references" means resource-to-resource references
// specifically); a var/local/module/data reference simply contributes no edge, which is
// a stated scope limit, not a silent loss — see ParsedResource.Attributes, which still
// captures the field's literal value when this traversal appears inside a value that
// evaluates without needing that reference (rare) or is otherwise a documented gap.
var reservedTraversalRoots = map[string]bool{
	"var": true, "local": true, "module": true, "data": true,
	"count": true, "each": true, "self": true, "path": true, "terraform": true,
}

// ResourceRef is one static reference from one resource block to another, e.g.
// `aws_subnet.public_a.id` inside an aws_lb's `subnets` attribute.
type ResourceRef struct {
	ResourceType string
	ResourceName string
}

func (r ResourceRef) Key() string { return r.ResourceType + "." + r.ResourceName }

// ParsedResource is one `resource "TYPE" "NAME" { ... }` block, before any provider
// mapping is applied — this is ingest's own generic output, not one of PC-7's five
// frozen contracts.
type ParsedResource struct {
	Type string
	Name string

	// HasCount, HasForEach, HasDynamicBlock: PC-12's second acceptance criterion.
	// True here means this resource's node must be built with
	// core.ResolutionUnresolved — never a parse error, per §15.
	HasCount        bool
	HasForEach      bool
	HasDynamicBlock bool

	// Attributes holds only the attributes whose expression evaluated to a literal
	// value with no external variables and no functions requiring a runtime context —
	// i.e. exactly §15's "static" scope. An attribute using a function call, a
	// conditional, or string interpolation with a variable is simply absent here
	// (never fabricated, never a parse error) — RawAttributes on core.Node is
	// documented as "additional," not required, so this is a safe, honest omission.
	Attributes map[string]any

	// NonLiteralAttributes names the top-level attributes that ARE declared but whose
	// expression is not a literal (a variable, a function call). Attributes omits them, so
	// without this an unknown value and an absent one look the same (PC-156).
	NonLiteralAttributes map[string]bool

	// AttrInfo classifies every declared attribute, keyed by name (top level) or "block.attr" (one level
	// of nesting; a repeated block keeps the last occurrence here, see NestedBlocks[...].Info for each).
	AttrInfo map[string]AttrInfo

	// References are every resource-to-resource traversal found anywhere in this
	// resource's body (including inside function calls like jsonencode(), and inside
	// list/object literals) — hcl.Expression.Variables() walks nested expressions
	// automatically, so this is not a hand-rolled, easily-incomplete AST walk.
	References []ResourceRef

	// AttributeReferences is References broken out per source attribute name — needed
	// specifically so an edge-only mapping (providers/aws.EdgeMapping) can identify
	// which reference came from which named attribute (e.g. "resource_arn" vs
	// "web_acl_arn" on aws_wafv2_web_acl_association), which the flat, deduplicated
	// References list alone cannot answer.
	AttributeReferences map[string][]ResourceRef

	// NestedBlocks is PC-111's addition: every occurrence of every nested block type,
	// keyed by block type name, one map of {attribute name -> literal value / resource
	// reference} per occurrence — e.g. NestedBlocks["route"] for an aws_route_table
	// with multiple `route { ... }` blocks. This exists alongside (not replacing) the
	// single-flattened-dotted-key walk below (Attributes["vpc_config.endpoint_..."]):
	// that walk silently keeps only the LAST occurrence when a block type repeats,
	// which is invisible and wrong for anything (like route tables) where the SAME
	// resource can legitimately declare the same nested block type more than once.
	// Every value here is either a literal (captured the same way top-level Attributes
	// are) or, for an attribute that is itself a resource reference (e.g. a route's
	// gateway_id = aws_internet_gateway.x.id), the resolved ResourceRef — ingest/
	// build.go's route-building logic needs the reference target, not just whichever
	// literal values happen to be present alongside it.
	NestedBlocks map[string][]NestedBlock

	SourceFile string
	SourceLine int
}

// NestedBlock is one occurrence of a repeated nested block (see ParsedResource.
// NestedBlocks above).
type NestedBlock struct {
	Attributes map[string]any
	References map[string]ResourceRef
	// Info classifies every attribute declared in this occurrence (PC-158).
	Info map[string]AttrInfo
}

// AttrShape says how readable a declared attribute's expression is to ingest.
type AttrShape string

const (
	// ShapeLiteral: a constant ingest evaluated (including jsonencode of constants).
	ShapeLiteral AttrShape = "literal"
	// ShapeRefs: nothing but resource references (one, or a list of them), e.g. aws_subnet.a.id.
	ShapeRefs AttrShape = "refs"
	// ShapeOther: anything else: a variable, a function, a conditional, a template mixing text and
	// references. Ingest cannot read it, and PC-158 requires that to be visible, never silently absent.
	ShapeOther AttrShape = "other"
)

// AttrInfo is what ingest knows about one declared attribute: its shape and, for ShapeRefs, the
// resources it points at. It exists because an unreadable value and an absent one otherwise look the same
// downstream, and "absent" often means a documented default (PC-158).
type AttrInfo struct {
	Shape AttrShape
	Refs  []ResourceRef
}

// classifyAttr decides an attribute's shape from its expression.
func classifyAttr(expr hcl.Expression) AttrInfo {
	if _, ok := literalValue(expr); ok {
		return AttrInfo{Shape: ShapeLiteral}
	}
	if sx, ok := expr.(hclsyntax.Expression); ok {
		if travs, pure := pureTraversals(sx); pure {
			var refs []ResourceRef
			for _, tr := range travs {
				ref, ok := resourceRefFromTraversal(tr)
				if !ok {
					return AttrInfo{Shape: ShapeOther}
				}
				refs = append(refs, ref)
			}
			return AttrInfo{Shape: ShapeRefs, Refs: refs}
		}
	}
	return AttrInfo{Shape: ShapeOther}
}

// pureTraversals returns the traversals when expr is a single traversal or a list made only of them.
func pureTraversals(expr hclsyntax.Expression) ([]hcl.Traversal, bool) {
	switch e := expr.(type) {
	case *hclsyntax.ScopeTraversalExpr:
		return []hcl.Traversal{e.Traversal}, true
	case *hclsyntax.TupleConsExpr:
		var out []hcl.Traversal
		for _, el := range e.Exprs {
			sub, ok := pureTraversals(el)
			if !ok {
				return nil, false
			}
			out = append(out, sub...)
		}
		return out, true
	case *hclsyntax.ParenthesesExpr:
		return pureTraversals(e.Expression)
	}
	return nil, false
}

func (r ParsedResource) Key() string { return r.Type + "." + r.Name }

// ParseDir parses every *.tf file directly inside dir (non-recursive, matching the
// golden bundle's own flat layout) and returns one ParsedResource per top-level
// `resource` block. Non-resource blocks (provider, terraform, variable, locals, output,
// data) are read for context (so variables can eventually be cross-referenced) but do
// not themselves produce a ParsedResource — only `resource` blocks map onto IR nodes.
//
// A genuine HCL syntax error (malformed braces, invalid tokens) is still a real parse
// error — §15's "never raise a parse error" promise is specifically about count/
// for_each/dynamic/complex-interpolation, the v1 subset's own named exclusions, not
// about tolerating invalid HCL.
func ParseDir(dir string) ([]ParsedResource, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("ingest: read dir %s: %w", dir, err)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".tf" {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	sort.Strings(files) // deterministic order — CLAUDE.md §23: determinism, always.

	parser := hclparse.NewParser()
	var resources []ParsedResource

	for _, path := range files {
		f, diags := parser.ParseHCLFile(path)
		if diags.HasErrors() {
			return nil, fmt.Errorf("ingest: parse %s: %s", path, diags.Error())
		}
		body, ok := f.Body.(*hclsyntax.Body)
		if !ok {
			return nil, fmt.Errorf("ingest: %s: expected hclsyntax.Body, got %T", path, f.Body)
		}

		for _, block := range body.Blocks {
			if block.Type != "resource" || len(block.Labels) != 2 {
				continue
			}
			resources = append(resources, parseResourceBlock(block, path))
		}
	}

	return resources, nil
}

func parseResourceBlock(block *hclsyntax.Block, sourceFile string) ParsedResource {
	r := ParsedResource{
		Type:                 block.Labels[0],
		Name:                 block.Labels[1],
		Attributes:           map[string]any{},
		NonLiteralAttributes: map[string]bool{},
		AttrInfo:             map[string]AttrInfo{},
		AttributeReferences:  map[string][]ResourceRef{},
		SourceFile:           sourceFile,
		SourceLine:           block.TypeRange.Start.Line,
	}

	for name, attr := range block.Body.Attributes {
		if name == "count" {
			r.HasCount = true
		}
		if name == "for_each" {
			r.HasForEach = true
		}
		collectReferences(attr.Expr, &r.References)
		collectAttributeReferences(attr.Expr, name, r.AttributeReferences)
		r.AttrInfo[name] = classifyAttr(attr.Expr)
		if v, ok := literalValue(attr.Expr); ok {
			r.Attributes[name] = v
		} else {
			r.NonLiteralAttributes[name] = true
		}
	}

	if r.NestedBlocks == nil {
		r.NestedBlocks = map[string][]NestedBlock{}
	}
	for _, nested := range block.Body.Blocks {
		if nested.Type == "dynamic" {
			r.HasDynamicBlock = true
			continue
		}
		nb := NestedBlock{Attributes: map[string]any{}, References: map[string]ResourceRef{}, Info: map[string]AttrInfo{}}
		// Nested config blocks (vpc_config, scaling_config, ingress, ...) are walked
		// one level for references and literals too, under a dotted key
		// ("vpc_config.endpoint_public_access") — this is exactly the dotted
		// source_attribute path providers/aws's mapping YAML already uses. Alongside
		// that flattened view (which silently keeps only the LAST occurrence of a
		// repeated block type), every occurrence is also recorded in full under
		// r.NestedBlocks[nested.Type] — see that field's own doc comment for why.
		for name, attr := range nested.Body.Attributes {
			collectReferences(attr.Expr, &r.References)
			collectAttributeReferences(attr.Expr, name, r.AttributeReferences)
			info := classifyAttr(attr.Expr)
			nb.Info[name] = info
			r.AttrInfo[nested.Type+"."+name] = info
			if v, ok := literalValue(attr.Expr); ok {
				r.Attributes[nested.Type+"."+name] = v
				nb.Attributes[name] = v
			}
			for _, trav := range attr.Expr.Variables() {
				if ref, ok := resourceRefFromTraversal(trav); ok {
					nb.References[name] = ref
					break
				}
			}
		}
		r.NestedBlocks[nested.Type] = append(r.NestedBlocks[nested.Type], nb)
	}

	return r
}

// collectReferences appends every resource-to-resource traversal in expr to refs,
// deduplicated. Uses hcl.Expression.Variables(), which walks into function-call
// arguments, list/object literals, and conditionals automatically — this is why a
// reference inside jsonencode(...) (see golden/aws/queue.tf's redrive_policy) is found
// without any special-casing for that construct.
// collectAttributeReferences records the references found in one specific named
// attribute's expression — the per-attribute counterpart to collectReferences.
func collectAttributeReferences(expr hcl.Expression, attrName string, byAttr map[string][]ResourceRef) {
	for _, trav := range expr.Variables() {
		ref, ok := resourceRefFromTraversal(trav)
		if !ok {
			continue
		}
		byAttr[attrName] = append(byAttr[attrName], ref)
	}
}

func collectReferences(expr hcl.Expression, refs *[]ResourceRef) {
	seen := map[string]bool{}
	for _, r := range *refs {
		seen[r.Key()] = true
	}
	for _, trav := range expr.Variables() {
		ref, ok := resourceRefFromTraversal(trav)
		if !ok || seen[ref.Key()] {
			continue
		}
		seen[ref.Key()] = true
		*refs = append(*refs, ref)
	}
}

func resourceRefFromTraversal(trav hcl.Traversal) (ResourceRef, bool) {
	if len(trav) < 2 {
		return ResourceRef{}, false
	}
	root, ok := trav[0].(hcl.TraverseRoot)
	if !ok || reservedTraversalRoots[root.Name] {
		return ResourceRef{}, false
	}
	attr, ok := trav[1].(hcl.TraverseAttr)
	if !ok {
		return ResourceRef{}, false
	}
	return ResourceRef{ResourceType: root.Name, ResourceName: attr.Name}, true
}

// staticEvalContext lets literalValue evaluate jsonencode() of constants (PC-157): the way Terraform
// authors write an IAM policy document. jsonencode is pure and deterministic (object keys are sorted),
// and a call that references a resource or variable still fails to evaluate, so it stays non-literal.
// No other function is offered, so nothing needing a runtime context is evaluated.
var staticEvalContext = &hcl.EvalContext{Functions: map[string]function.Function{"jsonencode": stdlib.JSONEncodeFunc}}

// literalValue evaluates expr with a context that offers only jsonencode — per hcl.Expression's own contract,
// this succeeds only for a pure constant (no variables, no functions needing a runtime
// context). Anything else (a function call, a conditional, a variable reference, real
// string interpolation) fails here and is simply omitted from ParsedResource.Attributes
// — never fabricated, per §15's "complex interpolation... out of scope for v1."
func literalValue(expr hcl.Expression) (any, bool) {
	val, diags := expr.Value(staticEvalContext)
	if diags.HasErrors() || !val.IsWhollyKnown() {
		return nil, false
	}
	return ctyToGo(val)
}
