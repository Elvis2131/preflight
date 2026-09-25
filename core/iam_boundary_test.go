package core_test

// PC-135's fourth acceptance criterion, verbatim: "Grep/boundary test confirms no
// IAM evaluation logic outside PC-134's package." Every consumer this ticket adds
// (core/trace_iam.go, core/iam_fault.go, core/iam_compliance_findings.go,
// core/journey_flow.go) is required to call PC-134's own two public entry points
// (EvaluateIAMRequest/EvaluateAssumeRole) and nothing else — the actual matching/
// precedence/condition logic (globMatch, actionResourceApplies, principalApplies,
// evaluateCondition, evaluateStatements) must be defined, and used, ONLY inside
// core/iam_evaluate.go itself. Same go/ast technique core/load_no_instance_count_test.go
// and core/sizing_test.go already established for a structural "never references X"
// guarantee.
import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

// iamInternalIdentifiers are core/iam_evaluate.go's own unexported evaluation
// primitives — never the exported IAMDecision/IAMEvaluationResult/EvaluateIAMRequest/
// EvaluateAssumeRole names, which every consumer is expected to reference.
var iamInternalIdentifiers = []string{
	"globMatch", "globMatchRunes", "matchesAny", "actionResourceApplies",
	"principalApplies", "evaluateCondition", "evaluateStatements", "candidateStatement",
	"supportedConditionOperators", "conditionOutcome",
}

func TestIAMEvaluationLogicConfinedToOwnFile(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	internal := map[string]bool{}
	for _, s := range iamInternalIdentifiers {
		internal[s] = true
	}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".go" {
			continue
		}
		// The file that DEFINES this logic, and its own test, are exempt — every
		// other .go file in package core (and core_test) must not reference any of
		// these identifiers at all.
		if name == "iam_evaluate.go" || name == "iam_evaluate_test.go" {
			continue
		}

		path := filepath.Join(root, name)
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			if internal[ident.Name] {
				t.Errorf("%s references %q — IAM evaluation logic must live only in core/iam_evaluate.go; every consumer must call EvaluateIAMRequest/EvaluateAssumeRole instead", name, ident.Name)
			}
			return true
		})
	}
}
