package gateway

import (
	"encoding/json"
	"lam.local/lam2/internal/fault"
	"sort"
	"strings"
)

// ReasoningCollector observes upstream data only to maintain the legacy
// DeepSeek tool-roundtrip contract. It never translates model responses.
type ReasoningCollector struct {
	text               strings.Builder
	calls              map[int]string
	maxCalls, maxBytes int
}

func NewReasoningCollector(calls, bytes int) *ReasoningCollector {
	return &ReasoningCollector{calls: map[int]string{}, maxCalls: calls, maxBytes: bytes}
}
func (c *ReasoningCollector) Observe(raw []byte) error {
	if strings.TrimSpace(string(raw)) == "[DONE]" {
		return nil
	}
	var payload struct {
		Choices []struct {
			Index   int
			Message json.RawMessage
			Delta   json.RawMessage
		}
	}
	if json.Unmarshal(raw, &payload) != nil {
		return fault.New("UPSTREAM_INVALID", "Malformed upstream JSON chunk")
	}
	for _, choice := range payload.Choices {
		if choice.Index != 0 {
			continue
		}
		message := choice.Delta
		if len(message) == 0 || string(message) == "null" {
			message = choice.Message
		}
		if len(message) == 0 {
			continue
		}
		var v struct {
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"`
			ToolCalls        []struct {
				Index int
				ID    string
			} `json:"tool_calls"`
		}
		if json.Unmarshal(message, &v) != nil {
			return fault.New("UPSTREAM_INVALID", "Invalid upstream choice")
		}
		text := v.ReasoningContent
		if text == "" {
			text = v.Reasoning
		}
		if c.text.Len()+len(text) > c.maxBytes {
			return fault.New("ADAPTER_REASONING_LIMIT_EXCEEDED", "Reasoning exceeds limit")
		}
		c.text.WriteString(text)
		for i, call := range v.ToolCalls {
			idx := call.Index
			if len(choice.Delta) == 0 {
				idx = i
			}
			if call.ID != "" {
				if len(call.ID) > 1024 {
					return fault.New("TOOL_ARGUMENTS_LIMIT", "Tool ID exceeds limit")
				}
				c.calls[idx] = call.ID
			}
			if len(c.calls) > c.maxCalls {
				return fault.New("TOOLS_LIMIT", "Too many upstream tool calls")
			}
		}
	}
	return nil
}
func (c *ReasoningCollector) Commit(h *ReasoningHistory, scope string) error {
	ids := make([]string, 0, len(c.calls))
	for _, id := range c.calls {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return h.Record(scope, ids, c.text.String())
}
