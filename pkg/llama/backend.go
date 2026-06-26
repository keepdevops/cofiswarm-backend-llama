package llama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/keepdevops/cofiswarm-backend-sdk/pkg/backend"
)

// Backend implements the InferenceBackend contract against a llama-server
// (llama.cpp) OpenAI-compatible endpoint: streaming chat, embeddings, and
// health. It coexists with Client, the non-streaming convenience helper, and is
// the llama-side peer of cofiswarm-backend-mlx.
type Backend struct {
	Host         string
	Port         int
	AgentID      string
	SystemPrompt string
	MaxTokens    int
	Temperature  float64
	baseURL      string
	client       *http.Client
}

var _ backend.InferenceBackend = (*Backend)(nil)

// NewBackend constructs a llama Backend (defaults: 127.0.0.1:8080, max_tokens 512,
// temperature 0.2, 300s stream timeout — matching MlxBackend's posture).
func NewBackend(host string, port int, agentID, systemPrompt string, maxTokens int, temperature float64) *Backend {
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 8080
	}
	if maxTokens <= 0 {
		maxTokens = backend.DefaultMaxTokens
	}
	if temperature == 0 {
		temperature = 0.2
	}
	return &Backend{
		Host: host, Port: port, AgentID: agentID, SystemPrompt: systemPrompt,
		MaxTokens: maxTokens, Temperature: temperature,
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
		client:  &http.Client{Timeout: 300 * time.Second},
	}
}

// messages builds the OpenAI chat array. Unlike MLX (which merges system into the
// user turn), llama-server honours a distinct system role.
func (b *Backend) messages(prompt string) []map[string]string {
	var msgs []map[string]string
	if b.SystemPrompt != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": b.SystemPrompt})
	}
	return append(msgs, map[string]string{"role": "user", "content": prompt})
}

// GenerateStream POSTs a streaming chat completion, calling emit for each content
// delta and a final Done chunk. Connection/HTTP failures return a Go error so the
// caller can mark the agent unavailable (rather than yielding error text).
func (b *Backend) GenerateStream(ctx context.Context, req backend.GenerateRequest, emit func(backend.TokenChunk) error) error {
	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = b.MaxTokens
	}
	temp := req.Temperature
	if temp == 0 {
		temp = b.Temperature
	}
	payload := map[string]any{
		"messages":     b.messages(req.Prompt),
		"max_tokens":   maxTok,
		"temperature":  temp,
		"cache_prompt": true,
		"stream":       true,
	}
	if len(req.Stop) > 0 {
		payload["stop"] = req.Stop
	}
	body, _ := json.Marshal(payload)

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("llama-backend %s: connection error: %w", b.AgentID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("llama-backend %s: HTTP %d: %s", b.AgentID, resp.StatusCode, truncate(string(raw), 200))
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(line[len("data:"):])
		if data == "[DONE]" {
			break
		}
		var ev struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue // skip a malformed SSE frame
		}
		if len(ev.Choices) > 0 && ev.Choices[0].Delta.Content != "" {
			if err := emit(backend.TokenChunk{Text: ev.Choices[0].Delta.Content}); err != nil {
				return err
			}
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("llama-backend %s: stream read: %w", b.AgentID, err)
	}
	return emit(backend.TokenChunk{Done: true})
}

// Embed calls the OpenAI-compatible /v1/embeddings endpoint (the server must be
// started with --embedding). Returns one vector per input text.
func (b *Backend) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body, _ := json.Marshal(map[string]any{"input": texts})
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+"/v1/embeddings", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llama-backend %s: embed connection error: %w", b.AgentID, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llama-backend %s: embed HTTP %d: %s", b.AgentID, resp.StatusCode, truncate(string(raw), 200))
	}
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("llama-backend %s: embed decode: %w", b.AgentID, err)
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("llama-backend %s: embed returned %d vectors for %d inputs", b.AgentID, len(out.Data), len(texts))
	}
	vecs := make([][]float32, len(out.Data))
	for i, d := range out.Data {
		vecs[i] = d.Embedding
	}
	return vecs, nil
}

// Health probes GET /health; OK on HTTP 200 (llama-server exposes /health).
func (b *Backend) Health(ctx context.Context) backend.HealthStatus {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, b.baseURL+"/health", nil)
	cl := &http.Client{Timeout: 5 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return backend.HealthStatus{OK: false, Detail: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return backend.HealthStatus{OK: true, Detail: fmt.Sprintf("port %d ok", b.Port)}
	}
	return backend.HealthStatus{OK: false, Detail: fmt.Sprintf("port %d HTTP %d", b.Port, resp.StatusCode)}
}

// Close is a no-op (http.Client needs no teardown).
func (b *Backend) Close() error { return nil }
