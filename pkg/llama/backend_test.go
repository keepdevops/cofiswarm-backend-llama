package llama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/keepdevops/cofiswarm-backend-sdk/pkg/backend"
)

func newBackendOn(t *testing.T, h http.HandlerFunc) (*Backend, func()) {
	t.Helper()
	srv := httptest.NewServer(h)
	b := NewBackend("", 0, "scout", "You are Scout.", 64, 0)
	b.baseURL = srv.URL
	return b, srv.Close
}

func TestBackendGenerateStream(t *testing.T) {
	var roles []string
	b, stop := newBackendOn(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(404)
			return
		}
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, m := range body.Messages {
			roles = append(roles, m.Role)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, tok := range []string{"SH", "IP"} {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", tok)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	defer stop()

	var out string
	var sawDone bool
	err := b.GenerateStream(context.Background(),
		backend.GenerateRequest{Prompt: "rate it"},
		func(c backend.TokenChunk) error {
			out += c.Text
			if c.Done {
				sawDone = true
			}
			return nil
		})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if out != "SHIP" {
		t.Errorf("content=%q, want SHIP", out)
	}
	if !sawDone {
		t.Error("never received the final Done chunk")
	}
	if len(roles) != 2 || roles[0] != "system" || roles[1] != "user" {
		t.Errorf("roles=%v, want [system user]", roles)
	}
}

// emit returning an error must abort the stream (caller-driven cancellation).
func TestBackendGenerateStreamEmitAbort(t *testing.T) {
	b, stop := newBackendOn(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < 5; i++ {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	defer stop()

	want := fmt.Errorf("client gone")
	got := 0
	err := b.GenerateStream(context.Background(), backend.GenerateRequest{Prompt: "p"},
		func(backend.TokenChunk) error { got++; return want })
	if err != want {
		t.Errorf("err=%v, want %v", err, want)
	}
	if got != 1 {
		t.Errorf("emit called %d times, want 1 (abort on first error)", got)
	}
}

func TestBackendEmbed(t *testing.T) {
	b, stop := newBackendOn(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]},{"embedding":[0.3,0.4]}]}`))
	})
	defer stop()

	vecs, err := b.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(vecs) != 2 || len(vecs[0]) != 2 || vecs[1][0] != 0.3 {
		t.Errorf("vecs=%v", vecs)
	}
}

func TestBackendEmbedCountMismatch(t *testing.T) {
	b, stop := newBackendOn(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1]}]}`))
	})
	defer stop()
	if _, err := b.Embed(context.Background(), []string{"a", "b"}); err == nil {
		t.Error("want error when vector count != input count")
	}
}

func TestBackendHealth(t *testing.T) {
	b, stop := newBackendOn(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	})
	defer stop()

	if h := b.Health(context.Background()); !h.OK {
		t.Errorf("health not OK: %+v", h)
	}

	bad := NewBackend("127.0.0.1", 1, "scout", "", 0, 0) // nothing listening
	if h := bad.Health(context.Background()); h.OK {
		t.Error("health should be false when unreachable")
	}
	if err := bad.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}
