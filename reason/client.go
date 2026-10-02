package reason

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is NVIDIA's hosted OpenAI-compatible endpoint (ADR-005).
const DefaultBaseURL = "https://integrate.api.nvidia.com/v1"

// Config configures a Client. APIKey is the only secret; it is sent as a bearer token
// and never logged or placed in an error message.
type Config struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
	// Reasoning selects how much the model thinks before answering (PC-160). "" leaves the
	// provider default untouched (the request is byte-identical to before this field existed);
	// ReasoningOff and ReasoningLowEffort map to NVIDIA's documented chat_template_kwargs
	// (model card: enable_thinking / low_effort). Whether the hosted endpoint honours them is
	// measured, not assumed — see docs/eval/nfr9-first-annotation-latency.md.
	Reasoning string
}

// Reasoning modes accepted in Config.Reasoning.
const (
	ReasoningDefault   = ""
	ReasoningOff       = "off"
	ReasoningLowEffort = "low_effort"
)

// Client is a minimal chat-completions client over net/http. No SDK (ADR-005).
type Client struct {
	cfg Config
}

// NewClient validates cfg. A missing key or model is a configuration error, not
// something to discover on the first request.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("reason: no API key configured")
	}
	if cfg.Model == "" {
		return nil, errors.New("reason: no model configured")
	}
	switch cfg.Reasoning {
	case ReasoningDefault, ReasoningOff, ReasoningLowEffort:
	default:
		return nil, fmt.Errorf("reason: unknown reasoning mode %q (want \"\", %q or %q)", cfg.Reasoning, ReasoningOff, ReasoningLowEffort)
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 2 * time.Minute}
	}
	return &Client{cfg: cfg}, nil
}

// Model returns the configured model ID.
func (c *Client) Model() string { return c.cfg.Model }

// Message is one chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Request is a chat-completions request. MaxTokens must leave room for the model's
// reasoning, which is billed against it before the answer starts.
type Request struct {
	Messages    []Message
	Temperature float64
	TopP        float64
	MaxTokens   int
}

// Usage is the token accounting the API returns.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Result is a completed generation. Content is the answer only; reasoning text is
// counted but deliberately not returned (it is scratch work, not narrative).
type Result struct {
	Content         string
	FinishReason    string
	Usage           Usage
	ReasoningChunks int
}

// ErrEmptyAnswer means the model finished without any answer text, typically because
// reasoning consumed the whole max_tokens budget.
var ErrEmptyAnswer = errors.New("reason: the model returned no answer text (reasoning may have used the whole token budget)")

type wireRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	TopP        float64   `json:"top_p"`
	MaxTokens   int       `json:"max_tokens"`
	Stream      bool      `json:"stream"`
	// ChatTemplateKwargs is omitted entirely unless a reasoning mode was chosen.
	ChatTemplateKwargs map[string]bool `json:"chat_template_kwargs,omitempty"`
}

func reasoningKwargs(mode string) map[string]bool {
	switch mode {
	case ReasoningOff:
		return map[string]bool{"enable_thinking": false}
	case ReasoningLowEffort:
		return map[string]bool{"enable_thinking": true, "low_effort": true}
	}
	return nil
}

type wireChunk struct {
	Choices []struct {
		Delta struct {
			Content          *string `json:"content"`
			ReasoningContent *string `json:"reasoning_content"`
		} `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
}

func (c *Client) do(ctx context.Context, req Request, stream bool) (*http.Response, error) {
	body, err := json.Marshal(wireRequest{
		Model: c.cfg.Model, Messages: req.Messages, Temperature: req.Temperature, TopP: req.TopP,
		MaxTokens: req.MaxTokens, Stream: stream, ChatTemplateKwargs: reasoningKwargs(c.cfg.Reasoning),
	})
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	resp, err := c.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, redact(err, c.cfg.APIKey)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("reason: chat completions returned HTTP %d: %s", resp.StatusCode, redactString(string(snippet), c.cfg.APIKey))
	}
	return resp, nil
}

// Chat performs a non-streaming completion.
func (c *Client) Chat(ctx context.Context, req Request) (Result, error) {
	resp, err := c.do(ctx, req, false)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	var w wireChunk
	if err := json.NewDecoder(resp.Body).Decode(&w); err != nil {
		return Result{}, fmt.Errorf("reason: decoding response: %w", err)
	}
	if len(w.Choices) == 0 {
		return Result{}, errors.New("reason: response has no choices")
	}
	out := Result{Content: w.Choices[0].Message.Content}
	if w.Choices[0].FinishReason != nil {
		out.FinishReason = *w.Choices[0].FinishReason
	}
	if w.Usage != nil {
		out.Usage = *w.Usage
	}
	if strings.TrimSpace(out.Content) == "" {
		return out, ErrEmptyAnswer
	}
	return out, nil
}

// ChatStream performs a streaming completion, calling onDelta for each piece of ANSWER
// text as it arrives (reasoning_content is never passed to onDelta). A stream that
// ends without `data: [DONE]` is an error — a cut connection must not look like a
// complete narrative.
func (c *Client) ChatStream(ctx context.Context, req Request, onDelta func(string)) (Result, error) {
	resp, err := c.do(ctx, req, true)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	return parseStream(resp.Body, onDelta)
}

func parseStream(r io.Reader, onDelta func(string)) (Result, error) {
	var out Result
	var answer strings.Builder
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	done := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue // blank separators, comments
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			done = true
			break
		}
		var chunk wireChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return out, fmt.Errorf("reason: malformed stream chunk: %w", err)
		}
		if chunk.Usage != nil {
			out.Usage = *chunk.Usage
		}
		// The final usage chunk has an empty choices array — not an error, not a delta.
		for _, ch := range chunk.Choices {
			if ch.Delta.ReasoningContent != nil {
				out.ReasoningChunks++
			}
			if ch.Delta.Content != nil && *ch.Delta.Content != "" {
				answer.WriteString(*ch.Delta.Content)
				if onDelta != nil {
					onDelta(*ch.Delta.Content)
				}
			}
			if ch.FinishReason != nil {
				out.FinishReason = *ch.FinishReason
			}
		}
	}
	if err := sc.Err(); err != nil {
		return out, fmt.Errorf("reason: reading stream: %w", err)
	}
	out.Content = answer.String()
	if !done {
		return out, errors.New("reason: stream ended before [DONE] — the response is incomplete")
	}
	if strings.TrimSpace(out.Content) == "" {
		return out, ErrEmptyAnswer
	}
	return out, nil
}

func redact(err error, key string) error {
	if key == "" || err == nil {
		return err
	}
	return errors.New(redactString(err.Error(), key))
}

func redactString(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "[redacted]")
}
