package usage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"strings"
	"time"
)

type CounterState struct {
	Previous        model.Tokens `json:"previous"`
	HasPrevious     bool         `json:"hasPrevious"`
	LastFingerprint string       `json:"lastFingerprint"`
	Resets          int          `json:"resets"`
}

func readTokens(raw json.RawMessage) (model.Tokens, bool, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return model.Tokens{}, false, nil
	}
	var m map[string]json.RawMessage
	if e := json.Unmarshal(raw, &m); e != nil {
		return model.Tokens{}, false, e
	}
	read := func(keys ...string) (int64, error) {
		for _, k := range keys {
			if v, ok := m[k]; ok {
				var n int64
				if e := json.Unmarshal(v, &n); e != nil {
					return 0, e
				}
				return n, nil
			}
		}
		return 0, nil
	}
	var t model.Tokens
	var e error
	t.Input, e = read("input_tokens", "prompt_tokens")
	if e != nil {
		return t, false, e
	}
	t.CachedInput, e = read("cached_input_tokens", "cached_prompt_tokens")
	if e != nil {
		return t, false, e
	}
	t.Output, e = read("output_tokens", "completion_tokens")
	if e != nil {
		return t, false, e
	}
	t.ReasoningOutput, e = read("reasoning_output_tokens", "reasoning_tokens")
	if e != nil {
		return t, false, e
	}
	return t, true, t.Validate()
}

// Delta interprets cumulative Codex counters once. Cache tokens are a subset of input.
func Delta(info []byte, state *CounterState) (model.Tokens, bool, error) {
	var v map[string]json.RawMessage
	if json.Unmarshal(info, &v) != nil {
		return model.Tokens{}, false, fault.New("USAGE_INVALID", "Malformed usage counters")
	}
	total, hasTotal, e := readTokens(v["total_token_usage"])
	if e != nil {
		return total, false, fault.Wrap("USAGE_INVALID", "Invalid cumulative counters", e)
	}
	last, hasLast, e := readTokens(v["last_token_usage"])
	if e != nil {
		return last, false, fault.Wrap("USAGE_INVALID", "Invalid last-turn counters", e)
	}
	if !hasTotal && !hasLast {
		return model.Tokens{}, false, nil
	}
	fingerprint := storage.Hash(info)
	if fingerprint == state.LastFingerprint {
		return model.Tokens{}, false, nil
	}
	var delta model.Tokens
	if hasTotal {
		if !state.HasPrevious {
			delta = total
		} else {
			prev := state.Previous
			if total.Input < prev.Input || total.Output < prev.Output || total.CachedInput < prev.CachedInput || total.ReasoningOutput < prev.ReasoningOutput {
				state.Resets++
				delta = total
				if hasLast {
					delta = last
				}
			} else {
				delta = model.Tokens{Input: total.Input - prev.Input, CachedInput: total.CachedInput - prev.CachedInput, Output: total.Output - prev.Output, ReasoningOutput: total.ReasoningOutput - prev.ReasoningOutput}
			}
		}
		// Original LAM uses last_token_usage for a call, not the entire cumulative
		// session total. Cumulative subtraction is only a compatibility fallback.
		if hasLast {
			delta = last
		}
		if e = delta.Validate(); e != nil {
			return delta, false, fault.Wrap("USAGE_INVALID", "Counter delta is inconsistent", e)
		}
		state.Previous = total
		state.HasPrevious = true
	} else {
		delta = last
	}
	state.LastFingerprint = fingerprint
	return delta, delta.Total() > 0, nil
}
func Price(e *model.UsageEvent, cards []model.RateCard) {
	originalCost, originalVersion, originalConfidence := e.Cost, e.PriceVersion, e.PricingConfidence
	e.Cost = nil
	e.PriceVersion = ""
	e.PricingConfidence = "unknown"
	var selected *model.RateCard
	for i := range cards {
		p := &cards[i]
		if p.Model != e.Model || p.EffectiveAt.After(e.Timestamp) {
			continue
		}
		if selected == nil || p.EffectiveAt.After(selected.EffectiveAt) {
			selected = p
		}
	}
	if selected == nil && len(e.Legacy) > 0 && originalCost != nil {
		e.Cost, e.PriceVersion, e.PricingConfidence = originalCost, originalVersion, originalConfidence
		return
	}
	if selected == nil {
		if historical, ok := legacyRate(e.Model, e.Tokens.Input); ok {
			if cost, err := historical.Cost(e.Tokens); err == nil {
				e.Cost = &cost
				e.PriceVersion = historical.Version
				e.PricingConfidence = "legacy-rate-estimate"
				if strings.EqualFold(e.Model, "codex-auto-review") {
					e.PricingConfidence = "legacy-alias-estimate"
				}
			}
			return
		}
	}
	if selected != nil {
		if cost, err := selected.Cost(e.Tokens); err == nil {
			e.Cost = &cost
			e.PriceVersion = selected.Version
			e.PricingConfidence = "rate-card-estimate"
		}
	}
}

type DayWindow struct {
	Date       string
	Start, End time.Time
}

func DayWindows(start, end time.Time, loc *time.Location) []DayWindow {
	if loc == nil {
		loc = time.Local
	}
	s := start.In(loc)
	s = time.Date(s.Year(), s.Month(), s.Day(), 0, 0, 0, 0, loc)
	out := []DayWindow{}
	for s.Before(end) && len(out) < 3660 {
		next := s.AddDate(0, 0, 1)
		out = append(out, DayWindow{s.Format("2006-01-02"), s, next})
		s = next
	}
	return out
}
func eventID(profile, session string, t time.Time, info []byte) string {
	return storage.Hash([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s", profile, session, t.UTC().Format(time.RFC3339Nano), storage.Hash(info))))
}
