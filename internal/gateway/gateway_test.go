package gateway

import (
	"context"
	"encoding/json"
	"io"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSSEFraming(t *testing.T) {
	raw := ": heartbeat\r\ndata: {\"text\":\"hi\"}\r\n\r\nevent: complete\ndata: [DONE]\n\n"
	var got []Frame
	e := ReadSSE(strings.NewReader(raw), 256, func(f Frame) error { got = append(got, f); return nil })
	if e != nil || len(got) != 2 || string(got[1].Data) != "[DONE]" {
		t.Fatal(got, e)
	}
}
func TestSSEOversizeAndIncomplete(t *testing.T) {
	if ReadSSE(strings.NewReader("data: "+strings.Repeat("x", 1024)+"\n\n"), 100, func(Frame) error { return nil }) == nil {
		t.Fatal("unbounded SSE")
	}
	var got []Frame
	e := ReadSSE(strings.NewReader("data: last"), 100, func(f Frame) error { got = append(got, f); return nil })
	if e != nil || len(got) != 1 {
		t.Fatal(e)
	}
}
func TestLimiterQueuesAndCancellation(t *testing.T) {
	l := NewLimiter(1, 1, 1, 1)
	ctx := context.Background()
	release, e := l.Acquire(ctx, "a")
	if e != nil {
		t.Fatal(e)
	}
	c, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, e := l.Acquire(c, "a")
		if e == nil {
			t.Error("cancelled waiter acquired")
		}
	}()
	for i := 0; i < 100 && l.Snapshot().Queued == 0; i++ {
		time.Sleep(time.Millisecond)
	}
	if _, e = l.Acquire(ctx, "a"); fault.Code(e) != "GATEWAY_BUSY" {
		t.Fatal(e)
	}
	cancel()
	wg.Wait()
	release()
	release()
	rel, e := l.Acquire(ctx, "b")
	if e != nil {
		t.Fatal(e)
	}
	rel()
	if s := l.Snapshot(); s.Active != 0 || s.Queued != 0 {
		t.Fatal(s)
	}
}

type testResolver struct{ snapshot Snapshot }

func (r testResolver) Resolve(ctx context.Context, tok string) (Snapshot, error) {
	if tok != "binding-token" {
		return Snapshot{}, fault.New("GATEWAY_UNAUTHORIZED", "invalid")
	}
	return r.snapshot, nil
}

// These transport tests deliberately use a fake converter. They do NOT certify CLIProxyAPI.
type testFactory struct{}

func (testFactory) New(ctx context.Context, raw []byte, model string, stream bool) (Exchange, error) {
	return &testExchange{}, nil
}

type testExchange struct{}

func (*testExchange) Request() []byte { return []byte(`{"model":"model-a","messages":[]}`) }
func (*testExchange) NonStream(ctx context.Context, raw []byte) ([]byte, error) {
	return []byte(`{"object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":2}}`), nil
}
func (*testExchange) Stream(ctx context.Context, f Frame) ([][]byte, error) {
	if string(f.Data) == "[DONE]" {
		return [][]byte{[]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")}, nil
	}
	return [][]byte{[]byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n")}, nil
}
func serverFor(t *testing.T, up http.HandlerFunc) (*Server, *httptest.Server) {
	t.Helper()
	u := httptest.NewServer(up)
	t.Cleanup(u.Close)
	caps := model.Capabilities{Streaming: "supported", FunctionTools: "supported", Images: "supported", StructuredOutputs: "supported", Reasoning: "supported", CustomTools: "unknown", Namespaces: "unknown"}
	sn := Snapshot{Binding: model.Binding{ID: "binding-a", Model: "model-a", ProviderID: "p", Generation: 1}, Provider: model.Provider{ID: "p", Protocol: "chat_completions", BaseURL: u.URL, Capabilities: caps}, Headers: http.Header{"Authorization": []string{"Bearer upstream-secret"}}}
	s := NewServer(model.DefaultLimits(), testResolver{sn}, testFactory{})
	return s, httptest.NewServer(s)
}
func post(t *testing.T, url, token, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", url+"/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestUnauthorizedNeverContactsUpstream(t *testing.T) {
	s, h := serverFor(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unauthorized upstream") })
	_ = s
	defer h.Close()
	r := post(t, h.URL, "wrong", `{"model":"model-a","input":"hello"}`)
	defer r.Body.Close()
	if r.StatusCode != 401 {
		t.Fatal(r.Status)
	}
}
func TestNonStreamTransport(t *testing.T) {
	s, h := serverFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer upstream-secret" {
			t.Error("wrong route or credential")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"hello"}}]}`)
	})
	defer h.Close()
	r := post(t, h.URL, "binding-token", `{"model":"model-a","input":"hello"}`)
	defer r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatal(r.Status)
	}
	raw, _ := io.ReadAll(r.Body)
	if !strings.Contains(string(raw), "completed") {
		t.Fatal(string(raw))
	}
	if s.Limiter.Snapshot().Active != 0 {
		t.Fatal("leaked slot")
	}
}
func TestStreamingTerminalExactlyOnce(t *testing.T) {
	_, h := serverFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\ndata: [DONE]\n\n")
	})
	defer h.Close()
	r := post(t, h.URL, "binding-token", `{"model":"model-a","input":"hello","stream":true}`)
	defer r.Body.Close()
	raw, _ := io.ReadAll(r.Body)
	if strings.Count(string(raw), "event: response.completed") != 1 {
		t.Fatal(string(raw))
	}
}
func TestMissingTerminalIsErrorNotSuccess(t *testing.T) {
	_, h := serverFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[]}\n\n")
	})
	defer h.Close()
	r := post(t, h.URL, "binding-token", `{"model":"model-a","input":"hello","stream":true}`)
	defer r.Body.Close()
	raw, _ := io.ReadAll(r.Body)
	if strings.Contains(string(raw), "response.completed") || !strings.Contains(string(raw), "UPSTREAM_TRUNCATED") {
		t.Fatal(string(raw))
	}
}
func TestUpstreamStatusRedaction(t *testing.T) {
	_, h := serverFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(429)
		io.WriteString(w, "secret API key upstream-secret")
	})
	defer h.Close()
	r := post(t, h.URL, "binding-token", `{"model":"model-a","input":"hello"}`)
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 429 || r.Header.Get("Retry-After") != "2" || strings.Contains(string(b), "upstream-secret") {
		t.Fatal(r.Status, string(b))
	}
}
func TestProtocolPreflightRejectsLoss(t *testing.T) {
	caps := model.UnknownCapabilities()
	for _, raw := range []string{`{"input":"x","previous_response_id":"old"}`, `{"input":"x","unknown_option":true}`, `{"input":"x","tools":[{"type":"web_search"}]}`, `{"input":"x","tools":[{"type":"custom","name":"x"}]}`, `{"input":"x","stream":true}`} {
		if e := Preflight([]byte(raw), caps, model.DefaultLimits()); fault.Code(e) != "PROTOCOL_LOSS" {
			t.Fatal(raw, e)
		}
	}
}
func TestRequestModelIsolation(t *testing.T) {
	_, h := serverFor(t, func(w http.ResponseWriter, r *http.Request) { t.Error("wrong model reached upstream") })
	defer h.Close()
	r := post(t, h.URL, "binding-token", `{"model":"another-profile-model","input":"hello"}`)
	defer r.Body.Close()
	if r.StatusCode != 400 {
		t.Fatal(r.Status)
	}
}
func TestInputValidation(t *testing.T) {
	for _, raw := range []string{"{", `{"model":"a","input":3}`, `{"input":null}`, `{"input":"x","stream":"yes"}`} {
		if Preflight([]byte(raw), model.UnknownCapabilities(), model.DefaultLimits()) == nil {
			t.Fatal(raw)
		}
	}
}
func TestBodyLimit(t *testing.T) {
	s, h := serverFor(t, func(w http.ResponseWriter, r *http.Request) { t.Error("too large reached upstream") })
	s.Limits.BodyBytes = 32
	defer h.Close()
	r := post(t, h.URL, "binding-token", `{"model":"model-a","input":"`+strings.Repeat("x", 200)+`"}`)
	defer r.Body.Close()
	if r.StatusCode != 413 {
		t.Fatal(r.Status)
	}
}
func TestUnknownRoutes(t *testing.T) {
	_, h := serverFor(t, func(w http.ResponseWriter, r *http.Request) {})
	defer h.Close()
	r, e := http.Get(h.URL + "/v1/unknown")
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	if r.StatusCode != 404 {
		t.Fatal(r.Status)
	}
}
func TestHealthChallenge(t *testing.T) {
	s, h := serverFor(t, func(w http.ResponseWriter, r *http.Request) {})
	s.Identity = Identity{Nonce: "instance-secret-at-least-16", ExecutableHash: "hash", PID: 12}
	defer h.Close()
	r, e := http.Get(h.URL + "/health?challenge=0123456789abcdef0123456789abcdef")
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	var out Health
	json.NewDecoder(r.Body).Decode(&out)
	if !VerifyHealth(s.Identity, "0123456789abcdef0123456789abcdef", out) {
		t.Fatal(out)
	}
	out.Proof = "wrong"
	if VerifyHealth(s.Identity, "0123456789abcdef0123456789abcdef", out) {
		t.Fatal("stranger trusted")
	}
}
func TestCancelReleasesUpstream(t *testing.T) {
	cancelled := make(chan struct{})
	_, h := serverFor(t, func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
			close(cancelled)
		case <-time.After(3 * time.Second):
		}
	})
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", h.URL+"/v1/responses", strings.NewReader(`{"model":"model-a","input":"hello"}`))
	req.Header.Set("Authorization", "Bearer binding-token")
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("upstream not cancelled")
	}
}

func TestCodexModelCatalogUsesBoundExplicitMetadata(t *testing.T) {
	const catalog = `{"slug":"model-a","display_name":"Model A","supported_reasoning_levels":[],"shell_type":"shell_command","visibility":"list","supported_in_api":true,"priority":1,"base_instructions":"Explicit provider instructions","supports_reasoning_summaries":false,"support_verbosity":false,"truncation_policy":{"limit":10000,"mode":"tokens"},"supports_parallel_tool_calls":false,"experimental_supported_tools":[]}`
	// JSON input exercises the persisted Provider shape, not a hard-coded response fixture.
	var p model.Provider
	raw := `{"id":"p","protocol":"chat_completions","models":[{"id":"model-a","codexCatalog":` + catalog + `}]}`
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	s := NewServer(model.DefaultLimits(), testResolver{Snapshot{Binding: model.Binding{ID: "b", Model: "model-a"}, Provider: p}}, testFactory{})
	r := httptest.NewRequest("GET", "/v1/models", nil)
	r.Header.Set("Authorization", "Bearer binding-token")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	var out struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(out.Models) != 1 || out.Models[0]["slug"] != "model-a" {
		t.Fatalf("not a Codex catalog: %d %s", w.Code, w.Body.String())
	}
}

func TestCodexModelCatalogNeverInventsCapabilities(t *testing.T) {
	s := NewServer(model.DefaultLimits(), testResolver{Snapshot{Binding: model.Binding{ID: "b", Model: "model-a"}}}, testFactory{})
	r := httptest.NewRequest("GET", "/v1/models", nil)
	r.Header.Set("Authorization", "Bearer binding-token")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "MODEL_CATALOG_UNAVAILABLE") {
		t.Fatalf("missing catalog silently fabricated: %d %s", w.Code, w.Body.String())
	}
}
