package core_test

// PC-115's own acceptance criterion, verbatim: "Test proves capacity/bottleneck logic
// doesn't read instance count from sizing fields." Structural, not behavioral: parses
// core/simulate.go and core/workload.go (the only files that touch Workload.Capacity,
// the real, only source of truth for capacity — PRD §4's own capacity semantics) and
// confirms neither ever references the Sizing identifier at all — not "happens not to
// read it today" but "cannot, because it never sees the type."

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

func TestCapacityLogicNeverReferencesSizing(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"simulate.go", "workload.go"} {
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
				t.Errorf("%s references the Sizing type/field — capacity logic must derive only from Workload.Capacity, never from sizing facts", file)
			}
			return true
		})
	}
}
