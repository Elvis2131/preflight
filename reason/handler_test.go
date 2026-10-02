package reason

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type sseEvent struct{ Event, Data string }

func readSSE(t *testing.T, body string) []sseEvent {
	t.Helper()
	var out []sseEvent
	var cur sseEvent
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			cur.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			cur.Data = strings.TrimPrefix(line, "data: ")
		case line == "" && cur.Event != "":
			out = append(out, cur)
			cur = sseEvent{}
		}
	}
	return out
}

func post(t *testing.T, h http.Handler, fs []any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"findings": fs})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/annotate", bytes.NewReader(b)))
	return rec
}

func TestHandler_NoKey_Is503Degraded_NotACrash(t *testing.T) {
	rec := post(t, NewAnnotateHandler(nil), nil)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"degraded":"no_api_key"`) {
		t.Fatalf("got %d %s, want 503 with degraded=no_api_key", rec.Code, rec.Body.String())
	}
}

func TestHandler_StreamsStartedAnnotationsDone(t *testing.T) {
	fs := goldenFindings(t)[:3]
	model := fakeModel(t, goodReply)
	items := make([]any, len(fs))
	for i := range fs {
		items[i] = fs[i]
	}
	rec := post(t, NewAnnotateHandler(model), items)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	evs := readSSE(t, rec.Body.String())
	if len(evs) != 5 || evs[0].Event != "started" || evs[4].Event != "done" {
		t.Fatalf("want started, 3 annotations, done; got %+v", evs)
	}
	for _, e := range evs[1:4] {
		if e.Event != "annotation" || !strings.Contains(e.Data, `"llm_reasoned"`) {
			t.Errorf("middle events must be llm_reasoned annotations: %+v", e)
		}
	}
	if !strings.Contains(evs[4].Data, `"annotated":3`) || !strings.Contains(evs[4].Data, `"call_failures":0`) {
		t.Errorf("done = %s", evs[4].Data)
	}
}

func TestHandler_UpstreamFailure_StreamsRejectedAndStillFinishes(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("slow down"))
	})
	fs := goldenFindings(t)[:2]
	items := []any{fs[0], fs[1]}
	evs := readSSE(t, post(t, NewAnnotateHandler(c), items).Body.String())
	if len(evs) != 4 || evs[1].Event != "rejected" || evs[2].Event != "rejected" || evs[3].Event != "done" {
		t.Fatalf("a 429 per finding must stream as rejected and the run must still finish: %+v", evs)
	}
	if !strings.Contains(evs[1].Data, `"call_failed"`) || !strings.Contains(evs[3].Data, `"call_failures":2`) {
		t.Errorf("rejections must be tagged call_failed and counted: %s / %s", evs[1].Data, evs[3].Data)
	}
	if strings.Contains(evs[1].Data+evs[3].Data, testKey) {
		t.Error("the API key must never appear in a stream")
	}
}

func TestHandler_RejectsGETAndBadBody(t *testing.T) {
	h := NewAnnotateHandler(fakeModel(t, goodReply))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/annotate", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d, want 405", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/annotate", strings.NewReader("{not json")))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad body = %d, want 400", rec.Code)
	}
}
