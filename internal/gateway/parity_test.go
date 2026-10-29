package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"lam.local/lam2/internal/model"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyCapturedRequestEnvelopesWithExplicitRedactedSchemaReplacement(t *testing.T) {
	paths, _ := filepath.Glob("../../tests/legacy-fixtures/codex-gateway-contract/normal/*request.json")
	if len(paths) != 5 {
		t.Fatalf("expected five real request fixtures, got %d", len(paths))
	}
	caps := model.Capabilities{Streaming: "supported", FunctionTools: "supported", Reasoning: "supported", StructuredOutputs: "supported", Images: "supported", CustomTools: "supported", Namespaces: "supported"}
	for _, p := range paths {
		t.Run(filepath.Base(p), func(t *testing.T) {
			b, e := os.ReadFile(p)
			if e != nil {
				t.Fatal(e)
			}
			var capture struct{ Body json.RawMessage }
			if e = json.Unmarshal(b, &capture); e != nil {
				t.Fatal(e)
			}
			// Captures intentionally replace a namespace's schema with {"redacted":...}.
			// The original Rust parser rejects this too. Assert rejection, then replace
			// only that test placeholder with a synthetic valid schema; production never
			// drops unknown namespace tools. This is envelope parity, not full capture replay.
			var body map[string]any
			_ = json.Unmarshal(capture.Body, &body)
			redacted := false
			for _, item := range body["tools"].([]any) {
				tool := item.(map[string]any)
				if tool["type"] == "namespace" {
					if placeholder, ok := tool["tools"].(map[string]any); ok && placeholder["redacted"] != nil {
						redacted = true
						tool["tools"] = []any{map[string]any{"type": "function", "name": "synthetic_fixture_tool", "parameters": map[string]any{"type": "object"}}}
					}
				}
			}
			if redacted {
				if e := Preflight(capture.Body, caps, model.DefaultLimits()); e == nil {
					t.Fatal("redacted schema must not be silently accepted")
				}
				capture.Body, _ = json.Marshal(body)
			}
			if e = Preflight(capture.Body, caps, model.DefaultLimits()); e != nil {
				t.Fatal(e)
			}
			bridge, normalized, e := PrepareCompatibility(capture.Body, model.DefaultLimits())
			if e != nil {
				t.Fatal(e)
			}
			if bridge == nil || !json.Valid(normalized) {
				t.Fatal("no normalized request")
			}
			if bytes.Contains(normalized, []byte(`"type":"namespace"`)) {
				t.Fatal("namespace not flattened")
			}
		})
	}
}
func TestCustomNamespaceToolHistoryAndResponse(t *testing.T) {
	raw := []byte(`{"model":"m","input":[{"type":"custom_tool_call","call_id":"c","name":"patch","namespace":"editor","input":"a\nb"},{"type":"custom_tool_call_output","call_id":"c","output":"ok"},{"type":"additional_tools","tools":[{"type":"function","name":"more","parameters":{"type":"object"}}]}],"tools":[{"type":"namespace","name":"editor","tools":[{"type":"custom","name":"patch","description":"edit"}]}],"tool_choice":{"type":"custom","name":"patch","namespace":"editor"},"client_metadata":{"thread":"not-a-prompt"}}`)
	b, n, e := PrepareCompatibility(raw, model.DefaultLimits())
	if e != nil {
		t.Fatal(e)
	}
	var req map[string]any
	json.Unmarshal(n, &req)
	tools := req["tools"].([]any)
	if len(tools) != 2 {
		t.Fatal(req)
	}
	items := req["input"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["name"] != "editor__patch" {
		t.Fatal(req)
	}
	out, e := b.RestoreResponse([]byte(`{"output":[{"type":"function_call","id":"i","call_id":"c2","name":"editor__patch","arguments":"{\"input\":\"x\\ny\"}"}]}`))
	if e != nil {
		t.Fatal(e)
	}
	var result struct{ Output []map[string]any }
	json.Unmarshal(out, &result)
	v := result.Output[0]
	if v["type"] != "custom_tool_call" || v["namespace"] != "editor" || v["name"] != "patch" || v["input"] != "x\ny" {
		t.Fatal(v)
	}
}
func TestCustomStreamingNeverLeaksWrapperArguments(t *testing.T) {
	b, _, e := PrepareCompatibility([]byte(`{"input":"x","tools":[{"type":"custom","name":"patch"}]}`), model.DefaultLimits())
	if e != nil {
		t.Fatal(e)
	}
	frames := []string{
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc","call_id":"c","name":"patch","arguments":""}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc","output_index":0,"delta":"{\"input\":\"hel"}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc","output_index":0,"delta":"lo\\nworld\"}"}`,
		`{"type":"response.function_call_arguments.done","item_id":"fc","output_index":0,"arguments":"{\"input\":\"hello\\nworld\"}"}`,
	}
	var events []map[string]any
	for _, f := range frames {
		out, e := b.RestoreEvent([]byte(f))
		if e != nil {
			t.Fatal(e)
		}
		for _, r := range out {
			var m map[string]any
			json.Unmarshal(r, &m)
			events = append(events, m)
		}
	}
	var text string
	done := 0
	for _, v := range events {
		switch v["type"] {
		case "response.function_call_arguments.delta":
			t.Fatal("wrapper leaked")
		case "response.custom_tool_call_input.delta":
			text += v["delta"].(string)
		case "response.custom_tool_call_input.done":
			done++
		}
	}
	if text != "hello\nworld" || done != 1 {
		t.Fatal(events)
	}
}
func TestToolNameCollisionAndOversizeAreRefused(t *testing.T) {
	raw := []byte(`{"input":"x","tools":[{"type":"function","name":"a__b"},{"type":"namespace","name":"a","tools":[{"type":"function","name":"b"}]}]}`)
	if _, _, e := PrepareCompatibility(raw, model.DefaultLimits()); e == nil {
		t.Fatal("collision accepted")
	}
	l := model.DefaultLimits()
	l.MaxTools = 1
	if _, _, e := PrepareCompatibility([]byte(`{"input":"x","tools":[{"type":"namespace","name":"a","tools":[{"type":"function","name":"b"},{"type":"function","name":"c"}]}]}`), l); e == nil {
		t.Fatal("nested tool limit bypassed")
	}
}
func TestHostedWebSearchAndAdditionalHistoryParity(t *testing.T) {
	raw := []byte(`{"input":[{"type":"web_search_call","action":{"query":"test"}},{"type":"compaction_trigger"}],"tools":[{"type":"web_search","search_context_size":"low"}]}`)
	b, n, e := PrepareCompatibility(raw, model.DefaultLimits())
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(n, []byte("web_search_call")) {
		t.Fatal("history lost")
	}
	out, e := b.PatchChat([]byte(`{"model":"m","messages":[]}`), "openai_chat_completions", nil)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(out, []byte(`"web_search_options"`)) {
		t.Fatal(string(out))
	}
	if _, e = b.PatchChat([]byte(`{"model":"m","messages":[]}`), "deepseek_chat_completions", nil); e == nil {
		t.Fatal("DeepSeek hosted web incorrectly enabled")
	}
}
func TestResponsesProviderPassthroughUsesResponsesRoute(t *testing.T) {
	raw := `{"model":"model-a","input":[{"type":"compaction","encrypted_content":"opaque"}],"tools":[{"type":"web_search"}],"stream":false}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("route = %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		if string(b) != raw {
			t.Errorf("request changed: %s", b)
		}
		if r.Header.Get("Session_id") != "thread-a" {
			t.Error("session header missing")
		}
		io.WriteString(w, `{"object":"response","status":"completed","output":[]}`)
	}))
	defer upstream.Close()
	sn := Snapshot{Binding: model.Binding{ID: "b", Model: "model-a"}, Provider: model.Provider{Protocol: "responses", BaseURL: upstream.URL + "/v1", Codex: model.CodexOptions{RouteViaGateway: true}, Models: []model.ProviderModel{{ID: "model-a"}}}, Headers: http.Header{}}
	s := NewServer(model.DefaultLimits(), testResolver{sn}, nil)
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer binding-token")
	req.Header.Set("Session_id", "thread-a")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
}
func TestModelsCatalogFallbackMatchesLegacy(t *testing.T) {
	sn := Snapshot{Binding: model.Binding{Model: "custom-a"}, Provider: model.Provider{Models: []model.ProviderModel{{ID: "custom-a", Label: "Custom", ContextWindow: 64000}}}}
	b, e := BoundModelCatalog(sn)
	if e != nil {
		t.Fatal(e)
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	if m["slug"] != "custom-a" || m["shell_type"] != "shell_command" || m["context_window"] != float64(64000) {
		t.Fatal(m)
	}
}
func TestReasoningHistoryIsBoundedAndIsolated(t *testing.T) {
	h := NewReasoningHistory(2, 1024)
	if e := h.Record("a", []string{"call"}, "secret-reasoning"); e != nil {
		t.Fatal(e)
	}
	if h.Lookup("b", "call") != "" {
		t.Fatal("cross-binding history")
	}
	if h.Lookup("a", "call") != "secret-reasoning" {
		t.Fatal("missing history")
	}
	if e := h.Record("c", []string{"call"}, strings.Repeat("x", 1025)); e == nil {
		t.Fatal("unbounded reasoning")
	}
	_ = context.Background()
}

func TestDeepSeekHistoryAndStreamReasoningCapture(t *testing.T) {
	collector := NewReasoningCollector(128, 1024)
	for _, chunk := range []string{
		`{"choices":[{"index":0,"delta":{"reasoning_content":"think "}}]}`,
		`{"choices":[{"index":0,"delta":{"reasoning_content":"carefully","tool_calls":[{"index":0,"id":"call-1"}]}}]}`,
	} {
		if e := collector.Observe([]byte(chunk)); e != nil {
			t.Fatal(e)
		}
	}
	h := NewReasoningHistory(2, 1024)
	if e := collector.Commit(h, "binding:thread"); e != nil {
		t.Fatal(e)
	}
	bridge, _, e := PrepareCompatibility([]byte(`{"input":"hello","reasoning":{"effort":"xhigh"}}`), model.DefaultLimits())
	if e != nil {
		t.Fatal(e)
	}
	b, e := bridge.PatchChat([]byte(`{"messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"f","arguments":"{}"}}]}],"tool_choice":"auto"}`), "deepseek_chat_completions", h.Snapshot("binding:thread"))
	if e != nil {
		t.Fatal(e)
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	if m["reasoning_effort"] != "max" || m["tool_choice"] != nil {
		t.Fatal(m)
	}
	msg := m["messages"].([]any)[0].(map[string]any)
	if msg["content"] != "" || msg["reasoning_content"] != "think carefully" {
		t.Fatal(msg)
	}
	if _, e = bridge.PatchChat([]byte(`{"messages":[{"role":"assistant","tool_calls":[{"id":"call-1"}]}]}`), "deepseek_chat_completions", h.Snapshot("other-binding")); e == nil {
		t.Fatal("reasoning crossed binding boundary")
	}
}
func TestRequestRejectsTrailingJSON(t *testing.T) {
	if _, _, e := PrepareCompatibility([]byte(`{"input":"x"}{"input":"hidden"}`), model.DefaultLimits()); e == nil {
		t.Fatal("trailing data ignored")
	}
}
