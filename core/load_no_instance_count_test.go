package core_test

// PC-126's own acceptance criterion, verbatim: "Test proves instance count is never
// used as capacity; missing capacity → not_assessable naming the field." The
// not_assessable half is covered directly in load_test.go
// (TestComputeComponentLoad_MissingCapacity_NotAssessable); this is the structural
// half — the same go/ast technique core/sizing_test.go already established for
// proving core/simulate.go never reads Sizing at all.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLogicNeverReferencesSizing(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"load.go", "load_findings.go"} {
		path := filepath.Join(root, file)
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			if ident.Name == "Sizing" {
				t.Errorf("%s references the Sizing type/field — capacity must come only from Workload.Capacity, never from an instance count", file)
			}
			return true
		})
	}
}
