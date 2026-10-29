// Package agentparser adapts the public parser without replacing it. Token
// accounting uses original cumulative counters, not summed turn_end events.
package agentparser

import (
	"encoding/json"
	parser "github.com/alexgorbatchev/agent-parser"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/usage"
	"time"
)

type Adapter struct{}

func New() usage.Parser { return &Adapter{} }
func (*Adapter) ParseLine(line []byte) ([]usage.Observation, error) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(line, &raw) != nil {
		return nil, fault.New("JSONL_INVALID", "Invalid JSONL record")
	}
	var typ string
	_ = json.Unmarshal(raw["type"], &typ)
	var payload map[string]json.RawMessage
	_ = json.Unmarshal(raw["payload"], &payload)
	var subtype string
	_ = json.Unmarshal(payload["type"], &subtype)
	var originalInfo json.RawMessage
	projected := line
	if typ == "event_msg" && subtype == "token_count" {
		originalInfo = append([]byte(nil), payload["info"]...)
		var info map[string]json.RawMessage
		if json.Unmarshal(originalInfo, &info) == nil {
			for _, k := range []string{"total_token_usage", "last_token_usage"} {
				var counters map[string]json.RawMessage
				if json.Unmarshal(info[k], &counters) != nil {
					continue
				}
				for native, alias := range map[string]string{"input_tokens": "prompt_tokens", "cached_input_tokens": "cached_prompt_tokens", "output_tokens": "completion_tokens", "reasoning_output_tokens": "reasoning_tokens"} {
					if v, ok := counters[native]; ok {
						counters[alias] = v
					}
				}
				info[k], _ = json.Marshal(counters)
			}
			payload["info"], _ = json.Marshal(info)
			raw["payload"], _ = json.Marshal(payload)
			projected, _ = json.Marshal(raw)
		}
	}
	parsed, err := parser.ParseCodexLine(projected)
	if err != nil {
		return nil, fault.Wrap("JSONL_PARSE_ERROR", "Agent parser rejected a record", err)
	}
	out := make([]usage.Observation, 0, len(parsed)+1)
	var stamp time.Time
	_ = json.Unmarshal(raw["timestamp"], &stamp)
	for _, event := range parsed {
		o := usage.Observation{Kind: event.EventType, Timestamp: time.UnixMilli(event.Timestamp).UTC(), Workspace: event.CWD}
		if event.Timestamp == 0 {
			o.Timestamp = stamp
		}
		if event.Data.Model != nil {
			o.Model = *event.Data.Model
		}
		out = append(out, o)
	}
	// The parser's normalized Input is uncached input in v1.0.1. Preserve the
	// upstream input/cache subset semantics for LAM's cumulative-counter index.
	if len(originalInfo) > 0 && string(originalInfo) != "null" {
		out = append(out, usage.Observation{Kind: "usage", Timestamp: stamp, Usage: originalInfo})
	}
	return out, nil
}
