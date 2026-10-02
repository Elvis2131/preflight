package reason

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

// PC-160: annotation runs are serialised per process (free-tier rate limit), so a second request
// does not even get its `started` event until the first run is finished. This pins that behaviour
// as the measured cause of PC-154's 180-200 s first-annotation times when a previous run was still
// going — P1 deliberately lets a run finish after its client disconnects so the set can be stored.
func TestHandler_SecondRunWaitsForTheFirst(t *testing.T) {
	release := make(chan struct{})
	inModel := make(chan struct{}, 4)
	model := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		inModel <- struct{}{}
		<-release
		var req wireRequest
		json.NewDecoder(r.Body).Decode(&req)
		user := req.Messages[len(req.Messages)-1].Content
		var pf promptFinding
		json.Unmarshal([]byte(strings.TrimPrefix(user, "FINDING:\n")), &pf)
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": goodReply(pf.ID, pf.AffectedComponents[0])}, "finish_reason": "stop"}},
			"usage":   map[string]any{"total_tokens": 15},
		})
	})
	srv := httptest.NewServer(NewAnnotateHandler(model))
	defer srv.Close()
	fs := goldenFindings(t)[:1]
	body, _ := json.Marshal(map[string]any{"findings": fs})

	// Each client keeps its connection open until the test ends, like P1 does: a client that hangs
	// up cancels its run (and frees the worker), which is not the case being pinned here.
	hangUp := make(chan struct{})
	defer close(hangUp)
	firstEvent := func() chan string {
		out := make(chan string, 1)
		go func() {
			resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
			if err != nil {
				out <- "error: " + err.Error()
				return
			}
			defer resp.Body.Close()
			sc := bufio.NewScanner(resp.Body)
			sent := false
			for sc.Scan() {
				if !sent && strings.HasPrefix(sc.Text(), "event: ") {
					out <- strings.TrimPrefix(sc.Text(), "event: ")
					sent = true
				}
			}
			if !sent {
				out <- "closed"
			}
			<-hangUp
		}()
		return out
	}

	a := firstEvent()
	if ev := <-a; ev != "started" {
		t.Fatalf("first run's first event = %q, want started", ev)
	}
	<-inModel // run A is inside the (blocked) model call and holds the worker
	b := firstEvent()
	select {
	case ev := <-b:
		t.Fatalf("second run produced %q while the first still held the worker; runs are meant to be serialised", ev)
	case <-time.After(300 * time.Millisecond):
	}
	close(release) // let A finish; B must then proceed
	select {
	case ev := <-b:
		if ev != "started" {
			t.Fatalf("second run's first event = %q, want started", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second run never started after the first finished")
	}
}
