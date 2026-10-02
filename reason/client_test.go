package reason

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const testKey = "test-secret-key-do-not-leak"

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewClient(Config{BaseURL: srv.URL, APIKey: testKey, Model: "nvidia/nemotron-3-super-120b-a12b"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func serveFile(t *testing.T, path string) http.HandlerFunc {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write(b)
	}
}

// The fixture is a REAL response from NVIDIA's API (2026-10-02): reasoning_content
// chunks first, then content, then a usage chunk with an empty choices array, then
// [DONE]. Only the answer may reach the caller.
func TestChatStream_RealCapturedStream(t *testing.T) {
	c := newTestClient(t, serveFile(t, "testdata/nvidia_stream_real.sse"))
	var deltas []string
	res, err := c.ChatStream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}, MaxTokens: 256}, func(s string) { deltas = append(deltas, s) })
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != "9.8 is larger than 9.11." {
		t.Errorf("Content = %q, want only the answer text", res.Content)
	}
	if strings.Join(deltas, "") != res.Content {
		t.Errorf("onDelta saw %q, want exactly the answer %q (reasoning must never be delivered)", strings.Join(deltas, ""), res.Content)
	}
	if res.ReasoningChunks == 0 {
		t.Error("the capture has reasoning chunks; they should be counted")
	}
	if res.FinishReason != "stop" || res.Usage.TotalTokens != 90 || res.Usage.CompletionTokens != 53 {
		t.Errorf("finish/usage = %q %+v, want stop and the real usage chunk (90 total, 53 completion)", res.FinishReason, res.Usage)
	}
}

func TestChatStream_SendsTheDocumentedRequest(t *testing.T) {
	var gotAuth, gotPath string
	var body map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		json.NewDecoder(r.Body).Decode(&body)
		serveFile(t, "testdata/nvidia_stream_real.sse")(w, r)
	})
	if _, err := c.ChatStream(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}, Temperature: 0.5, TopP: 1, MaxTokens: 1024}, nil); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/chat/completions" || gotAuth != "Bearer "+testKey {
		t.Errorf("path=%q auth=%q", gotPath, gotAuth)
	}
	if body["model"] != "nvidia/nemotron-3-super-120b-a12b" || body["stream"] != true || body["max_tokens"] != float64(1024) {
		t.Errorf("request body = %v", body)
	}
}

func TestChatStream_TruncatedStreamIsAnError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")) // connection ends, no [DONE]
	})
	res, err := c.ChatStream(context.Background(), Request{}, nil)
	if err == nil || !strings.Contains(err.Error(), "[DONE]") {
		t.Fatalf("got %v, want an incomplete-stream error", err)
	}
	if res.Content != "partial" {
		t.Errorf("Content = %q; what arrived is still reported alongside the error", res.Content)
	}
}

func TestChatStream_ReasoningOnly_EmptyAnswerError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking\"}}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n"))
	})
	_, err := c.ChatStream(context.Background(), Request{}, nil)
	if !errors.Is(err, ErrEmptyAnswer) {
		t.Fatalf("got %v, want ErrEmptyAnswer (reasoning used the whole budget)", err)
	}
}

func TestChat_NonStreaming(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	})
	res, err := c.Chat(context.Background(), Request{})
	if err != nil || res.Content != "hello" || res.Usage.TotalTokens != 5 {
		t.Fatalf("got %+v err=%v", res, err)
	}
}

// An upstream error body can echo request details; the key must never come back out.
func TestErrors_NeverContainTheAPIKey(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid key ` + testKey + `"}`))
	})
	_, err := c.Chat(context.Background(), Request{})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("got %v, want an HTTP 401 error", err)
	}
	if strings.Contains(err.Error(), testKey) {
		t.Errorf("error leaks the API key: %v", err)
	}
}

func TestChatStream_ContextCancelStopsTheCall(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body) // the server only notices a disconnect once the body is read
		<-r.Context().Done()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := c.ChatStream(ctx, Request{}, nil); err == nil {
		t.Fatal("a cancelled context must fail the call, not hang")
	}
}

func TestNewClient_RequiresKeyAndModel(t *testing.T) {
	if _, err := NewClient(Config{Model: "m"}); err == nil {
		t.Error("missing key must be refused at construction")
	}
	if _, err := NewClient(Config{APIKey: "k"}); err == nil {
		t.Error("missing model must be refused at construction")
	}
}

// PC-160: a reasoning mode only adds chat_template_kwargs when chosen; the default request must
// stay exactly what it was, so nothing about existing behaviour changes unless a mode is opted in.
func TestReasoningMode_WireFormat(t *testing.T) {
	cases := []struct {
		mode string
		want string // the chat_template_kwargs JSON, or "" when the key must be absent
	}{
		{ReasoningDefault, ""},
		{ReasoningOff, `{"enable_thinking":false}`},
		{ReasoningLowEffort, `{"enable_thinking":true,"low_effort":true}`},
	}
	for _, tc := range cases {
		var got map[string]json.RawMessage
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &got)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"total_tokens":1}}`))
		}))
		c, err := NewClient(Config{BaseURL: srv.URL, APIKey: testKey, Model: "m", Reasoning: tc.mode})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}, MaxTokens: 8}); err != nil {
			t.Fatal(err)
		}
		srv.Close()
		raw, present := got["chat_template_kwargs"]
		if tc.want == "" && present {
			t.Errorf("mode %q: chat_template_kwargs must be absent, got %s", tc.mode, raw)
		}
		if tc.want != "" && string(raw) != tc.want {
			t.Errorf("mode %q: chat_template_kwargs = %s, want %s", tc.mode, raw, tc.want)
		}
	}
	if _, err := NewClient(Config{APIKey: testKey, Model: "m", Reasoning: "max"}); err == nil {
		t.Error("an unknown reasoning mode must be a configuration error")
	}
}

// PC-160: the operator-facing setting resolves to the measured product default.
func TestProductReasoning(t *testing.T) {
	for in, want := range map[string]string{
		"":                       ReasoningLowEffort,
		ReasoningProviderDefault: ReasoningDefault,
		ReasoningOff:             ReasoningOff,
		ReasoningLowEffort:       ReasoningLowEffort,
	} {
		if got := ProductReasoning(in); got != want {
			t.Errorf("ProductReasoning(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := NewClient(Config{APIKey: testKey, Model: "m", Reasoning: ProductReasoning("nonsense")}); err == nil {
		t.Error("a mistyped setting must be a start-up error, not silently the default")
	}
}
