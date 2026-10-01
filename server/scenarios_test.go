package server_test

// PC-131: saved Failure Lab scenarios are DEFINITIONS; every result is recomputed
// against the version asked about, so a report reflects the current design.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"preflight/core"
	"preflight/server"
)

func scenarioDoc(withDB1 bool) core.CanvasDocument {
	nodes := []core.CanvasNode{
		{ID: "dns1", Type: "dns", Label: "DNS", Capability: map[string]string{}},
		{ID: "db2", Type: "managed_database", Label: "DB2", Capability: map[string]string{}},
	}
	edges := []core.CanvasEdge{{ID: "e2", Type: "routes_to", From: "dns1", To: "db2"}}
	if withDB1 {
		nodes = append(nodes, core.CanvasNode{ID: "db1", Type: "managed_database", Label: "DB1", Capability: map[string]string{}})
		edges = append(edges, core.CanvasEdge{ID: "e1", Type: "routes_to", From: "dns1", To: "db1"})
	}
	return core.CanvasDocument{Nodes: nodes, Edges: edges}
}

func TestScenarios_SavedDefinitionsAreReEvaluatedAgainstEachVersion_NotReplayed(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	w := baseInlineWorkload(nil)

	// v1 has db1; v2 removes it.
	if _, err := server.AssessCanvas(store, server.AssessCanvasRequest{SessionID: "sc", Canvas: scenarioDoc(true), Workload: &w}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveScenario("sc", core.SavedScenario{Name: "lose db1", Faults: []core.Fault{{Type: "node_loss", Target: "db1"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.AssessCanvas(store, server.AssessCanvasRequest{SessionID: "sc", Canvas: scenarioDoc(false), Workload: &w}); err != nil {
		t.Fatal(err)
	}

	r1, err := server.GetReport(store, "sc", 1)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := server.GetReport(store, "sc", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.Scenarios) != 1 || len(r2.Scenarios) != 1 {
		t.Fatalf("both reports must list the saved scenario, got %d and %d", len(r1.Scenarios), len(r2.Scenarios))
	}
	if r1.Scenarios[0].Verdict.State != core.AssessmentStateAssessed {
		t.Fatalf("v1: db1 exists, the scenario must evaluate, got %+v", r1.Scenarios[0].Verdict)
	}
	// In v2 the target no longer exists: the SAME saved definition now reads not_assessable
	// and says why. A replay of v1's stored result could never produce this.
	v := r2.Scenarios[0].Verdict
	if v.State != core.AssessmentStateNotAssessable || !strings.Contains(v.Reason, "db1") {
		t.Fatalf("v2: want not_assessable naming the missing target db1, got %+v", v)
	}
	if r1.Scenarios[0].Name != "lose db1" || r2.Scenarios[0].Faults[0].Target != "db1" {
		t.Fatalf("the definition itself must be unchanged across versions: %+v / %+v", r1.Scenarios[0], r2.Scenarios[0])
	}
}

func TestScenarios_ReportHasEmptyScenariosArrayWhenNoneSaved(t *testing.T) {
	store, _ := server.OpenStore(":memory:")
	defer store.Close()
	w := baseInlineWorkload(nil)
	if _, err := server.AssessCanvas(store, server.AssessCanvasRequest{SessionID: "none", Canvas: scenarioDoc(true), Workload: &w}); err != nil {
		t.Fatal(err)
	}
	r, err := server.GetReport(store, "none", 1)
	if err != nil {
		t.Fatal(err)
	}
	if r.Scenarios == nil || len(r.Scenarios) != 0 {
		t.Fatalf("want a non-nil empty slice (no null in JSON), got %#v", r.Scenarios)
	}
}

func TestScenarios_HTTP_SaveListDeleteAndEvaluate(t *testing.T) {
	store, _ := server.OpenStore(":memory:")
	defer store.Close()
	w := baseInlineWorkload(nil)
	if _, err := server.AssessCanvas(store, server.AssessCanvasRequest{SessionID: "h", Canvas: scenarioDoc(true), Workload: &w}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sessions/{id}/scenarios", server.ListScenariosHandler(store))
	mux.HandleFunc("PUT /sessions/{id}/scenarios/{name}", server.SaveScenarioHandler(store))
	mux.HandleFunc("DELETE /sessions/{id}/scenarios/{name}", server.DeleteScenarioHandler(store))
	mux.HandleFunc("GET /sessions/{id}/versions/{n}/scenarios", server.EvaluateScenariosHandler(store))
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}

	if rec := do("PUT", "/sessions/h/scenarios/lose%20db1%20and%20db2", `{"faults":[{"type":"node_loss","target":"db1"},{"type":"node_loss","target":"db2"}]}`); rec.Code != 200 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do("GET", "/sessions/h/scenarios", ""); !strings.Contains(rec.Body.String(), `"name":"lose db1 and db2"`) {
		t.Fatalf("list must show the saved definition (name URL-decoded): %s", rec.Body.String())
	}
	rec := do("GET", "/sessions/h/versions/1/scenarios", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"verdict"`) {
		t.Fatalf("evaluate: %d %s", rec.Code, rec.Body.String())
	}
	// Invalid input is rejected with a structured error, not stored.
	for body, want := range map[string]string{`{"faults":[]}`: "at least one fault", `{"faults":[{"type":"node_loss"}]}`: "needs a type and a target"} {
		if rec := do("PUT", "/sessions/h/scenarios/bad", body); rec.Code != 400 || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("body %s: got %d %s, want 400 containing %q", body, rec.Code, rec.Body.String(), want)
		}
	}
	if rec := do("DELETE", "/sessions/h/scenarios/lose%20db1%20and%20db2", ""); rec.Code != 204 {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := do("DELETE", "/sessions/h/scenarios/lose%20db1%20and%20db2", ""); rec.Code != 404 {
		t.Fatalf("second delete should be 404, got %d", rec.Code)
	}
}

// The canvas runs on another origin, so the browser preflights PUT/DELETE; the server
// must allow them or saving/deleting a scenario fails silently in the workspace (found
// by driving the real UI — the Go-level handler tests could never see it).
func TestCORS_AllowsPutAndDeleteForScenarios(t *testing.T) {
	h := server.WithCORS(http.NewServeMux())
	req := httptest.NewRequest("OPTIONS", "/sessions/x/scenarios/y", nil)
	req.Header.Set("Origin", "http://localhost:5183")
	req.Header.Set("Access-Control-Request-Method", "PUT")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	allowed := rec.Header().Get("Access-Control-Allow-Methods")
	for _, m := range []string{"PUT", "DELETE"} {
		if !strings.Contains(allowed, m) {
			t.Errorf("Access-Control-Allow-Methods = %q, must include %s", allowed, m)
		}
	}
}
