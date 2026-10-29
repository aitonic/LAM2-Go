package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/provider"
	"lam.local/lam2/internal/storage"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type Snapshot struct {
	Binding  model.Binding
	Provider model.Provider
	Headers  http.Header
	Proxy    model.Proxy
}
type Resolver interface {
	Resolve(context.Context, string) (Snapshot, error)
}
type Exchange interface {
	Request() []byte
	NonStream(context.Context, []byte) ([]byte, error)
	Stream(context.Context, Frame) ([][]byte, error)
}
type Translator interface {
	New(context.Context, []byte, string, bool) (Exchange, error)
}
type Server struct {
	Limits             model.GatewayLimits
	Resolver           Resolver
	Translator         Translator
	HTTP               *http.Client
	ClientFor          func(Snapshot) (*http.Client, error)
	Limiter            *Limiter
	Identity           Identity
	Log                *slog.Logger
	Shutdown           func()
	Requests, Failures atomic.Uint64
	LastActive         atomic.Int64
}

func NewServer(l model.GatewayLimits, r Resolver, t Translator) *Server {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 64
	transport.MaxIdleConnsPerHost = 8
	transport.ResponseHeaderTimeout = time.Duration(l.FirstResponseSeconds) * time.Second
	return &Server{Limits: l, Resolver: r, Translator: t, HTTP: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, Limiter: NewLimiter(l.MaxConcurrent, l.PerBinding, l.MaxQueued, l.PerBindingQueued)}
}
func WriteError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	e := fault.Public(err)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": e.Code, "type": "lam_gateway_error", "message": e.Message}})
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	s.LastActive.Store(started.UnixNano())
	s.Requests.Add(1)
	id := storage.NewID()
	w.Header().Set("X-Request-ID", id)
	w.Header().Set("Cache-Control", "no-store")
	defer func() {
		s.LastActive.Store(time.Now().UnixNano())
		if s.Log != nil {
			s.Log.Info("gateway_request", "request_id", id, "duration_ms", time.Since(started).Milliseconds())
		}
	}()
	if r.URL.Path == "/control/shutdown" {
		s.control(w, r)
		return
	}
	if r.URL.Path == "/health" {
		s.health(w, r)
		return
	}
	if r.URL.Path != "/v1/models" && r.URL.Path != "/v1/responses" && r.URL.Path != "/v1/responses/compact" {
		WriteError(w, 404, fault.New("ROUTE_NOT_FOUND", "Unsupported gateway route"))
		return
	}
	if r.URL.Path == "/v1/responses/compact" {
		WriteError(w, 501, loss("compaction"))
		return
	}
	if r.Header.Get("Origin") != "" {
		WriteError(w, 403, fault.New("ORIGIN_FORBIDDEN", "Browser origins are not accepted"))
		return
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || len(auth) > 8192 {
		WriteError(w, 401, fault.New("GATEWAY_UNAUTHORIZED", "A binding token is required"))
		return
	}
	sn, e := s.Resolver.Resolve(r.Context(), strings.TrimPrefix(auth, "Bearer "))
	if e != nil {
		WriteError(w, 401, fault.New("GATEWAY_UNAUTHORIZED", "Binding authentication failed"))
		return
	}
	if r.URL.Path == "/v1/models" {
		if r.Method != "GET" {
			WriteError(w, 405, fault.New("METHOD_NOT_ALLOWED", "GET is required"))
			return
		}
		catalog, err := ProviderModelCatalog(sn)
		if err != nil {
			WriteError(w, http.StatusServiceUnavailable, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"models": catalog})
		return
	}
	if r.Method != "POST" {
		WriteError(w, 405, fault.New("METHOD_NOT_ALLOWED", "POST is required"))
		return
	}
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if ct != "application/json" {
		WriteError(w, 415, fault.New("CONTENT_TYPE_INVALID", "application/json is required"))
		return
	}
	raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, s.Limits.BodyBytes))
	if e != nil {
		WriteError(w, 413, fault.New("BODY_LIMIT", "Request body exceeds maximum size"))
		return
	}
	var request struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if json.Unmarshal(raw, &request) != nil {
		WriteError(w, 400, fault.New("REQUEST_INVALID", "Invalid JSON request"))
		return
	}
	if request.Model != "" && request.Model != sn.Binding.Model && !sn.Provider.HasModel(request.Model) {
		WriteError(w, 400, fault.New("MODEL_NOT_BOUND", "Model does not belong to this binding"))
		return
	}
	selectedModel := request.Model
	if selectedModel == "" {
		selectedModel = sn.Binding.Model
	}
	native := sn.Provider.Protocol == "responses"
	if !native {
		if e = Preflight(raw, sn.Provider.ModelCapabilities(selectedModel), s.Limits); e != nil {
			WriteError(w, 400, e)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(s.Limits.RequestSeconds)*time.Second)
	defer cancel()
	release, e := s.Limiter.Acquire(ctx, sn.Binding.ID)
	if e != nil {
		WriteError(w, 429, fault.New("GATEWAY_BUSY", "Gateway queue is full or request was cancelled"))
		return
	}
	defer release()
	scope := sn.Binding.ID + ":" + fmt.Sprint(sn.Binding.Generation)
	session := r.Header.Get("Session_id")
	if session == "" {
		session = r.Header.Get("Session-Id")
	}
	if len(session) > 256 || strings.ContainsAny(session, "\r\n\x00") {
		WriteError(w, 400, fault.New("HEADER_INVALID", "Invalid session identity"))
		return
	}
	if session == "" {
		session = id
	} // no shared history across anonymous requests
	ctx = WithExchangeContext(ctx, ExchangeContext{Scope: scope + ":" + session, Compatibility: sn.Provider.Compatibility})
	var ex Exchange
	if native {
		ex = &passthroughExchange{body: raw}
	} else if s.Translator != nil {
		ex, e = s.Translator.New(ctx, raw, selectedModel, request.Stream)
	} else {
		e = fault.New("TRANSLATOR_UNAVAILABLE", "No protocol translator is available")
	}
	if e != nil {
		WriteError(w, 400, e)
		return
	}
	body := ex.Request()
	if !json.Valid(body) || int64(len(body)) > s.Limits.ResponseBytes {
		WriteError(w, 502, fault.New("TRANSLATION_INVALID", "Translator produced an invalid request"))
		return
	}
	route := "chat/completions"
	if native {
		route = "responses"
	}
	if !native && sn.Provider.UpstreamPath != "" {
		route = strings.TrimPrefix(sn.Provider.UpstreamPath, "/")
	}
	endpoint, e := provider.EndpointFor(sn.Provider, route)
	if e != nil {
		WriteError(w, 502, e)
		return
	}
	up, e := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if e != nil {
		WriteError(w, 502, fault.New("UPSTREAM_INVALID", "Invalid upstream endpoint"))
		return
	}
	up.Header = sn.Headers.Clone()
	if up.Header == nil {
		up.Header = http.Header{}
	}
	for _, key := range []string{"Session_id", "Session-Id", "Conversation_id", "Conversation-Id", "OpenAI-Beta", "Originator", "Version", "X-Codex-Turn-State", "X-Codex-Turn-Metadata", "X-Codex-Parent-Thread-Id"} {
		if v := r.Header.Get(key); v != "" && len(v) <= 8192 && !strings.ContainsAny(v, "\r\n\x00") {
			up.Header.Set(key, v)
		}
	}
	up.Header.Set("Content-Type", "application/json")
	if request.Stream {
		up.Header.Set("Accept", "text/event-stream")
	}
	var firstExpired atomic.Bool
	firstTimer := time.AfterFunc(time.Duration(s.Limits.FirstResponseSeconds)*time.Second, func() { firstExpired.Store(true); cancel() })
	defer firstTimer.Stop()
	client := s.HTTP
	if s.ClientFor != nil {
		client, e = s.ClientFor(sn)
		if e != nil {
			WriteError(w, 502, fault.New("PROXY_INVALID", "Cannot configure the selected profile proxy"))
			return
		}
	}
	resp, e := client.Do(up)
	if e != nil {
		status, code := 502, "UPSTREAM_UNREACHABLE"
		if firstExpired.Load() {
			status, code = 504, "FIRST_RESPONSE_TIMEOUT"
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status, code = 504, "REQUEST_TIMEOUT"
		}
		WriteError(w, status, fault.New(code, "Upstream request failed"))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		firstTimer.Stop()
		if v := resp.Header.Get("Retry-After"); len(v) < 128 && !strings.ContainsAny(v, "\r\n") {
			w.Header().Set("Retry-After", v)
		}
		status := resp.StatusCode
		if status < 400 {
			status = 502
		}
		errorBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		message := upstreamErrorMessage(errorBody, sn.Headers)
		WriteError(w, status, fault.New("UPSTREAM_HTTP_ERROR", message))
		return
	}
	if !request.Stream {
		data, e := io.ReadAll(io.LimitReader(resp.Body, s.Limits.ResponseBytes+1))
		firstTimer.Stop()
		if e != nil || int64(len(data)) > s.Limits.ResponseBytes || !json.Valid(data) {
			WriteError(w, 502, fault.New("UPSTREAM_INVALID", "Invalid or oversized upstream JSON"))
			return
		}
		converted, e := ex.NonStream(ctx, data)
		if e != nil || !json.Valid(converted) {
			WriteError(w, 502, fault.New("TRANSLATION_INVALID", "Invalid converted response"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(converted)
		return
	}
	content, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if content != "text/event-stream" {
		WriteError(w, 502, fault.New("UPSTREAM_INVALID", "Upstream did not return an SSE stream"))
		return
	}
	s.stream(ctx, w, resp.Body, ex, firstTimer, &firstExpired)
}
func (s *Server) stream(ctx context.Context, w http.ResponseWriter, body io.Reader, ex Exchange, firstTimer *time.Timer, expired *atomic.Bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	controller := http.NewResponseController(w)
	terminal := false
	toolBytes := map[string]int{}
	var total int64
	write := func(raw []byte) error {
		if len(raw) > s.Limits.FrameBytes {
			return fault.New("SSE_FRAME_TOO_LARGE", "Converted event exceeds maximum")
		}
		_ = controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
		if _, e := w.Write(raw); e != nil {
			return e
		}
		return controller.Flush()
	}
	e := ReadSSE(body, s.Limits.FrameBytes, func(f Frame) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		total += int64(len(f.Data))
		if total > s.Limits.ResponseBytes {
			return fault.New("RESPONSE_LIMIT", "Stream exceeds response limit")
		}
		if len(f.Data) == 0 {
			return nil
		}
		if string(f.Data) != "[DONE]" && !json.Valid(f.Data) {
			return fault.New("UPSTREAM_INVALID", "Malformed SSE data")
		}
		if meaningful(f.Data) {
			firstTimer.Stop()
		}
		chunks, err := ex.Stream(ctx, f)
		if err != nil {
			return err
		}
		for _, chunk := range chunks {
			if terminal {
				break
			}
			err = ReadSSE(bytes.NewReader(chunk), s.Limits.FrameBytes, func(out Frame) error {
				var event map[string]json.RawMessage
				if json.Unmarshal(out.Data, &event) != nil {
					return fault.New("TRANSLATION_INVALID", "Invalid translated event")
				}
				var kind string
				json.Unmarshal(event["type"], &kind)
				if kind == "" {
					kind = out.Event
				}
				if kind == "response.function_call_arguments.delta" || kind == "response.custom_tool_call_input.delta" {
					var delta, item string
					json.Unmarshal(event["delta"], &delta)
					json.Unmarshal(event["item_id"], &item)
					toolBytes[item] += len(delta)
					if len(toolBytes) > s.Limits.MaxTools || toolBytes[item] > s.Limits.ToolArgumentBytes {
						return fault.New("TOOL_ARGUMENTS_LIMIT", "Tool stream exceeds limits")
					}
				}
				isTerminal := kind == "response.completed" || kind == "response.failed" || kind == "response.incomplete" || kind == "error"
				raw := []byte("event: " + kind + "\ndata: " + string(out.Data) + "\n\n")
				if err := write(raw); err != nil {
					return err
				}
				if isTerminal {
					terminal = true
					firstTimer.Stop()
					return errTerminal
				}
				return nil
			})
			if errors.Is(err, errTerminal) {
				return errTerminal
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if terminal {
		return
	}
	if ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled) && !expired.Load() {
		return
	}
	code := "UPSTREAM_TRUNCATED"
	if expired.Load() {
		code = "FIRST_RESPONSE_TIMEOUT"
	} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		code = "REQUEST_TIMEOUT"
	} else if e != nil {
		code = fault.Code(e)
		if code == "INTERNAL_ERROR" {
			code = "UPSTREAM_STREAM_ERROR"
		}
	}
	s.Failures.Add(1)
	payload, _ := json.Marshal(map[string]any{"type": "response.failed", "response": map[string]any{"status": "failed", "error": map[string]any{"code": code, "message": "Upstream stream did not complete successfully"}}})
	_ = write([]byte("event: response.failed\ndata: " + string(payload) + "\n\n"))
}
func meaningful(b []byte) bool {
	if string(b) == "[DONE]" {
		return true
	}
	var native struct {
		Type  string `json:"type"`
		Delta string `json:"delta"`
		Text  string `json:"text"`
	}
	_ = json.Unmarshal(b, &native)
	if strings.HasPrefix(native.Type, "response.") && (native.Delta != "" || native.Text != "" || native.Type == "response.completed" || native.Type == "response.failed" || native.Type == "response.incomplete" || native.Type == "response.output_item.added") {
		return true
	}
	var m struct {
		Choices []struct {
			Delta struct {
				Content   string            `json:"content"`
				Reasoning string            `json:"reasoning_content"`
				Tools     []json.RawMessage `json:"tool_calls"`
			} `json:"delta"`
			Finish any `json:"finish_reason"`
		} `json:"choices"`
	}
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	for _, c := range m.Choices {
		if c.Delta.Content != "" || c.Delta.Reasoning != "" || len(c.Delta.Tools) > 0 || c.Finish != nil {
			return true
		}
	}
	return false
}
func Listen(address string) (net.Listener, error) {
	host, port, e := net.SplitHostPort(address)
	if e != nil || host != "127.0.0.1" {
		return nil, fault.New("LISTEN_ADDRESS_INVALID", "Gateway must listen on IPv4 loopback")
	}
	n, e := strconv.Atoi(port)
	if e != nil || n < 0 || n > 65535 {
		return nil, fault.New("LISTEN_ADDRESS_INVALID", "Invalid gateway port")
	}
	return net.Listen("tcp4", address)
}
