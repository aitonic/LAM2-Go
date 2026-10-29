package gateway

import (
	"encoding/json"
	"fmt"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"strings"
)

// Preflight refuses unsupported semantics instead of allowing a translator to drop them.
func Preflight(raw []byte, caps model.Capabilities, l model.GatewayLimits) error {
	original := raw
	_, normalized, normalizeErr := PrepareCompatibility(raw, l)
	if normalizeErr != nil {
		return normalizeErr
	}
	raw = normalized
	_ = original
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil || m == nil {
		return fault.New("REQUEST_INVALID", "Expected a JSON object")
	}
	allowed := map[string]bool{}
	for _, k := range strings.Fields("model input instructions stream tools tool_choice parallel_tool_calls max_output_tokens temperature top_p text reasoning metadata store truncation user service_tier include prompt_cache_key prompt_cache_retention safety_identifier") {
		allowed[k] = true
	}
	for k := range m {
		if !allowed[k] {
			return fault.New("PROTOCOL_LOSS", "Unsupported Responses field: "+k)
		}
	}
	var input any
	if json.Unmarshal(m["input"], &input) != nil {
		return fault.New("REQUEST_INVALID", "Input is required")
	}
	switch input.(type) {
	case string, []any:
	default:
		return fault.New("REQUEST_INVALID", "Input must be a string or item array")
	}
	var stream bool
	if v, ok := m["stream"]; ok {
		if json.Unmarshal(v, &stream) != nil {
			return fault.New("REQUEST_INVALID", "Stream must be a boolean")
		}
	}
	var originalMap map[string]any
	_ = json.Unmarshal(original, &originalMap)
	if ts, ok := originalMap["tools"].([]any); ok {
		for _, v := range ts {
			if t, ok := v.(map[string]any); ok && (t["type"] == "web_search" || t["type"] == "web_search_preview") && !model.Supported(caps.FunctionTools) {
				return loss("hosted search")
			}
		}
	}
	if stream && !model.Supported(caps.Streaming) {
		return loss("streaming")
	}
	if v, ok := m["store"]; ok && string(v) != "false" && string(v) != "null" {
		return loss("server-side response storage")
	}
	if len(m["reasoning"]) > 0 && string(m["reasoning"]) != "null" && !model.Supported(caps.Reasoning) {
		return loss("reasoning")
	}
	if len(m["text"]) > 0 && string(m["text"]) != "null" {
		var cfg struct {
			Format struct {
				Type string `json:"type"`
			} `json:"format"`
		}
		if json.Unmarshal(m["text"], &cfg) != nil {
			return loss("invalid text format")
		}
		if cfg.Format.Type != "" && cfg.Format.Type != "text" && !model.Supported(caps.StructuredOutputs) {
			return loss("structured output")
		}
	}
	var tools []map[string]json.RawMessage
	if v, ok := m["tools"]; ok {
		if json.Unmarshal(v, &tools) != nil {
			return fault.New("REQUEST_INVALID", "Tools must be an array")
		}
	}
	if len(tools) > l.MaxTools {
		return fault.New("TOOLS_LIMIT", "Tool count exceeds maximum")
	}
	for _, t := range tools {
		var kind string
		json.Unmarshal(t["type"], &kind)
		switch kind {
		case "function":
			if !model.Supported(caps.FunctionTools) {
				return loss("function tools")
			}
		case "namespace":
			return loss("namespace tools require a verified compatibility adapter")
		case "custom":
			return loss("custom tools require a verified compatibility adapter")
		default:
			return loss("tool type " + kind)
		}
	}
	if arr, ok := input.([]any); ok {
		for _, v := range arr {
			item, ok := v.(map[string]any)
			if !ok {
				return fault.New("REQUEST_INVALID", "Input item must be an object")
			}
			kind, _ := item["type"].(string)
			switch kind {
			case "", "message":
			case "function_call", "function_call_output":
				if !model.Supported(caps.FunctionTools) {
					return loss("tool history")
				}
			case "reasoning":
				if !model.Supported(caps.Reasoning) {
					return loss("reasoning history")
				}
			default:
				return loss("input item " + kind)
			}
			if args, ok := item["arguments"].(string); ok {
				if len(args) > l.ToolArgumentBytes {
					return fault.New("TOOL_ARGUMENTS_LIMIT", "Tool arguments exceed maximum")
				}
				if !json.Valid([]byte(args)) {
					return fault.New("REQUEST_INVALID", "Tool arguments must contain JSON")
				}
			}
			if content, ok := item["content"].([]any); ok {
				for _, part := range content {
					p, ok := part.(map[string]any)
					if !ok {
						return fault.New("REQUEST_INVALID", "Content item must be an object")
					}
					typ, _ := p["type"].(string)
					switch typ {
					case "input_text", "output_text", "text":
					case "input_image":
						if !model.Supported(caps.Images) {
							return loss("image input")
						}
					default:
						return loss("content type " + typ)
					}
				}
			}
		}
	}
	return nil
}
func loss(feature string) error {
	return fault.New("PROTOCOL_LOSS", fmt.Sprintf("Cannot preserve %s for this provider", feature))
}
