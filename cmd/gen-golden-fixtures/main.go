// Command gen-golden-fixtures derives golden/fixtures/*.ir.json and *.findings.json
// from a real run of the ingest parser and PC-14/17/18's analysis engines (via
// core.BuildFindings — shared with server.Assess, PC-21, so the generator and the live
// server never run two independently-drifting copies of the same finding-building
// logic) against golden/aws, golden/aws-broken, and (PC-22) golden/azure. PC-15's own
// Conversation is explicit: "don't hand-write expected IR/findings, derive them from a
// first correct run and then hand-verify." This command is that derivation step for
// every bundle — the fixtures it writes are regenerated output, not hand-edited,
// exactly like contracts/*.schema.json.
//
// azure.findings.json is generated and committed same as the AWS ones, but it is NOT
// evidence that Azure has real, comparable findings yet — core.BuildFindings is
// hardcoded to AWS node IDs and (for RDS encryption) an AWS-specific raw attribute key
// name, so every entry in azure.findings.json reads not_assessable. That is the
// honest, current output, not a bug in this generator — see golden/azure/README.md
// and golden/fixtures/README.md for the full account of this stated gap.
//
// Run with: go run ./cmd/gen-golden-fixtures
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"preflight/core"
	"preflight/ingest"
	"preflight/providers"
	awsprovider "preflight/providers/aws"
	azureprovider "preflight/providers/azure"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	outDir := filepath.Join(root, "golden", "fixtures")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fail(err)
	}

	awsRegistry, err := awsprovider.Load()
	if err != nil {
		fail(fmt.Errorf("load AWS provider mappings: %w", err))
	}
	azureRegistry, err := azureprovider.Load()
	if err != nil {
		fail(fmt.Errorf("load Azure provider mappings: %w", err))
	}
	workload, err := ingest.LoadWorkload(filepath.Join(root, "golden", "workload.yaml"))
	if err != nil {
		fail(fmt.Errorf("load workload: %w", err))
	}

	bundles := []struct {
		dir      string
		irName   string
		findName string
		registry providers.Registry
	}{
		{"golden/aws", "aws.ir.json", "aws.findings.json", awsRegistry},
		{"golden/aws-broken", "aws-broken.ir.json", "aws-broken.findings.json", awsRegistry},
		{"golden/azure", "azure.ir.json", "azure.findings.json", azureRegistry},
		{"golden/azure-broken", "azure-broken.ir.json", "azure-broken.findings.json", azureRegistry},
	}

	for _, b := range bundles {
		result, err := ingest.Ingest(filepath.Join(root, b.dir), b.registry, 1)
		if err != nil {
			fail(fmt.Errorf("ingest %s: %w", b.dir, err))
		}
		if result.Insufficient != nil {
			fail(fmt.Errorf("%s unexpectedly failed the MVG check: %+v", b.dir, result.Insufficient))
		}

		writeJSON(outDir, b.irName, result.IR)
		fmt.Printf("wrote %s (%d nodes, %d edges, %d out-of-vocabulary, %d edge-only)\n",
			filepath.Join("golden", "fixtures", b.irName),
			len(result.IR.Nodes), len(result.IR.Edges), len(result.OutOfVocabulary), len(result.EdgeOnlyResources))

		findings := core.BuildFindings(result.IR, workload)
		findings = append(findings, core.BuildUnsupportedRouteFindings(toUnsupportedRouteInfos(result.UnsupportedRoutes))...)
		writeJSON(outDir, b.findName, findings)
		fmt.Printf("wrote %s (%d findings)\n", filepath.Join("golden", "fixtures", b.findName), len(findings))

		if b.dir == "golden/aws" {
			// PC-120: one golden report fixture, byte-compared in CI
			// (report_golden_test.go) — golden/aws only, deliberately: this is a
			// projection over the same IR/findings already generated above, not a
			// second independent thing to keep in sync across four bundles. Cost is
			// nil (no pricing snapshot exists in this generator, same as every other
			// output here) — a real, honest "cost section unavailable" case, not a
			// gap invented for test coverage.
			scorecard := core.BuildScorecard(findings, 1)
			report := core.BuildReport("golden", 1, result.IR, workload, findings, scorecard, nil, core.PriceTable{}, nil)
			writeJSON(outDir, "aws.report.json", report)
			fmt.Printf("wrote %s\n", filepath.Join("golden", "fixtures", "aws.report.json"))
		}
	}
}

// toUnsupportedRouteInfos converts ingest's own UnsupportedRoute into core's
// UnsupportedRouteInfo — core cannot import ingest (I1), so this small conversion
// lives at this real caller boundary, mirroring server.assessFromResult's own copy.
func toUnsupportedRouteInfos(routes []ingest.UnsupportedRoute) []core.UnsupportedRouteInfo {
	infos := make([]core.UnsupportedRouteInfo, len(routes))
	for i, r := range routes {
		infos[i] = core.UnsupportedRouteInfo{RouteTableID: r.RouteTableKey, TargetKind: r.TargetKind, Source: r.Source}
	}
	return infos
}

func writeJSON(outDir, name string, v any) {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fail(fmt.Errorf("marshal %s: %w", name, err))
	}
	buf = append(buf, '\n')
	if err := os.WriteFile(filepath.Join(outDir, name), buf, 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gen-golden-fixtures:", err)
	os.Exit(1)
}
