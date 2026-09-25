// Package conformancescan is the shared AST scan both cmd/gen-conformance-report and
// cmd/check-conformance-links need: "what are all the conformance specs" is one
// question, answered once here, not reimplemented per command.
package conformancescan

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

// Entry is one discovered conformance test's reportable metadata.
type Entry struct {
	Area   string
	ID     string
	Rule   string
	Source string
	File   string
}

// Scan walks confDir, parsing every _test.go file and extracting harness.Spec{...}
// composite literals via go/ast. "Area" is the immediate subdirectory name
// (networking, routing, cidr, placement, failover, pricing) — the harness/ directory
// itself is skipped, since it holds the format's own self-tests (spec_test.go), not
// conformance tests.
func Scan(confDir string) ([]Entry, error) {
	var out []Entry
	fset := token.NewFileSet()

	err := filepath.WalkDir(confDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		area := filepath.Base(filepath.Dir(path))
		if area == "harness" {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		ast.Inspect(f, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := cl.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Spec" {
				return true
			}
			if ident, ok := sel.X.(*ast.Ident); !ok || ident.Name != "harness" {
				return true
			}
			e := Entry{Area: area, File: filepath.Base(path)}
			for _, elt := range cl.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				lit, ok := kv.Value.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				switch key.Name {
				case "ID":
					e.ID = value
				case "Rule":
					e.Rule = value
				case "Source":
					e.Source = value
				}
			}
			if e.ID != "" {
				out = append(out, e)
			}
			return true
		})
		return nil
	})
	return out, err
}
