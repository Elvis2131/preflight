// Command gen-template-fixtures derives golden/templates/<id>/{ir,findings,failure}.json
// from a real run of each template's canvas document through the production pipeline
// (PC-108). PC-15's rule applies unchanged: expected output is derived from a first
// correct run and then hand-verified, never hand-written — these files are regenerated
// output, not hand-edited, exactly like contracts/*.schema.json and golden/fixtures.
//
// Run with: go run ./cmd/gen-template-fixtures
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"preflight/golden/templates"
	awsprovider "preflight/providers/aws"
)

func main() {
	registry, err := awsprovider.Load()
	if err != nil {
		fail(fmt.Errorf("load AWS provider mappings: %w", err))
	}
	ids, err := templates.IDs()
	if err != nil {
		fail(err)
	}
	for _, id := range ids {
		d, err := templates.Derive(id, registry)
		if err != nil {
			fail(err)
		}
		for name, b := range map[string][]byte{"ir": d.IR, "findings": d.Findings, "failure": d.Failure} {
			path := filepath.Join("golden", "templates", id, name+".json")
			if err := os.WriteFile(path, b, 0o644); err != nil {
				fail(err)
			}
		}
		fmt.Printf("wrote golden/templates/%s/{ir,findings,failure}.json\n", id)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gen-template-fixtures:", err)
	os.Exit(1)
}
