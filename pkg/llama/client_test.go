package llama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestChatAndHealth(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(200)
		case "/v1/chat/completions":
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"SHIP"}}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	host, portStr, _ := strings.Cut(strings.TrimPrefix(srv.URL, "http://"), ":")
	port, _ := strconv.Atoi(portStr)
	c := New(host, port)

	out, err := c.Chat(context.Background(), "be terse", "review this", 64)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if out != "SHIP" {
		t.Errorf("content=%q", out)
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 2 {
		t.Errorf("messages=%v (want system+user)", gotBody["messages"])
	}
	if gotBody["max_tokens"].(float64) != 64 {
		t.Errorf("max_tokens=%v", gotBody["max_tokens"])
	}
	if !c.Health(context.Background()) {
		t.Error("health should be true on 200")
	}

	// unreachable host -> health false, chat error (port 1: nothing listens)
	bad := New("127.0.0.1", 1)
	if bad.Health(context.Background()) {
		t.Error("health should be false when unreachable")
	}
	if _, err := bad.Chat(context.Background(), "", "x", 0); err == nil {
		t.Error("chat to unreachable should error")
	}
}
