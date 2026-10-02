package server_test

// PC-154: LLM annotations for a stored version, streamed after the deterministic result,
// stored beside it, and never able to delay, change or fail it. A STUB reason worker stands in
// for P2 so every failure mode is exercised deterministically (no key, no network).

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"preflight/core"
	"preflight/server"
)

type sseEv struct{ Event, Data string }

func parseSSE(body string) []sseEv {
	var out []sseEv
	var cur sseEv
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		l := sc.Text()
		switch {
		case strings.HasPrefix(l, "event: "):
			cur.Event = strings.TrimPrefix(l, "event: ")
		case strings.HasPrefix(l, "data: "):
			cur.Data = strings.TrimPrefix(l, "data: ")
		case l == "" && cur.Event != "":
			out = append(out, cur)
			cur = sseEv{}
		}
	}
	return out
}

func eventNames(evs []sseEv) []string {
	var n []string
	for _, e := range evs {
		n = append(n, e.Event)
	}
	return n
}

func assessGolden(t *testing.T, store *server.Store, session string) server.AssessResponse {
	t.Helper()
	bundle, _ := filepath.Abs("../golden/aws")
	workload, _ := filepath.Abs("../golden/workload.yaml")
	resp, err := server.Assess(store, server.AssessRequest{SessionID: session, BundleDir: bundle, WorkloadPath: workload})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	return resp
}

// stubWorker answers POST /v1/annotate like reasond. mode picks the behaviour; calls counts hits.
func stubWorker(t *testing.T, mode string, calls *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		if mode == "nokey" {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"degraded":"no_api_key"}`))
			return
		}
		var req struct{ Findings []core.Finding }
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		send := func(ev string, v any) {
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev, b)
			fl.Flush()
		}
		send("started", map[string]any{"model": "stub-model", "findings": len(req.Findings)})
		for i, f := range req.Findings {
			failed := mode == "rate_limited" || (mode == "partial" && i%2 == 1)
			if failed {
				send("rejected", core.LLMRejection{FindingID: f.ID, Reason: "model call failed: HTTP 429", Kind: core.LLMRejectionCallFailed})
				continue
			}
			send("annotation", core.LLMAnnotation{
				FindingID: f.ID, Narrative: "About " + f.ID, CitedEvidence: f.Dimensions.AffectedComponents[:1],
				Provenance: core.NewProvenance(core.KindLLMReasoned, "stub").WithReason("cites " + f.Dimensions.AffectedComponents[0]),
			})
		}
		if mode == "truncated" {
			return // connection ends with no `done`
		}
		send("done", map[string]any{})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func openWithWorker(t *testing.T, url string) *server.Store {
	t.Helper()
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if url != "" {
		store.AttachReasonWorker(url)
	}
	return store
}

func streamAnnotations(t *testing.T, store *server.Store, session string) (int, []sseEv, time.Duration) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sessions/{id}/versions/{n}/annotations", server.GetAnnotationsHandler(store))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	start := time.Now()
	resp, err := http.Get(srv.URL + "/sessions/" + session + "/versions/1/annotations")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	first := time.Duration(0)
	var sb strings.Builder
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if first == 0 && strings.HasPrefix(sc.Text(), "event: annotation") {
			first = time.Since(start)
		}
		sb.WriteString(sc.Text() + "\n")
	}
	return resp.StatusCode, parseSSE(sb.String()), first
}

func TestAnnotations_StreamedStoredAndReplayedWithoutCallingTheWorkerAgain(t *testing.T) {
	var calls int32
	store := openWithWorker(t, stubWorker(t, "ok", &calls).URL)
	resp := assessGolden(t, store, "ann")
	before, _ := json.Marshal(resp.Findings)

	code, evs, first := streamAnnotations(t, store, "ann")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	names := eventNames(evs)
	if names[len(names)-1] != "done" || strings.Count(strings.Join(names, ","), "annotation") != len(resp.Findings) {
		t.Fatalf("want one annotation per finding then done, got %v", names)
	}
	if !strings.Contains(evs[len(evs)-1].Data, `"status":"complete"`) {
		t.Errorf("done = %s, want complete", evs[len(evs)-1].Data)
	}
	// NFR-9 plumbing: with an instant worker the first annotation reaches the caller well inside
	// 3 s. (The real model's first annotation is bound by its own latency — not asserted offline.)
	if first == 0 || first > 3*time.Second {
		t.Errorf("first annotation after %v, want < 3s with an instant worker", first)
	}

	// Stored: a second stream replays from storage and never calls the worker again.
	_, evs2, _ := streamAnnotations(t, store, "ann")
	if atomic.LoadInt32(&calls) != 1 || strings.Join(eventNames(evs2), ",") != strings.Join(names, ",") {
		t.Errorf("worker calls = %d (want 1); replay events differ from the first run", atomic.LoadInt32(&calls))
	}
	// Read-back as JSON, labelled and separate from findings.
	set, ok, err := store.GetAnnotations("ann", 1)
	if err != nil || !ok || set.Status != core.LLMStatusComplete || len(set.Annotations) != len(resp.Findings) || set.Notice == "" {
		t.Fatalf("stored set = %+v ok=%v err=%v", set, ok, err)
	}
	for _, a := range set.Annotations {
		if a.Provenance.Kind != core.KindLLMReasoned {
			t.Errorf("%s: provenance %q, want llm_reasoned", a.FindingID, a.Provenance.Kind)
		}
	}
	// I3: the findings are exactly what they were.
	stored, _ := server.GetStoredVersion(store, "ann", 1)
	after, _ := json.Marshal(stored.Findings)
	if string(before) != string(after) {
		t.Error("annotating changed the stored findings (I3)")
	}
}

// Each way the narrative layer can be unavailable is a `degraded` event on a 200 stream —
// never an HTTP error, never a change to a finding (NFR-11).
func TestAnnotations_Degraded_Cases(t *testing.T) {
	cases := []struct {
		name       string
		workerURL  func(*testing.T) string
		wantReason string
		wantStored bool
	}{
		{"worker not configured", func(*testing.T) string { return "" }, "no reason worker is configured", false},
		{"worker unreachable (down)", func(t *testing.T) string {
			s := httptest.NewServer(http.NotFoundHandler())
			url := s.URL
			s.Close()
			return url
		}, "unreachable", false},
		{"key absent (503 no_api_key)", func(t *testing.T) string { var c int32; return stubWorker(t, "nokey", &c).URL }, "no API key", false},
		{"provider rate-limited (429 on every call)", func(t *testing.T) string { var c int32; return stubWorker(t, "rate_limited", &c).URL }, "could not be annotated", false},
		{"stream cut before done", func(t *testing.T) string { var c int32; return stubWorker(t, "truncated", &c).URL }, "ended before it finished", true},
		{"partial (half the calls fail)", func(t *testing.T) string { var c int32; return stubWorker(t, "partial", &c).URL }, "could not be annotated", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := openWithWorker(t, tc.workerURL(t))
			resp := assessGolden(t, store, "deg")
			before, _ := json.Marshal(resp.Findings)

			code, evs, _ := streamAnnotations(t, store, "deg")
			if code != 200 {
				t.Fatalf("a degraded narrative layer must not be an HTTP error: got %d", code)
			}
			names := eventNames(evs)
			if names[len(names)-1] != "done" {
				t.Fatalf("the stream must always end with done: %v", names)
			}
			var reason string
			for _, e := range evs {
				if e.Event == "degraded" {
					reason = e.Data
				}
			}
			if reason == "" || !strings.Contains(reason, tc.wantReason) {
				t.Errorf("degraded reason = %q, want it to mention %q", reason, tc.wantReason)
			}
			if !strings.Contains(evs[len(evs)-1].Data, `"status":"degraded"`) {
				t.Errorf("done = %s, want status degraded", evs[len(evs)-1].Data)
			}
			if _, stored, _ := store.GetAnnotations("deg", 1); stored != tc.wantStored {
				t.Errorf("stored = %v, want %v (an infrastructure failure with nothing produced must not be cached)", stored, tc.wantStored)
			}
			stored, _ := server.GetStoredVersion(store, "deg", 1)
			after, _ := json.Marshal(stored.Findings)
			if string(before) != string(after) {
				t.Error("a degraded narrative layer changed the findings")
			}
		})
	}
}

// NFR-5: the deterministic payload is byte-identical whether the reason worker is running,
// stopped, or never configured. (compute_duration_ms is measured, so it is zeroed.)
func TestAssess_DeterministicPayload_IdenticalWithWorkerRunningStoppedOrAbsent(t *testing.T) {
	var calls int32
	running := stubWorker(t, "ok", &calls)
	stopped := httptest.NewServer(http.NotFoundHandler())
	stoppedURL := stopped.URL
	stopped.Close()

	payload := func(url string) string {
		resp := assessGolden(t, openWithWorker(t, url), "nfr5")
		resp.ComputeDurationMS = 0
		b, _ := json.Marshal(resp)
		return string(b)
	}
	a, b, c := payload(running.URL), payload(stoppedURL), payload("")
	if a != b || b != c {
		t.Fatal("the /assess payload differs depending on the reason worker's state (NFR-5)")
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Errorf("/assess called the reason worker %d times; it must never", calls)
	}
}

// NFR-6: /assess answers in well under 5 s on the golden fixture whatever the worker is doing.
func TestAssess_UnderFiveSeconds_WithAWorkerThatNeverAnswers(t *testing.T) {
	hang := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-hang }))
	defer func() { close(hang); slow.Close() }()
	store := openWithWorker(t, slow.URL)
	start := time.Now()
	assessGolden(t, store, "nfr6")
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("/assess took %v with a hung worker, want < 5s (NFR-6)", d)
	}
}

func TestAnnotationsHandler_404sAndJSONReadBack(t *testing.T) {
	var calls int32
	store := openWithWorker(t, stubWorker(t, "ok", &calls).URL)
	assessGolden(t, store, "rb")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sessions/{id}/versions/{n}/annotations", server.GetAnnotationsHandler(store))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	get := func(path string) int {
		r, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		return r.StatusCode
	}
	if get("/sessions/nope/versions/1/annotations") != 404 || get("/sessions/rb/versions/9/annotations") != 404 || get("/sessions/rb/versions/x/annotations") != 400 {
		t.Error("unknown session/version must 404 and a bad version must 400")
	}
	if get("/sessions/rb/versions/1/annotations?format=json") != 404 {
		t.Error("before anything is stored, ?format=json must be 404 annotations_not_stored")
	}
	streamAnnotations(t, store, "rb") // generates and stores
	if get("/sessions/rb/versions/1/annotations?format=json") != 200 {
		t.Error("after a run, ?format=json must return the stored set")
	}
}
