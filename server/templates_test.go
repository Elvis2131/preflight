package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"preflight/golden/templates"
	"preflight/server"
)

func TestTemplatesEndpoints_ListAndLoadAndUnknown(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /templates", server.ListTemplatesHandler())
	mux.HandleFunc("GET /templates/{id}", server.GetTemplateHandler())

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/templates", nil))
	var list []templates.Meta
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != 200 || len(list) < 3 {
		t.Fatalf("list: code %d err %v body %s, want 200 and at least 3 templates", rec.Code, err, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/templates/"+list[0].ID, nil))
	var got templates.Template
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 || len(got.Canvas.Nodes) == 0 || got.Meta.ID != list[0].ID {
		t.Fatalf("get: code %d err %v, want 200 and a populated template", rec.Code, err)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/templates/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown template: code %d, want 404", rec.Code)
	}
}
