package gateway

import (
	"context"
	"lam.local/lam2/internal/fault"
	"sync"
)

type exchangeKey struct{}
type ExchangeContext struct{ Scope, Compatibility string }

func WithExchangeContext(ctx context.Context, c ExchangeContext) context.Context {
	return context.WithValue(ctx, exchangeKey{}, c)
}
func ExchangeContextFrom(ctx context.Context) ExchangeContext {
	v, _ := ctx.Value(exchangeKey{}).(ExchangeContext)
	return v
}

type reasoningEntry struct {
	Calls map[string]string
	Bytes int
	Stamp uint64
}

// ReasoningHistory holds bounded process-local tool reasoning, separated by
// binding generation AND CLI session. No prompt/reasoning is persisted/logged.
type ReasoningHistory struct {
	mu                    sync.Mutex
	entries               map[string]*reasoningEntry
	maxSessions, maxBytes int
	tick                  uint64
}

func NewReasoningHistory(sessions, bytes int) *ReasoningHistory {
	return &ReasoningHistory{entries: map[string]*reasoningEntry{}, maxSessions: max(1, sessions), maxBytes: max(1, bytes)}
}
func (h *ReasoningHistory) Record(scope string, calls []string, text string) error {
	if text == "" || len(calls) == 0 || scope == "" {
		return nil
	}
	if len(text) > h.maxBytes {
		return fault.New("ADAPTER_REASONING_LIMIT_EXCEEDED", "Reasoning history exceeds limit")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.tick++
	e := h.entries[scope]
	if e == nil {
		if len(h.entries) >= h.maxSessions {
			var oldest string
			var stamp uint64 = ^uint64(0)
			for k, v := range h.entries {
				if v.Stamp < stamp {
					oldest = k
					stamp = v.Stamp
				}
			}
			delete(h.entries, oldest)
		}
		e = &reasoningEntry{Calls: map[string]string{}}
		h.entries[scope] = e
	}
	// Do not duplicate the same turn's reasoning across all IDs for byte accounting.
	used := map[string]bool{}
	total := len(text)
	used[text] = true
	for id, v := range e.Calls {
		replace := false
		for _, c := range calls {
			if c == id {
				replace = true
				break
			}
		}
		if !replace && !used[v] {
			total += len(v)
			used[v] = true
		}
	}
	if total > h.maxBytes {
		return fault.New("ADAPTER_REASONING_LIMIT_EXCEEDED", "Session reasoning exceeds limit")
	}
	for _, c := range calls {
		e.Calls[c] = text
	}
	e.Bytes = total
	e.Stamp = h.tick
	return nil
}
func (h *ReasoningHistory) Lookup(scope, call string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if e := h.entries[scope]; e != nil {
		h.tick++
		e.Stamp = h.tick
		return e.Calls[call]
	}
	return ""
}
func (h *ReasoningHistory) Snapshot(scope string) map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[string]string{}
	if e := h.entries[scope]; e != nil {
		h.tick++
		e.Stamp = h.tick
		for k, v := range e.Calls {
			out[k] = v
		}
	}
	return out
}
