package aws_test

// PC-109: the workspace's service → icon lookup (canvas/src/awsIcons.ts) is keyed by the
// service IDs. Icons can describe a design-only catalogue entry or a registry node;
// presentation never grants a backend capability. Edge-only mappings cannot be nodes.
// The icon assets and their licence position live in canvas/public/aws-icons/.

import (
	"os"
	"regexp"
	"testing"

	"preflight/providers/aws"
)

func TestIconLookupOnlyNamesCatalogServicesOrRegistryNodes(t *testing.T) {
	directory, err := os.ReadFile("../../canvas/src/awsServiceDirectory.ts")
	if err != nil {
		t.Fatalf("read awsServiceDirectory.ts: %v", err)
	}
	catalogIDs := make(map[string]bool)
	for _, row := range regexp.MustCompile(`"resource_type": "([^"]+)"`).FindAllSubmatch(directory, -1) {
		catalogIDs[string(row[1])] = true
	}
	if len(catalogIDs) < 250 {
		t.Fatalf("parsed only %d catalogue IDs", len(catalogIDs))
	}
	reg, err := aws.Load()
	if err != nil {
		t.Fatalf("aws.Load: %v", err)
	}

	var src []byte
	for _, name := range []string{"awsIcons.ts", "awsServiceIconFiles.ts"} {
		content, err := os.ReadFile("../../canvas/src/" + name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		src = append(src, content...)
	}
	// Both provider resource IDs and explicit UI-only aws_service: identities.
	keys := regexp.MustCompile(`(?m)^\s+"?(aws_service:[a-z0-9-]+|aws_[a-z0-9_]+)"?:`).FindAllSubmatch(src, -1)
	if len(keys) < 230 {
		t.Fatalf("parsed only %d service keys from awsIcons.ts — the parser, not the file, is probably wrong", len(keys))
	}
	for _, k := range keys {
		id := string(k[1])
		m, ok := reg.Lookup(id)
		if !ok {
			if !catalogIDs[id] {
				t.Errorf("icon lookup names %q, absent from both the AWS catalogue and registry", id)
			}
			continue
		}
		if m.IsEdgeMapping() {
			t.Errorf("icon lookup names %q, an edge-only mapping with no node to draw an icon on", id)
		}
	}
}
