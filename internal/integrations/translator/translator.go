// Package translator contains only the public CLIProxyAPI adapter. The HTTP
// server, admission control and binding isolation remain LAM application code.
package translator

import (
	"bytes"
	"context"
	"encoding/json"
	sdk "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	_ "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator/builtin"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/gateway"
	"lam.local/lam2/internal/model"
)

type Adapter struct{ history *gateway.ReasoningHistory }

func New() (*Adapter, error) {
	from, to := sdk.FormatOpenAIResponse, sdk.FormatOpenAI
	if !sdk.HasRequestTransformer(from, to) || !sdk.HasStreamResponseTransformer(from, to) || !sdk.HasNonStreamResponseTransformer(from, to) {
		return nil, fault.New("TRANSLATOR_UNAVAILABLE", "Required Responses/Chat transforms were not registered")
	}
	return &Adapter{history: gateway.NewReasoningHistory(256, 1<<20)}, nil
}

type exchange struct {
	model             string
	original, request []byte
	state             any
	bridge            *gateway.ToolBridge
	history           *gateway.ReasoningHistory
	scope             string
	collector         *gateway.ReasoningCollector
}

func (a *Adapter) New(ctx context.Context, raw []byte, modelID string, stream bool) (gateway.Exchange, error) {
	bridge, normalized, err := gateway.PrepareCompatibility(raw, model.DefaultLimits())
	if err != nil {
		return nil, err
	}
	info := gateway.ExchangeContextFrom(ctx)

	r := sdk.TranslateRequestEnvelope(ctx, sdk.FormatOpenAIResponse, sdk.FormatOpenAI, sdk.RequestEnvelope{Format: sdk.FormatOpenAIResponse, Model: modelID, Stream: stream, Body: normalized})
	if r.Err != nil {
		return nil, fault.Wrap("PROTOCOL_LOSS", "Translator refused this request", r.Err)
	}
	if !json.Valid(r.Body) {
		return nil, fault.New("PROTOCOL_LOSS", "Translator returned invalid JSON")
	}
	body, err := bridge.PatchChat(r.Body, info.Compatibility, a.history.Snapshot(info.Scope))
	if err != nil {
		return nil, err
	}
	return &exchange{model: modelID, original: normalized, request: body, bridge: bridge, history: a.history, scope: info.Scope, collector: gateway.NewReasoningCollector(128, 1<<20)}, nil
}
func (e *exchange) Request() []byte { return e.request }
func (e *exchange) NonStream(ctx context.Context, raw []byte) ([]byte, error) {
	if err := e.collector.Observe(raw); err != nil {
		return nil, err
	}
	out := sdk.TranslateNonStream(ctx, sdk.FormatOpenAI, sdk.FormatOpenAIResponse, e.model, e.original, e.request, raw, &e.state)
	if v, ok := e.state.(interface{ ToolInputError() error }); ok && v.ToolInputError() != nil {
		return nil, fault.New("PROTOCOL_LOSS", "Invalid upstream tool arguments")
	}
	if !json.Valid(out) {
		return nil, fault.New("PROTOCOL_LOSS", "Invalid translated response")
	}
	if err := e.collector.Commit(e.history, e.scope); err != nil {
		return nil, err
	}
	return e.bridge.RestoreResponse(out)
}
func (e *exchange) Stream(ctx context.Context, f gateway.Frame) ([][]byte, error) {
	if len(bytes.TrimSpace(f.Data)) == 0 {
		return nil, nil
	}
	if err := e.collector.Observe(f.Data); err != nil {
		return nil, err
	}
	// This public transform consumes the SSE data-line form, not a bare JSON chunk.
	line := append([]byte("data: "), f.Data...)
	out := sdk.TranslateStream(ctx, sdk.FormatOpenAI, sdk.FormatOpenAIResponse, e.model, e.original, e.request, line, &e.state)
	if v, ok := e.state.(interface{ ToolInputError() error }); ok && v.ToolInputError() != nil {
		return nil, fault.New("PROTOCOL_LOSS", "Invalid upstream tool arguments")
	}
	var restored [][]byte
	for _, chunk := range out {
		emit := func(frame gateway.Frame) error {
			if len(bytes.TrimSpace(frame.Data)) == 0 || bytes.Equal(bytes.TrimSpace(frame.Data), []byte("[DONE]")) {
				return nil
			}
			events, err := e.bridge.RestoreEvent(frame.Data)
			if err != nil {
				return err
			}
			for _, event := range events {
				b, err := gateway.SSEEvent(event)
				if err != nil {
					return err
				}
				restored = append(restored, b)
			}
			return nil
		}
		if json.Valid(bytes.TrimSpace(chunk)) {
			if err := emit(gateway.Frame{Data: chunk}); err != nil {
				return nil, err
			}
		} else {
			if err := gateway.ReadSSE(bytes.NewReader(chunk), model.DefaultLimits().FrameBytes, emit); err != nil {
				return nil, err
			}
		}
	}
	if err := e.collector.Commit(e.history, e.scope); err != nil {
		return nil, err
	}
	return restored, nil
}
