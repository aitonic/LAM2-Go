package gateway

// This is the LAM compatibility shim around the external protocol translator,
// not a second general Responses/Chat converter. It bridges the Codex-specific
// tool/history shapes observed in LAM 0.4.4 and restores their original identity.
import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"sort"
	"strings"
)

type ToolIdentity struct{ Name, Namespace, Kind string }
type ToolBridge struct {
	Tools   map[string]ToolIdentity
	items   map[string]ToolIdentity
	buffers map[string]string
	emitted map[string]bool
	web     map[string]any
	source  map[string]any
	limit   int
}

func object(b []byte) (map[string]any, error) {
	var v map[string]any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if e := d.Decode(&v); e != nil || v == nil {
		return nil, fault.New("REQUEST_INVALID", "Expected a JSON object")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, fault.New("REQUEST_INVALID", "Trailing JSON data is not allowed")
	}
	return v, nil
}
func str(v any) string { s, _ := v.(string); return s }
func rawText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}
func copyMap(v map[string]any) map[string]any {
	out := make(map[string]any, len(v))
	for k, x := range v {
		out[k] = x
	}
	return out
}
func FlattenToolName(namespace, name string) string {
	if namespace == "" {
		return name
	}
	sanitize := func(s string) string {
		var b strings.Builder
		for i := 0; i < len(s); i++ {
			c := s[i]
			if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' {
				b.WriteByte(c)
			} else {
				b.WriteByte('_')
			}
		}
		return b.String()
	}
	full := sanitize(namespace) + "__" + sanitize(name)
	if len(full) <= 64 {
		return full
	}
	hash := sha256.Sum256([]byte(full))
	return full[:46] + "__" + hex.EncodeToString(hash[:8])
}
func validToolName(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
func (b *ToolBridge) register(id ToolIdentity) (string, error) {
	name := FlattenToolName(id.Namespace, id.Name)
	if !validToolName(name) {
		return "", fault.New("TOOL_NAME_INVALID", "Invalid function name")
	}
	if old, ok := b.Tools[name]; ok && old != id {
		return "", fault.New("TOOL_NAME_COLLISION", "Two tool identities map to the same upstream name")
	}
	b.Tools[name] = id
	return name, nil
}
func toolArray(v any) ([]any, bool) {
	if a, ok := v.([]any); ok {
		return a, true
	}
	if m, ok := v.(map[string]any); ok {
		for _, k := range []string{"tools", "children"} {
			if a, ok := m[k].([]any); ok {
				return a, true
			}
		}
	}
	return nil, false
}
func PrepareCompatibility(raw []byte, l model.GatewayLimits) (*ToolBridge, []byte, error) {
	src, e := object(raw)
	if e != nil {
		return nil, nil, e
	}
	b := &ToolBridge{Tools: map[string]ToolIdentity{}, items: map[string]ToolIdentity{}, buffers: map[string]string{}, emitted: map[string]bool{}, source: src, limit: l.ToolArgumentBytes}
	m := copyMap(src)
	// client_metadata belongs to Codex transport, not model messages. The old
	// gateway intentionally consumed it; it must not cause request rejection.
	delete(m, "client_metadata")
	all := []any{}
	if v, ok := src["tools"]; ok && v != nil {
		a, yes := toolArray(v)
		if !yes {
			return nil, nil, loss("malformed tools")
		}
		all = append(all, a...)
	}
	if v, ok := src["additional_tools"]; ok && v != nil {
		a, yes := toolArray(v)
		if !yes {
			return nil, nil, loss("malformed additional_tools")
		}
		all = append(all, a...)
		delete(m, "additional_tools")
	}
	if items, ok := src["input"].([]any); ok {
		for _, it := range items {
			if v, ok := it.(map[string]any); ok && str(v["type"]) == "additional_tools" {
				a, yes := toolArray(v["tools"])
				if !yes {
					return nil, nil, loss("malformed additional_tools")
				}
				all = append(all, a...)
			}
		}
	}
	outTools := []any{}
	seen := map[string]bool{}
	var flatten func([]any, string, int) error
	flatten = func(tools []any, ns string, depth int) error {
		if depth > 16 {
			return fault.New("TOOLS_LIMIT", "Tool namespace nesting is too deep")
		}
		for _, v := range tools {
			t, ok := v.(map[string]any)
			if !ok {
				return loss("invalid tool definition")
			}
			kind := str(t["type"])
			if kind == "namespace" {
				name := str(t["name"])
				if name == "" {
					return loss("empty namespace")
				}
				if ns != "" {
					name = ns + "." + name
				}
				children, ok := toolArray(t["tools"])
				if !ok {
					return loss("invalid namespace children")
				}
				if e := flatten(children, name, depth+1); e != nil {
					return e
				}
				continue
			}
			if kind == "web_search" || kind == "web_search_preview" {
				if b.web != nil {
					return loss("multiple hosted web searches")
				}
				b.web = map[string]any{}
				for _, k := range []string{"indexed_web_access", "filters", "search_content_types"} {
					if x, ok := t[k]; ok && x != nil {
						return loss("web search option " + k)
					}
				}
				for _, k := range []string{"search_context_size", "user_location"} {
					if v, ok := t[k]; ok {
						b.web[k] = v
					}
				}
				continue
			}
			f := t
			if nested, ok := t["function"].(map[string]any); ok {
				f = nested
			}
			name := str(f["name"])
			if kind == "tool_search" {
				name = "tool_search"
			}
			if kind == "local_shell" {
				name = "local_shell"
			}
			if kind != "function" && kind != "custom" && kind != "tool_search" && kind != "local_shell" {
				return loss("tool type " + kind)
			}
			mapped, e := b.register(ToolIdentity{name, ns, kind})
			if e != nil {
				return e
			}
			if seen[mapped] {
				continue
			}
			seen[mapped] = true
			params := f["parameters"]
			if params == nil {
				params = map[string]any{"type": "object"}
			}
			def := map[string]any{"type": "function", "name": mapped, "parameters": params}
			if d, ok := f["description"]; ok {
				def["description"] = d
			}
			if strict, ok := f["strict"]; ok {
				def["strict"] = strict
			}
			if kind == "custom" {
				def["parameters"] = map[string]any{"type": "object", "properties": map[string]any{"input": map[string]any{"type": "string"}}, "required": []string{"input"}}
				def["strict"] = false
			}
			outTools = append(outTools, def)
			if len(outTools) > l.MaxTools {
				return fault.New("TOOLS_LIMIT", "Flattened tools exceed the configured limit")
			}
		}
		return nil
	}
	if e = flatten(all, "", 0); e != nil {
		return nil, nil, e
	}
	if len(all) > 0 {
		m["tools"] = outTools
	}
	if choice, ok := m["tool_choice"].(map[string]any); ok {
		kind := str(choice["type"])
		if kind == "function" || kind == "custom" {
			m["tool_choice"] = map[string]any{"type": "function", "name": FlattenToolName(str(choice["namespace"]), str(choice["name"]))}
		}
	}
	if input, ok := src["input"].([]any); ok {
		result := []any{}
		calls := map[string]bool{}
		for _, v := range input {
			item, ok := v.(map[string]any)
			if !ok {
				return nil, nil, loss("non-object input item")
			}
			it := copyMap(item)
			kind := str(it["type"])
			switch kind {
			case "additional_tools":
				continue
			case "function_call", "custom_tool_call", "tool_search_call", "local_shell_call":
				callID := str(it["call_id"])
				if callID == "" || calls[callID] {
					return nil, nil, loss("missing or duplicate tool call ID")
				}
				calls[callID] = true
				name, ns, k := str(it["name"]), str(it["namespace"]), "function"
				switch kind {
				case "custom_tool_call":
					k = "custom"
					it["arguments"] = rawText(map[string]any{"input": it["input"]})
					delete(it, "input")
				case "tool_search_call":
					k = "tool_search"
					name = "tool_search"
					it["arguments"] = rawText(it["arguments"])
				case "local_shell_call":
					k = "local_shell"
					name = "local_shell"
					it["arguments"] = rawText(it["action"])
					delete(it, "action")
				}
				mapped, e := b.register(ToolIdentity{name, ns, k})
				if e != nil {
					return nil, nil, e
				}
				args := str(it["arguments"])
				if len(args) > l.ToolArgumentBytes {
					return nil, nil, fault.New("TOOL_ARGUMENTS_LIMIT", "Tool arguments exceed maximum")
				}
				if !json.Valid([]byte(args)) {
					return nil, nil, loss("invalid tool call JSON")
				}
				it["type"] = "function_call"
				it["name"] = mapped
				delete(it, "namespace")
			case "function_call_output", "custom_tool_call_output", "mcp_tool_call_output", "tool_search_output":
				id := str(it["call_id"])
				if !calls[id] {
					return nil, nil, loss("tool result without an earlier call")
				}
				if kind == "tool_search_output" {
					it["output"] = rawText(it["tools"])
					delete(it, "tools")
				}
				if _, ok := it["output"].(string); !ok {
					it["output"] = toolOutput(it["output"])
				}
				it["type"] = "function_call_output"
			case "web_search_call", "agent_message", "image_generation_call", "compaction", "context_compaction", "compaction_trigger":
				content := historyText(it)
				it = map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": content}}}
			case "message", "", "reasoning":
			default:
				return nil, nil, loss("input item " + kind)
			}
			result = append(result, it)
		}
		m["input"] = result
	}
	normalized, e := json.Marshal(m)
	return b, normalized, e
}
func toolOutput(v any) string {
	if a, ok := v.([]any); ok {
		var parts []string
		for _, p := range a {
			if m, ok := p.(map[string]any); ok {
				if text := str(m["text"]); text != "" {
					parts = append(parts, text)
					continue
				}
			}
			parts = append(parts, rawText(p))
		}
		return strings.Join(parts, "\n")
	}
	return rawText(v)
}
func historyText(m map[string]any) string {
	k := str(m["type"])
	switch k {
	case "web_search_call":
		return "[web_search_call] " + rawText(m["action"])
	case "agent_message":
		return "[agent_message " + str(m["author"]) + " -> " + str(m["recipient"]) + "] " + toolOutput(m["content"])
	case "compaction", "context_compaction":
		return "[" + k + "] " + str(m["encrypted_content"])
	case "compaction_trigger":
		return "[compaction_trigger]"
	default:
		return "[" + k + "] " + rawText(m)
	}
}

// PatchChat supplies only fields/semantics omitted by a generic SDK conversion.
func (b *ToolBridge) PatchChat(raw []byte, compatibility string, history map[string]string) ([]byte, error) {
	m, e := object(raw)
	if e != nil {
		return nil, e
	}
	if b.web != nil {
		if strings.Contains(compatibility, "deepseek") {
			return nil, loss("hosted search in DeepSeek")
		}
		m["web_search_options"] = b.web
	}
	// Preserve common options exactly when a converter omits them.
	for _, key := range []string{"temperature", "top_p", "parallel_tool_calls", "user", "service_tier", "metadata", "store", "prompt_cache_key", "prompt_cache_retention", "safety_identifier"} {
		if v, ok := b.source[key]; ok {
			m[key] = v
		}
	}
	if v, ok := b.source["max_output_tokens"]; ok {
		m["max_completion_tokens"] = v
	}
	if text, ok := b.source["text"].(map[string]any); ok {
		if f, ok := text["format"].(map[string]any); ok {
			switch str(f["type"]) {
			case "json_schema":
				inner := copyMap(f)
				delete(inner, "type")
				m["response_format"] = map[string]any{"type": "json_schema", "json_schema": inner}
			case "json_object":
				m["response_format"] = map[string]any{"type": "json_object"}
			}
		}
	}
	if r, ok := b.source["reasoning"].(map[string]any); ok {
		if effort, ok := r["effort"]; ok {
			m["reasoning_effort"] = effort
		}
	}
	deepseek := strings.Contains(compatibility, "deepseek")
	thinking := deepseek && str(m["reasoning_effort"]) != "none"
	if deepseek {
		mode := "disabled"
		if thinking {
			mode = "enabled"
			effort := "high"
			switch str(m["reasoning_effort"]) {
			case "xhigh", "max", "ultra":
				effort = "max"
			}
			m["reasoning_effort"] = effort
		} else {
			delete(m, "reasoning_effort")
		}
		m["thinking"] = map[string]any{"type": mode}
		if thinking {
			if choice, ok := m["tool_choice"]; ok {
				if str(choice) == "auto" {
					delete(m, "tool_choice")
				} else if str(choice) != "none" {
					return nil, fault.New("ADAPTER_UNSUPPORTED_TOOL_CHOICE", "Explicit tool choice is unsupported in DeepSeek thinking mode")
				}
			}
		}
	}
	messages, _ := m["messages"].([]any)
	system := []any{}
	rest := []any{}
	for _, v := range messages {
		msg, ok := v.(map[string]any)
		if !ok {
			return nil, loss("invalid translated message")
		}
		if deepseek && str(msg["role"]) == "developer" {
			msg["role"] = "system"
		}
		if str(msg["role"]) == "assistant" {
			if calls, ok := msg["tool_calls"].([]any); ok && len(calls) > 0 {
				if deepseek && msg["content"] == nil {
					msg["content"] = ""
				}
				if str(msg["reasoning_content"]) == "" {
					var parts []string
					seen := map[string]bool{}
					for _, c := range calls {
						call, ok := c.(map[string]any)
						if !ok {
							continue
						}
						r := history[str(call["id"])]
						if r != "" && !seen[r] {
							parts = append(parts, r)
							seen[r] = true
						}
					}
					if len(parts) > 0 {
						msg["reasoning_content"] = strings.Join(parts, "\n")
					}
				}
				if thinking && str(msg["reasoning_content"]) == "" {
					return nil, fault.New("ADAPTER_REASONING_UNAVAILABLE", "Tool history requires the original reasoning content")
				}
			}
		}
		if str(msg["role"]) == "system" {
			system = append(system, msg)
		} else {
			rest = append(rest, msg)
		}
	}
	m["messages"] = append(system, rest...)
	return json.Marshal(m)
}
func (b *ToolBridge) restoreItem(item map[string]any) error {
	if str(item["type"]) != "function_call" {
		return nil
	}
	identity, ok := b.Tools[str(item["name"])]
	if !ok {
		return nil
	}
	if id := str(item["id"]); id != "" {
		b.items[id] = identity
	}
	item["name"] = identity.Name
	if identity.Namespace != "" {
		item["namespace"] = identity.Namespace
	}
	args := str(item["arguments"])
	switch identity.Kind {
	case "custom":
		text := ""
		if args != "" {
			var v struct {
				Input *string `json:"input"`
			}
			if json.Unmarshal([]byte(args), &v) != nil || v.Input == nil {
				return loss("custom tool input wrapper")
			}
			text = *v.Input
		}
		item["type"] = "custom_tool_call"
		item["input"] = text
		delete(item, "arguments")
	case "tool_search", "local_shell":
		value := any(map[string]any{})
		if args != "" {
			d := json.NewDecoder(strings.NewReader(args))
			d.UseNumber()
			if d.Decode(&value) != nil {
				return loss("tool arguments")
			}
		}
		if identity.Kind == "tool_search" {
			item["type"] = "tool_search_call"
			item["arguments"] = value
			item["execution"] = "client"
		} else {
			item["type"] = "local_shell_call"
			item["action"] = value
			delete(item, "arguments")
		}
	}
	return nil
}
func (b *ToolBridge) restoreOutput(m map[string]any) error {
	if output, ok := m["output"].([]any); ok {
		for _, v := range output {
			if it, ok := v.(map[string]any); ok {
				if e := b.restoreItem(it); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
func (b *ToolBridge) RestoreResponse(raw []byte) ([]byte, error) {
	m, e := object(raw)
	if e != nil {
		return nil, e
	}
	if e = b.restoreOutput(m); e != nil {
		return nil, e
	}
	return json.Marshal(m)
}
func (b *ToolBridge) RestoreEvent(raw []byte) ([][]byte, error) {
	m, e := object(raw)
	if e != nil {
		return nil, e
	}
	kind := str(m["type"])
	if item, ok := m["item"].(map[string]any); ok {
		if e = b.restoreItem(item); e != nil {
			return nil, e
		}
	}
	if response, ok := m["response"].(map[string]any); ok {
		if e = b.restoreOutput(response); e != nil {
			return nil, e
		}
	}
	id := str(m["item_id"])
	identity, known := b.items[id]
	if known && identity.Kind == "custom" && (kind == "response.function_call_arguments.delta" || kind == "response.function_call_arguments.done") {
		if kind == "response.function_call_arguments.delta" {
			b.buffers[id] += str(m["delta"])
			if len(b.buffers[id]) > b.limit {
				return nil, fault.New("TOOL_ARGUMENTS_LIMIT", "Custom tool arguments exceed maximum")
			}
			return nil, nil
		}
		args := str(m["arguments"])
		if args == "" {
			args = b.buffers[id]
		}
		if len(args) > b.limit {
			return nil, fault.New("TOOL_ARGUMENTS_LIMIT", "Custom tool arguments exceed maximum")
		}
		if b.emitted[id] {
			return nil, nil
		}
		var wrapper struct {
			Input *string `json:"input"`
		}
		if json.Unmarshal([]byte(args), &wrapper) != nil || wrapper.Input == nil {
			return nil, loss("custom tool input wrapper")
		}
		b.emitted[id] = true
		delete(b.buffers, id)
		delta := copyMap(m)
		delta["type"] = "response.custom_tool_call_input.delta"
		delta["delta"] = *wrapper.Input
		delete(delta, "arguments")
		m["type"] = "response.custom_tool_call_input.done"
		m["input"] = *wrapper.Input
		delete(m, "arguments")
		a, _ := json.Marshal(delta)
		z, _ := json.Marshal(m)
		return [][]byte{a, z}, nil
	}
	// Tool search/local-shell output items carry their native structure. Arguments
	// deltas remain function-shaped only for ordinary/namespaced function tools.
	if known && (identity.Kind == "tool_search" || identity.Kind == "local_shell") && strings.HasPrefix(kind, "response.function_call_arguments.") {
		return nil, nil
	}
	out, e := json.Marshal(m)
	return [][]byte{out}, e
}
func (b *ToolBridge) ToolNames() []string {
	names := make([]string, 0, len(b.Tools))
	for n := range b.Tools {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
func SSEEvent(raw []byte) ([]byte, error) {
	v, e := object(raw)
	if e != nil {
		return nil, e
	}
	kind := str(v["type"])
	if kind == "" || strings.ContainsAny(kind, "\r\n") {
		return nil, fault.New("TRANSLATION_INVALID", "Invalid event name")
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", kind, raw)), nil
}
