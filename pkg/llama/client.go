// Package llama is a thin HTTP client for a llama-server OpenAI-compatible endpoint
// (Go port of cofiswarm_backend_llama.client.LlamaClient). Coexists with the Python client.
package llama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks to a llama-server at host:port.
type Client struct {
	Host    string
	Port    int
	Timeout time.Duration
	http    *http.Client
}

// New returns a client (defaults: 127.0.0.1:8080, 120s timeout — matching LlamaClient).
func New(host string, port int) *Client {
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 8080
	}
	return &Client{Host: host, Port: port, Timeout: 120 * time.Second, http: &http.Client{Timeout: 120 * time.Second}}
}

func (c *Client) baseURL() string { return fmt.Sprintf("http://%s:%d", c.Host, c.Port) }

// Chat sends one non-streaming chat completion and returns the assistant content.
func (c *Client) Chat(ctx context.Context, systemPrompt, prompt string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 512
	}
	var messages []map[string]string
	if systemPrompt != "" {
		messages = append(messages, map[string]string{"role": "system", "content": systemPrompt})
	}
	messages = append(messages, map[string]string{"role": "user", "content": prompt})
	body, _ := json.Marshal(map[string]any{
		"messages":     messages,
		"max_tokens":   maxTokens,
		"cache_prompt": true,
		"stop":         []string{"", "<|im_start|>", "<|eot_id|>"},
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("llama chat %s: %w", c.baseURL(), err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llama chat HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("llama chat decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llama chat: empty choices")
	}
	return out.Choices[0].Message.Content, nil
}

// Health GETs /health; true on HTTP 200.
func (c *Client) Health(ctx context.Context) bool {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL()+"/health", nil)
	cl := &http.Client{Timeout: 5 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
