package aws_test

// PC-109: the workspace's service → icon lookup (canvas/src/awsIcons.ts) is keyed by the
// capability registry's service IDs. Every key must be a REAL node-mapped service in this
// registry — otherwise an icon (or label) points at a service the engine does not model.
// The icon assets and their licence position live in canvas/public/aws-icons/.

import (
	"os"
	"regexp"
	"testing"

	"preflight/providers/aws"
)

func TestIconLookupOnlyNamesServicesTheRegistryModels(t *testing.T) {
	src, err := os.ReadFile("../../canvas/src/awsIcons.ts")
	if err != nil {
		t.Fatalf("read awsIcons.ts: %v", err)
	}
	reg, err := aws.Load()
	if err != nil {
		t.Fatalf("aws.Load: %v", err)
	}

	// Each "  aws_xxx: " entry inside the two lookup tables.
	keys := regexp.MustCompile(`(?m)^\s+(aws_[a-z0-9_]+):`).FindAllSubmatch(src, -1)
	if len(keys) < 15 {
		t.Fatalf("parsed only %d service keys from awsIcons.ts — the parser, not the file, is probably wrong", len(keys))
	}
	for _, k := range keys {
		id := string(k[1])
		m, ok := reg.Lookup(id)
		if !ok {
			t.Errorf("awsIcons.ts names %q, which is not a service in the AWS registry", id)
			continue
		}
		if m.IsEdgeMapping() {
			t.Errorf("awsIcons.ts names %q, an edge-only mapping with no node to draw an icon on", id)
		}
	}
}
