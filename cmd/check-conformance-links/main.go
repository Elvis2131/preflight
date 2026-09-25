// Command check-conformance-links is PC-119's doc-link checker: "flags citation URLs
// that stop resolving, since AWS docs move." Deliberately a periodic job, not a
// per-commit CI gate — the Card is explicit about this, and this codebase already
// separates "runs on every commit" (go test ./...) from "needs real external state and
// runs on its own schedule" (validate/'s Rung 3, credentialed and cloud-scoped). A
// citation URL is a real external dependency this project doesn't control; failing
// every commit's CI because AWS reorganised a doc page would be exactly the kind of
// flaky, unrelated-to-the-actual-change build failure this project's own CI discipline
// avoids elsewhere.
//
// Reuses cmd/gen-conformance-report's own AST scan (same package, exported) rather
// than re-parsing — one source of "what are all the conformance specs," not two.
//
// Run with: go run ./cmd/check-conformance-links
// Intended to be wired into a scheduled (not push/PR-triggered) GitHub Actions
// workflow — see .github/workflows/conformance-links.yml.
package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"preflight/cmd/conformancescan"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	confDir := filepath.Join(root, "tests", "aws-conformance")

	entries, err := conformancescan.Scan(confDir)
	if err != nil {
		fail(err)
	}

	seen := map[string]bool{}
	var urls []string
	for _, e := range entries {
		if !seen[e.Source] {
			seen[e.Source] = true
			urls = append(urls, e.Source)
		}
	}

	client := &http.Client{Timeout: 15 * time.Second}
	var broken []string
	for _, url := range urls {
		if err := checkURL(client, url); err != nil {
			broken = append(broken, fmt.Sprintf("%s: %v", url, err))
		}
	}

	fmt.Printf("checked %d distinct citation URLs across %d conformance tests\n", len(urls), len(entries))
	if len(broken) > 0 {
		fmt.Println("BROKEN CITATIONS:")
		for _, b := range broken {
			fmt.Println(" -", b)
		}
		os.Exit(1)
	}
	fmt.Println("all citations resolved")
}

// checkURL sends a real HEAD request (falling back to GET, since some AWS doc pages
// reject HEAD) and treats any non-2xx/3xx status, or a request error, as broken. Not
// mocked: the entire point of this command is to observe the real, current state of
// AWS's own documentation, which by definition cannot be verified against a fixture.
func checkURL(client *http.Client, url string) error {
	resp, err := client.Head(url)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode < 400 {
			return nil
		}
	}
	resp2, err2 := client.Get(url)
	if err2 != nil {
		if err != nil {
			return err
		}
		return err2
	}
	defer resp2.Body.Close()
	if resp2.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp2.StatusCode)
	}
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "check-conformance-links:", err)
	os.Exit(1)
}
