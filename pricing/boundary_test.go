package pricing_test

// ADR-006's own stated consequence: "pricing/ (types + SQLite store) has zero
// network-capable imports." Proven structurally, not asserted in a comment — the same
// AST-parsing technique providers/aws/mapping_test.go's own
// TestAddingA9thResourceTypeRequiresZeroAnalyseChanges already uses.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"
)

func TestPricingPackageImportsNoNetworkCapability(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, root, func(fi os.FileInfo) bool {
		// Exclude this file's own package (pricing_test) and any _test.go file —
		// only the real, shipped pricing package's own imports matter here.
		return !isTestFile(fi.Name())
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parsing pricing/: %v", err)
	}

	forbidden := []string{"net/http", "net", "github.com/aws/aws-sdk-go"}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, imp := range file.Imports {
				path := importPathValue(imp)
				for _, f := range forbidden {
					if path == f {
						t.Errorf("pricing package imports %q — this package must have zero network capability (ADR-006's own stated boundary); the real fetcher belongs in cmd/runnerd/internal/pricingfetch", path)
					}
				}
			}
		}
	}
}

func isTestFile(name string) bool {
	return len(name) > 8 && name[len(name)-8:] == "_test.go"
}

func importPathValue(imp *ast.ImportSpec) string {
	v := imp.Path.Value
	if len(v) >= 2 {
		return v[1 : len(v)-1]
	}
	return v
}
