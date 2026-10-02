// This file is PC-154: LLM narratives for a stored version, streamed to the caller AFTER the
// deterministic result and stored beside it. Process boundary (ADR-003): the LLM call runs in
// the reason worker (P2, reasond); this file is P1 and talks to it over plain HTTP. P1 imports
// neither reason/ nor reads any API key (boundary_test.go proves both) — the wire types are
// core.LLMAnnotation et al., data only. /assess itself never waits on or depends on any of
// this (NFR-6/NFR-5); a missing, down, keyless or rate-limited worker is a `degraded` event on
// the stream, never an HTTP error and never a change to a finding (NFR-11, I3).
//
// STORAGE DECISION (recorded, like PC-19/94 did for the delta): annotations are STORED with the
// version (table `annotations`, keyed by session and version) and replayed on every later read,
// not regenerated. Why: NVIDIA's free tier is rate-limited (~40 RPM, best effort) and a
// narrative run costs ~3 minutes and ~25k tokens, so regenerating on every GET would be slow,
// costly and flaky. What is persisted: a COMPLETE set, or a degraded set that still has at least
// one annotation. A run that produced nothing because the worker was unreachable or keyless is
// NOT persisted, so a later request retries it instead of caching the failure.
package server

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"preflight/core"
)

// annotationsRunTimeout bounds one whole annotation run (a golden assessment takes ~3 min on
// the real model).
const annotationsRunTimeout = 15 * time.Minute

// ---- storage ------------------------------------------------------------------------

func (s *Store) migrateAnnotations() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS annotations (
	session_id       TEXT NOT NULL REFERENCES sessions(id),
	version_number   INTEGER NOT NULL,
	status           TEXT NOT NULL,
	reason           TEXT NOT NULL DEFAULT '',
	model            TEXT NOT NULL DEFAULT '',
	annotations_json TEXT NOT NULL,
	rejected_json    TEXT NOT NULL,
	PRIMARY KEY (session_id, version_number)
)`)
	if err != nil {
		return fmt.Errorf("server: migrate annotations: %w", err)
	}
	return nil
}

// PutAnnotations stores (or replaces) the annotation set of one version.
func (s *Store) PutAnnotations(sessionID string, versionNumber int, set core.LLMAnnotationSet) error {
	ann, err := json.Marshal(nonNilAnnotations(set.Annotations))
	if err != nil {
		return err
	}
	rej, err := json.Marshal(nonNilRejections(set.Rejected))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO annotations (session_id, version_number, status, reason, model, annotations_json, rejected_json)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(session_id, version_number) DO UPDATE SET status=excluded.status, reason=excluded.reason, model=excluded.model,
	annotations_json=excluded.annotations_json, rejected_json=excluded.rejected_json`,
		sessionID, versionNumber, set.Status, set.Reason, set.Model, string(ann), string(rej))
	return err
}

// GetAnnotations reads a version's stored annotation set; ok is false when none was stored.
func (s *Store) GetAnnotations(sessionID string, versionNumber int) (core.LLMAnnotationSet, bool, error) {
	var set core.LLMAnnotationSet
	var ann, rej string
	err := s.db.QueryRow(`SELECT status, reason, model, annotations_json, rejected_json FROM annotations WHERE session_id=? AND version_number=?`,
		sessionID, versionNumber).Scan(&set.Status, &set.Reason, &set.Model, &ann, &rej)
	if err != nil {
		if err == sql.ErrNoRows {
			return core.LLMAnnotationSet{}, false, nil
		}
		return core.LLMAnnotationSet{}, false, err
	}
	if err := json.Unmarshal([]byte(ann), &set.Annotations); err != nil {
		return core.LLMAnnotationSet{}, false, err
	}
	if err := json.Unmarshal([]byte(rej), &set.Rejected); err != nil {
		return core.LLMAnnotationSet{}, false, err
	}
	set.Annotations, set.Rejected = nonNilAnnotations(set.Annotations), nonNilRejections(set.Rejected)
	set.Notice = core.LLMAnnotationNotice
	return set, true, nil
}

func nonNilAnnotations(a []core.LLMAnnotation) []core.LLMAnnotation {
	if a == nil {
		return []core.LLMAnnotation{}
	}
	return a
}

func nonNilRejections(r []core.LLMRejection) []core.LLMRejection {
	if r == nil {
		return []core.LLMRejection{}
	}
	return r
}

// ---- the reason worker (P2) client and job hub --------------------------------------

// ReasonWorker is P1's handle on the reason worker. Configured by URL only — P1 holds no key.
type ReasonWorker struct {
	url    string
	client *http.Client

	mu   sync.Mutex
	jobs map[string]*annotationJob
}

// AttachReasonWorker points this Store at a reason worker (cmd/assessd reads PREFLIGHT_REASOND_URL).
// Optional: with none attached the annotation endpoint reports `degraded` (not configured) and
// every other behaviour is unchanged.
func (s *Store) AttachReasonWorker(url string) {
	s.reasonWorker = &ReasonWorker{url: strings.TrimRight(url, "/"), client: &http.Client{}, jobs: map[string]*annotationJob{}}
}

// annotationEvent is one event on the P1 -> caller stream.
type annotationEvent struct {
	Event string
	Data  any
}

// annotationJob is one running (or just-finished) annotation run for one version, shared by
// every caller streaming it.
type annotationJob struct {
	mu       sync.Mutex
	events   []annotationEvent
	finished bool
	notify   chan struct{}
}

func newAnnotationJob() *annotationJob { return &annotationJob{notify: make(chan struct{})} }

func (j *annotationJob) emit(ev annotationEvent) {
	j.mu.Lock()
	j.events = append(j.events, ev)
	close(j.notify)
	j.notify = make(chan struct{})
	j.mu.Unlock()
}

func (j *annotationJob) finish() {
	j.mu.Lock()
	j.finished = true
	close(j.notify)
	j.notify = make(chan struct{})
	j.mu.Unlock()
}

// next returns events from index i, whether the job is finished, and a channel closed on change.
func (j *annotationJob) next(i int) ([]annotationEvent, bool, <-chan struct{}) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]annotationEvent(nil), j.events[i:]...), j.finished, j.notify
}

// jobFor returns the running job for a version or starts one.
func (rw *ReasonWorker) jobFor(store *Store, sessionID string, versionNumber int, findings []core.Finding) *annotationJob {
	key := sessionID + "/" + strconv.Itoa(versionNumber)
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if j, ok := rw.jobs[key]; ok {
		j.mu.Lock()
		done := j.finished
		j.mu.Unlock()
		if !done {
			return j
		}
		delete(rw.jobs, key) // a finished job that wasn't persisted is retried
	}
	j := newAnnotationJob()
	rw.jobs[key] = j
	go rw.run(store, j, sessionID, versionNumber, findings)
	return j
}

func (rw *ReasonWorker) run(store *Store, j *annotationJob, sessionID string, versionNumber int, findings []core.Finding) {
	defer j.finish()
	ctx, cancel := context.WithTimeout(context.Background(), annotationsRunTimeout)
	defer cancel()

	set := core.LLMAnnotationSet{Annotations: []core.LLMAnnotation{}, Rejected: []core.LLMRejection{}, Notice: core.LLMAnnotationNotice}
	degrade := func(reason string, persist bool) {
		set.Status, set.Reason = core.LLMStatusDegraded, reason
		if persist {
			_ = store.PutAnnotations(sessionID, versionNumber, set)
		}
		j.emit(annotationEvent{"degraded", map[string]string{"reason": reason}})
		j.emit(annotationEvent{"done", doneData(set)})
	}

	body, err := json.Marshal(map[string]any{"findings": findings})
	if err != nil {
		degrade("could not serialise the findings for the reason worker: "+err.Error(), false)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rw.url+"/v1/annotate", bytes.NewReader(body))
	if err != nil {
		degrade("reason worker URL is invalid: "+err.Error(), false)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := rw.client.Do(req)
	if err != nil {
		degrade("the reason worker is unreachable: "+err.Error(), false)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		var d struct{ Degraded, Reason string }
		_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&d)
		reason := "the reason worker is unavailable"
		if d.Degraded == "no_api_key" {
			reason = "the reason worker has no API key configured, so no narratives are generated"
		}
		degrade(reason, false)
		return
	}
	if resp.StatusCode != http.StatusOK {
		degrade(fmt.Sprintf("the reason worker answered HTTP %d", resp.StatusCode), false)
		return
	}

	sawDone := false
	callFailures := 0
	readSSE(resp.Body, func(event, data string) {
		switch event {
		case "started":
			set.Model = jsonField(data, "model")
		case "annotation":
			var a core.LLMAnnotation
			if json.Unmarshal([]byte(data), &a) == nil {
				set.Annotations = append(set.Annotations, a)
				j.emit(annotationEvent{"annotation", a})
			}
		case "rejected":
			var rj core.LLMRejection
			if json.Unmarshal([]byte(data), &rj) == nil {
				set.Rejected = append(set.Rejected, rj)
				if rj.Kind == core.LLMRejectionCallFailed {
					callFailures++
				}
				j.emit(annotationEvent{"rejected", rj})
			}
		case "done":
			sawDone = true
		}
	})
	switch {
	case !sawDone:
		degrade("the reason worker's stream ended before it finished", len(set.Annotations) > 0)
	case len(set.Annotations) == len(findings):
		set.Status = core.LLMStatusComplete
		_ = store.PutAnnotations(sessionID, versionNumber, set)
		j.emit(annotationEvent{"done", doneData(set)})
	default:
		why := fmt.Sprintf("%d of %d findings could not be annotated", len(findings)-len(set.Annotations), len(findings))
		if callFailures > 0 {
			why += fmt.Sprintf(" (%d model calls failed — the provider was unavailable or rate-limited)", callFailures)
		}
		degrade(why, len(set.Annotations) > 0)
	}
}

func doneData(set core.LLMAnnotationSet) map[string]any {
	return map[string]any{"status": set.Status, "reason": set.Reason, "annotated": len(set.Annotations), "rejected": len(set.Rejected)}
}

func jsonField(data, field string) string {
	var m map[string]any
	if json.Unmarshal([]byte(data), &m) != nil {
		return ""
	}
	s, _ := m[field].(string)
	return s
}

// readSSE parses Server-Sent Events from r, calling fn per complete event.
func readSSE(r io.Reader, fn func(event, data string)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var event, data string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "":
			if event != "" {
				fn(event, data)
			}
			event, data = "", ""
		}
	}
}

// ---- HTTP -----------------------------------------------------------------------------

// GetAnnotationsHandler serves GET /sessions/{id}/versions/{n}/annotations:
//
//   - ?format=json returns the STORED set (404 annotations_not_stored when none) — the read-back;
//   - otherwise a text/event-stream: stored annotations replay immediately; if none are stored
//     the reason worker is asked and annotations stream as they arrive. Events: annotation,
//     rejected, degraded {reason}, done {status, annotated, rejected}. It always ends with `done`.
//
// Failure of the narrative layer is a `degraded` event with HTTP 200 — never an error status.
func GetAnnotationsHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("id")
		versionNumber, err := strconv.Atoi(r.PathValue("n"))
		if err != nil {
			writeError(w, newAPIError(http.StatusBadRequest, "invalid_request_body", "invalid version number in path: %s", err))
			return
		}
		exists, err := store.SessionExists(sessionID)
		if err != nil {
			writeError(w, err)
			return
		}
		if !exists {
			writeError(w, newAPIError(http.StatusNotFound, "session_not_found", "server: no session %s found", sessionID))
			return
		}
		stored, ok, err := store.GetVersion(sessionID, versionNumber)
		if err != nil {
			writeError(w, err)
			return
		}
		if !ok {
			writeError(w, newAPIError(http.StatusNotFound, "version_not_found", "server: no version %d found for session %s", versionNumber, sessionID))
			return
		}
		set, have, err := store.GetAnnotations(sessionID, versionNumber)
		if err != nil {
			writeError(w, err)
			return
		}

		if r.URL.Query().Get("format") == "json" {
			if !have {
				writeError(w, newAPIError(http.StatusNotFound, "annotations_not_stored", "server: no annotations are stored for version %d of session %s", versionNumber, sessionID))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(set)
			return
		}

		flusher, canFlush := w.(http.Flusher)
		if !canFlush {
			writeError(w, newAPIError(http.StatusInternalServerError, "streaming_unsupported", "server: streaming is not supported by this connection"))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		send := func(event string, v any) {
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
			flusher.Flush()
		}

		if have {
			for _, a := range set.Annotations {
				send("annotation", a)
			}
			for _, rj := range set.Rejected {
				send("rejected", rj)
			}
			if set.Status == core.LLMStatusDegraded {
				send("degraded", map[string]string{"reason": set.Reason})
			}
			send("done", doneData(set))
			return
		}
		if store.reasonWorker == nil {
			send("degraded", map[string]string{"reason": "no reason worker is configured (PREFLIGHT_REASOND_URL), so no narratives are generated"})
			send("done", map[string]any{"status": core.LLMStatusDegraded, "annotated": 0, "rejected": 0})
			return
		}

		job := store.reasonWorker.jobFor(store, sessionID, versionNumber, stored.Findings)
		for i := 0; ; {
			events, finished, wait := job.next(i)
			for _, ev := range events {
				send(ev.Event, ev.Data)
			}
			i += len(events)
			if finished && len(events) == 0 {
				return
			}
			if finished {
				continue
			}
			select {
			case <-wait:
			case <-r.Context().Done():
				return
			}
		}
	}
}
