package model

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

//go:embed assets/legacy-codex-model-catalog.json
var legacyCatalog []byte

// ModelContextWindow is a compatibility baseline, not a claim that bundled
// catalog data describes a future upstream model. Explicit metadata wins.
func ModelContextWindow(m ProviderModel) int64 {
	if m.ContextWindow > 0 {
		return m.ContextWindow
	}
	var c struct {
		Models []struct {
			Slug   string `json:"slug"`
			Window int64  `json:"context_window"`
		}
	}
	_ = json.Unmarshal(legacyCatalog, &c)
	for _, v := range c.Models {
		if v.Slug == m.ID && v.Window > 0 {
			return v.Window
		}
	}
	return 0
}
func CompactThreshold(window int64) int64 { return int64(math.Round(float64(window) * .9)) }
func ModelCatalogEntry(m ProviderModel, priority int) (json.RawMessage, error) {
	if m.ID == "" {
		return nil, fmt.Errorf("missing model ID")
	}
	if len(m.CodexCatalog) > 0 {
		var v map[string]json.RawMessage
		if json.Unmarshal(m.CodexCatalog, &v) != nil || v == nil {
			return nil, fmt.Errorf("invalid explicit model catalog")
		}
		var slug string
		if json.Unmarshal(v["slug"], &slug) != nil || slug != m.ID {
			return nil, fmt.Errorf("catalog slug mismatch")
		}
		for _, k := range []string{"display_name", "shell_type", "visibility", "base_instructions", "supported_reasoning_levels", "supported_in_api", "priority", "supports_reasoning_summaries", "support_verbosity", "truncation_policy", "supports_parallel_tool_calls", "experimental_supported_tools"} {
			if _, ok := v[k]; !ok {
				return nil, fmt.Errorf("catalog field missing: %s", k)
			}
		}
		return append(json.RawMessage(nil), m.CodexCatalog...), nil
	}
	label := m.Label
	if label == "" {
		label = m.ID
	}
	levels := []map[string]string{}
	for _, e := range []string{"low", "medium", "high", "xhigh", "max", "ultra"} {
		levels = append(levels, map[string]string{"effort": e, "description": "Codex reasoning effort: " + e})
	}
	// Exact conservative operational defaults used by LAM 0.4.4's catalog builder.
	// These configure Codex UI/compaction; they do not mark an upstream as tested.
	v := map[string]any{"slug": m.ID, "display_name": label, "supported_reasoning_levels": levels, "shell_type": "shell_command", "visibility": "list", "supported_in_api": true, "priority": priority, "base_instructions": "You are a coding agent. Follow the provided developer and user instructions, use available tools carefully, and make verifiable changes in the current workspace.", "supports_reasoning_summaries": false, "support_verbosity": false, "truncation_policy": map[string]any{"mode": "tokens", "limit": 10000}, "supports_parallel_tool_calls": false, "experimental_supported_tools": []string{}}
	if w := ModelContextWindow(m); w > 0 {
		v["context_window"] = w
		v["auto_compact_token_limit"] = CompactThreshold(w)
	}
	return json.Marshal(v)
}
func ModelsCatalog(models []ProviderModel) ([]byte, error) {
	sorted := append([]ProviderModel(nil), models...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	out := []json.RawMessage{}
	seen := map[string]bool{}
	for i, m := range sorted {
		if seen[m.ID] {
			return nil, fmt.Errorf("duplicate model ID")
		}
		seen[m.ID] = true
		b, e := ModelCatalogEntry(m, i+1)
		if e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return json.MarshalIndent(map[string]any{"models": out}, "", "  ")
}
