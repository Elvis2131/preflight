package server_test

// A regression test for the two additions in server/openapi.go: /openapi.json must
// stay valid JSON with the real AssessRequest/AssessResponse fields present (not silently
// drift from the actual wire types), and /swagger must keep serving real HTML, not a
// panic or an empty body.

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"preflight/server"
)

func TestOpenAPISpecHandler_ServesValidSpecMatchingRealFields(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	server.OpenAPISpecHandler()(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var spec map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
	if spec["openapi"] != "3.1.0" {
		t.Errorf("openapi version = %v, want 3.1.0", spec["openapi"])
	}

	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("spec has no paths object")
	}
	for _, p := range []string{"/healthz", "/assess"} {
		if _, ok := paths[p]; !ok {
			t.Errorf("paths missing %q", p)
		}
	}

	assessPost := paths["/assess"].(map[string]any)["post"].(map[string]any)
	reqSchema := assessPost["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	reqProps := reqSchema["properties"].(map[string]any)
	for _, field := range []string{"session_id", "bundle_dir", "workload_path"} {
		if _, ok := reqProps[field]; !ok {
			t.Errorf("AssessRequest schema missing field %q — spec has drifted from the real struct", field)
		}
	}

	respSchema := assessPost["responses"].(map[string]any)["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	respProps := respSchema["properties"].(map[string]any)
	for _, field := range []string{"session_id", "version_number", "findings", "scorecard", "assurance_delta", "graph", "degraded", "degraded_reason", "compute_duration_ms"} {
		if _, ok := respProps[field]; !ok {
			t.Errorf("AssessResponse schema missing field %q — spec has drifted from the real struct", field)
		}
	}
}

func TestSwaggerUIHandler_ServesHTMLPointingAtTheSpec(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/swagger", nil)
	server.SwaggerUIHandler()(rec, req)

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html prefix", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/openapi.json") {
		t.Error("swagger page does not reference /openapi.json")
	}
	if !strings.Contains(body, "swagger-ui-bundle") {
		t.Error("swagger page does not load the swagger-ui bundle")
	}
}
