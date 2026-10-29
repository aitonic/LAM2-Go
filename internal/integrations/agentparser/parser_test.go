package agentparser

import (
	"lam.local/lam2/internal/usage"
	"testing"
)

func TestNativeAndAliasCounters(t *testing.T) {
	for _, line := range []string{
		`{"timestamp":"2026-10-20T10:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":20,"reasoning_output_tokens":5}}}}`,
		`{"timestamp":"2026-10-20T10:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"prompt_tokens":100,"cached_prompt_tokens":40,"completion_tokens":20,"reasoning_tokens":5}}}}`,
	} {
		p := New()
		o, e := p.ParseLine([]byte(line))
		if e != nil {
			t.Fatal(e)
		}
		var state usage.CounterState
		found := false
		for _, v := range o {
			if len(v.Usage) == 0 {
				continue
			}
			d, ok, e := usage.Delta(v.Usage, &state)
			if e != nil || !ok || d.Input != 100 || d.CachedInput != 40 || d.Total() != 120 {
				t.Fatalf("%+v %v", d, e)
			}
			found = true
		}
		if !found {
			t.Fatal("no usage observation")
		}
	}
}
func TestMalformedJSON(t *testing.T) {
	if _, e := New().ParseLine([]byte(`{`)); e == nil {
		t.Fatal("accepted malformed input")
	}
}
