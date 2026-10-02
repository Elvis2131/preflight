//go:build live_llm

package reason

// Deliberately behind a build tag, like cmd/runnerd/internal/pricingfetch's live test: a
// real call to NVIDIA's endpoint must never fail an unrelated commit. Run with
//   NVIDIA_API_KEY=... go test -tags live_llm ./reason/ -run Live -v
// The key is read from the environment only and is never written anywhere.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func liveClient(t *testing.T) *Client {
	t.Helper()
	key := os.Getenv("NVIDIA_API_KEY")
	if key == "" {
		t.Skip("NVIDIA_API_KEY not set")
	}
	c, err := NewClient(Config{APIKey: key, Model: "nvidia/nemotron-3-super-120b-a12b"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Confirms against the REAL API what the ADR requires be confirmed before SSE handling
// is built: OpenAI-style deltas, reasoning separated from the answer, usage chunk, [DONE].
func TestLive_StreamShape(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var deltas int
	res, err := c.ChatStream(ctx, Request{Messages: []Message{{Role: "user", Content: "Which number is larger, 9.11 or 9.8? One sentence."}}, Temperature: 0.5, TopP: 1, MaxTokens: 1024}, func(string) { deltas++ })
	if err != nil {
		t.Fatalf("live stream failed: %v", err)
	}
	if !strings.Contains(res.Content, "9.8") || deltas == 0 || res.FinishReason != "stop" || res.Usage.TotalTokens == 0 {
		t.Errorf("unexpected live result: %+v (deltas=%d)", res, deltas)
	}
	t.Logf("live: %d answer deltas, %d reasoning chunks, usage %+v", deltas, res.ReasoningChunks, res.Usage)
}
