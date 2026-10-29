package handoff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

type CompatibilityIssue struct {
	ItemID string `json:"itemId"`
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}
type CompatibilityReport struct {
	Disposition string               `json:"disposition"`
	Issues      []CompatibilityIssue `json:"issues"`
}

// AnalyzeCompatibility matches the original cross-provider relay boundary.
// It retains item identities and reasons, never prompts, arguments or tool output.
func AnalyzeCompatibility(raw []byte, functionTools bool) CompatibilityReport {
	out := CompatibilityReport{Disposition: "compatible", Issues: []CompatibilityIssue{}}
	add := func(id, kind, reason string) {
		out.Issues = append(out.Issues, CompatibilityIssue{id, kind, reason})
		out.Disposition = "blocked"
	}
	if !utf8.Valid(raw) {
		add("document", "corrupt", "Invalid UTF-8")
		return out
	}
	calls, results := map[string][]string{}, map[string][]string{}
	for n, line := range bytes.Split(raw, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		id := fmt.Sprintf("line-%d", n+1)
		var root map[string]any
		if json.Unmarshal(line, &root) != nil || root == nil {
			add(id, "corrupt", "Invalid JSONL")
			continue
		}
		v, ok := root["payload"].(map[string]any)
		if !ok {
			v = root
		}
		for _, key := range []string{"id", "call_id"} {
			if x, ok := v[key].(string); ok && x != "" && len(x) <= 256 {
				id = x
				break
			}
		}
		if containsKey(root, "previous_response_id", 0) {
			add(id, "previous_response_state", "State belongs to the original response store")
			continue
		}
		typ, _ := root["type"].(string)
		if typ == "response_item" {
			typ, _ = v["type"].(string)
		}
		switch typ {
		case "session_meta", "turn_context", "token_count", "task_started", "task_complete", "event_msg":
		case "message", "response":
			content, _ := v["content"].([]any)
			for _, item := range content {
				p, _ := item.(map[string]any)
				kind, _ := p["type"].(string)
				switch kind {
				case "input_image", "output_image", "image", "input_audio", "output_audio", "audio", "input_file", "file":
					add(id, kind, "No verified cross-provider history mapping")
				}
			}
		case "function_call", "function_call_output":
			callID, _ := v["call_id"].(string)
			if callID == "" {
				callID, _ = v["callId"].(string)
			}
			if callID == "" || len(callID) > 256 {
				callID = fmt.Sprintf("missing-call-id-%d", n+1)
			}
			if !functionTools {
				add(id, typ, "Target has no verified function-tool mapping")
			}
			if typ == "function_call" {
				calls[callID] = append(calls[callID], id)
			} else {
				results[callID] = append(results[callID], id)
			}
		case "reasoning":
			if containsKey(v, "encrypted_content", 0) || containsKey(v, "encryptedContent", 0) {
				add(id, "encrypted_reasoning", "Encrypted state is bound to its original runtime")
			}
		case "":
			if root["session_id"] == nil && root["cwd"] == nil {
				add(id, "unknown", "Unknown session item")
			}
		default:
			add(id, typ, "Protocol state has no verified portable mapping")
		}
	}
	for id, c := range calls {
		if len(c) != 1 || len(results[id]) != 1 {
			for _, item := range append(c, results[id]...) {
				add(item, "function_state", "Incomplete or ambiguous tool call/result pair")
			}
		}
	}
	for id, r := range results {
		if len(calls[id]) == 0 {
			for _, item := range r {
				add(item, "function_result", "No matching call")
			}
		}
	}
	return out
}
func containsKey(v any, key string, depth int) bool {
	if depth > 64 {
		return true
	}
	switch x := v.(type) {
	case map[string]any:
		for k, n := range x {
			if k == key && n != nil {
				return true
			}
			if containsKey(n, key, depth+1) {
				return true
			}
		}
	case []any:
		for _, n := range x {
			if containsKey(n, key, depth+1) {
				return true
			}
		}
	}
	return false
}
