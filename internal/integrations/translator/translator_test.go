package translator

import (
	"context"
	"encoding/json"
	"lam.local/lam2/internal/gateway"
	"strings"
	"testing"
)

func TestRegisteredNonStreamDirection(t *testing.T) {
	tr, e := New()
	if e != nil {
		t.Fatal(e)
	}
	ex, e := tr.New(context.Background(), []byte(`{"model":"m","input":"hello","stream":false}`), "m", false)
	if e != nil {
		t.Fatal(e)
	}
	var req map[string]any
	if e = json.Unmarshal(ex.Request(), &req); e != nil {
		t.Fatal(e)
	}
	if _, ok := req["messages"]; !ok {
		t.Fatalf("not chat: %s", ex.Request())
	}
	out, e := ex.NonStream(context.Background(), []byte(`{"id":"chat-1","object":"chat.completion","created":1770000000,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110,"prompt_tokens_details":{"cached_tokens":40}}}`))
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(out), `"output"`) || !strings.Contains(string(out), "hello") {
		t.Fatalf("not responses: %s", out)
	}
}
func TestStreamTerminalOnce(t *testing.T) {
	tr, e := New()
	if e != nil {
		t.Fatal(e)
	}
	ex, e := tr.New(context.Background(), []byte(`{"model":"m","input":"hello","stream":true}`), "m", true)
	if e != nil {
		t.Fatal(e)
	}
	var all string
	for _, d := range []string{`{"id":"c1","created":1770000000,"model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":"hello"},"finish_reason":null}]}`, `{"id":"c1","created":1770000000,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`, `[DONE]`} {
		chunks, e := ex.Stream(context.Background(), gateway.Frame{Data: []byte(d)})
		if e != nil {
			t.Fatal(e)
		}
		for _, b := range chunks {
			all += string(b)
		}
	}
	if !strings.Contains(all, "hello") || strings.Count(all, "event: response.completed") != 1 {
		t.Fatalf("unexpected events: %s", all)
	}
}
