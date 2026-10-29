package usage

import "encoding/json"

// parserDiagnosticCategory preserves the original Usage diagnostics vocabulary.
// The agent-parser SDK remains authoritative for event extraction. This tiny
// classification pass only records why a raw Codex envelope cannot be used;
// it is not a second Codex event parser or a replacement for the SDK.
func parserDiagnosticCategory(line []byte) (category string, skipped bool) {
	if !json.Valid(line) {
		return "invalid_json", false
	}
	var v struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(line, &v); err != nil {
		return "invalid_json", false
	}
	if len(v.Payload) == 0 || string(v.Payload) == "null" {
		return "missing_payload", false
	}
	if v.Type != "event_msg" {
		return "", false
	}
	var p struct {
		Type string          `json:"type"`
		Info json.RawMessage `json:"info"`
	}
	if json.Unmarshal(v.Payload, &p) != nil {
		return "missing_payload", false
	}
	if p.Type != "token_count" {
		// Preserve the original's known non-token messages without treating
		// ordinary agent events as malformed usage entries.
		switch p.Type {
		case "agent_message", "user_message", "agent_reasoning", "token_count", "context_compacted", "turn_aborted", "task_started", "task_complete", "task_failed", "web_search_call", "file_change", "exec_command_begin", "exec_command_end":
			return "", false
		}
		return "unknown_event_shape", false
	}
	if len(p.Info) == 0 || string(p.Info) == "null" {
		return "missing_info", true
	}
	var info struct {
		Total json.RawMessage `json:"total_token_usage"`
		Last  json.RawMessage `json:"last_token_usage"`
	}
	if json.Unmarshal(p.Info, &info) != nil {
		return "missing_info", true
	}
	if len(info.Total) == 0 || string(info.Total) == "null" {
		return "missing_total_token_usage", false
	}
	if len(info.Last) == 0 || string(info.Last) == "null" {
		return "missing_last_token_usage", false
	}
	return "", false
}
