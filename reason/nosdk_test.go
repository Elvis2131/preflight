package reason

// PC-77 acceptance: "reason/'s HTTP client code has zero SDK dependencies (Anthropic's
// or OpenAI-compatible) — direct net/http only." Enforced by parsing the real imports,
// so adding an SDK fails the build's tests rather than a review.

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReason_ImportsOnlyStdlibAndCore(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, dir := range []string{"../cmd/reasond", "../cmd/reason-eval"} {
		more, _ := filepath.Glob(dir + "/*.go")
		files = append(files, more...)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, f := range files {
		af, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range af.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			first := strings.SplitN(path, "/", 2)[0]
			stdlib := !strings.Contains(first, ".") && first != "preflight"
			if stdlib || path == "preflight/core" || (strings.HasPrefix(f, "../cmd/") && path == "preflight/reason") {
				checked++
				continue
			}
			t.Errorf("%s imports %q — reason/ may use only the standard library and preflight/core (ADR-005: no SDK)", f, path)
		}
	}
	if checked == 0 {
		t.Fatal("parsed no imports; the test is not looking at anything")
	}
}

func TestGoMod_HasNoLLMSDK(t *testing.T) {
	b, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"anthropic", "openai", "sashabaranov", "langchain", "nvidia", "genai", "generative-ai"} {
		if strings.Contains(strings.ToLower(string(b)), banned) {
			t.Errorf("go.mod mentions %q — ADR-005 forbids any LLM SDK, Anthropic's or OpenAI-compatible", banned)
		}
	}
}
