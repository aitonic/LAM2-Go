package gateway

import (
	"context"
	"encoding/json"
	"lam.local/lam2/internal/fault"
)

// Native Responses goes through no Chat translator, so images, hosted tools,
// compaction history and provider-specific fields remain byte-for-byte intact.
type passthroughExchange struct{ body []byte }

func (p *passthroughExchange) Request() []byte { return p.body }
func (p *passthroughExchange) NonStream(_ context.Context, b []byte) ([]byte, error) {
	if !json.Valid(b) {
		return nil, fault.New("UPSTREAM_INVALID", "Invalid Responses payload")
	}
	return b, nil
}
func (p *passthroughExchange) Stream(_ context.Context, f Frame) ([][]byte, error) {
	if len(f.Data) == 0 || string(f.Data) == "[DONE]" {
		return nil, nil
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(f.Data, &m) != nil {
		return nil, fault.New("UPSTREAM_INVALID", "Invalid Responses SSE")
	}
	var kind string
	_ = json.Unmarshal(m["type"], &kind)
	if kind == "" {
		kind = f.Event
		m["type"], _ = json.Marshal(kind)
		f.Data, _ = json.Marshal(m)
	}
	b, e := SSEEvent(f.Data)
	if e != nil {
		return nil, e
	}
	return [][]byte{b}, nil
}
