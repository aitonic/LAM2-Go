package gateway

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"io"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Supervisor struct{ Root, Executable string }

func (s *Supervisor) identityPath() string { return filepath.Join(s.Root, "gateway", "identity.json") }
func localHTTP() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	return &http.Client{Transport: t, Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func (s *Supervisor) Probe(ctx context.Context) (Identity, Health, error) {
	var i Identity
	var h Health
	raw, e := storage.ReadFile(s.identityPath(), 16<<10)
	if e != nil {
		return i, h, e
	}
	if json.Unmarshal(raw, &i) != nil {
		return i, h, fault.New("GATEWAY_IDENTITY_INVALID", "Invalid gateway identity")
	}
	host, _, e := net.SplitHostPort(i.Address)
	if e != nil || host != "127.0.0.1" {
		return i, h, fault.New("GATEWAY_IDENTITY_INVALID", "Gateway must be on IPv4 loopback")
	}
	hash, e := storage.FileHash(s.Executable)
	if e != nil {
		return i, h, e
	}
	if hash != i.ExecutableHash {
		return i, h, fault.New("GATEWAY_VERSION_CONFLICT", "Stop the existing gateway before switching application builds")
	}
	challenge := storage.NewID() + storage.NewID()
	req, e := http.NewRequestWithContext(ctx, "GET", "http://"+i.Address+"/health?challenge="+url.QueryEscape(challenge), nil)
	if e != nil {
		return i, h, e
	}
	client := localHTTP()
	defer client.CloseIdleConnections()
	res, e := client.Do(req)
	if e != nil {
		return i, h, e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 16384)).Decode(&h) != nil || !VerifyHealth(i, challenge, h) {
		return i, h, fault.New("GATEWAY_IDENTITY_INVALID", "Listener failed the gateway identity challenge")
	}
	return i, h, nil
}
func (s *Supervisor) Ensure(ctx context.Context, b model.Binding) (string, error) {
	if err := storage.EnsurePrivateDir(filepath.Join(s.Root, "gateway")); err != nil {
		return "", err
	}
	unlock, e := storage.Lock(ctx, filepath.Join(s.Root, "gateway", "launch.lock"))
	if e != nil {
		return "", e
	}
	defer unlock()
	if i, _, err := s.Probe(ctx); err == nil {
		if b.GatewayURL != "" && strings.TrimSuffix(b.GatewayURL, "/v1") != "http://"+i.Address {
			return "", fault.New("GATEWAY_ADDRESS_CONFLICT", "Binding points to a different gateway address")
		}
		return "http://" + i.Address + "/v1", nil
	} else if fault.Code(err) == "GATEWAY_VERSION_CONFLICT" || fault.Code(err) == "GATEWAY_IDENTITY_INVALID" {
		return "", err
	}
	address := "127.0.0.1:0"
	if b.GatewayURL != "" {
		u, e := url.Parse(b.GatewayURL)
		if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" {
			return "", fault.New("GATEWAY_ADDRESS_INVALID", "Invalid saved binding gateway")
		}
		address = u.Host
	}
	if address == "127.0.0.1:0" {
		if store, e := storage.Open(s.Root); e == nil {
			if st, e := store.Read(ctx); e == nil && st.Settings.GatewayPort > 0 {
				address = net.JoinHostPort("127.0.0.1", strconv.Itoa(st.Settings.GatewayPort))
			}
		}
	}
	// Preserve the established address across restarts, including before a new bind.
	if address == "127.0.0.1:0" {
		if raw, e := storage.ReadFile(s.identityPath(), 16384); e == nil {
			var old Identity
			if json.Unmarshal(raw, &old) == nil {
				if host, _, e := net.SplitHostPort(old.Address); e == nil && host == "127.0.0.1" {
					address = old.Address
				}
			}
		}
	}
	if address == "127.0.0.1:0" {
		if st, e := storage.Open(s.Root); e == nil {
			if state, e := st.Read(ctx); e == nil {
				for _, saved := range state.Bindings {
					if saved.RouteKind == "gateway" && saved.GatewayURL != "" {
						u, e := url.Parse(saved.GatewayURL)
						if e == nil && u.Hostname() == "127.0.0.1" {
							address = u.Host
							break
						}
					}
				}
			}
		}
	}
	logPath := filepath.Join(s.Root, "gateway", "daemon.log")
	if f, e := os.Stat(logPath); e == nil && f.Size() > 5<<20 {
		_ = os.Rename(logPath, logPath+".previous")
	}
	log, e := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return "", e
	}
	defer log.Close()
	cmd := exec.Command(s.Executable, "--home", s.Root, "gateway", "--listen", address)
	detach(cmd)
	cmd.Stdout = log
	cmd.Stderr = log
	if e = cmd.Start(); e != nil {
		return "", fault.Wrap("GATEWAY_START_FAILED", "Could not start gateway process", e)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(80 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timer.C:
			return "", fault.New("GATEWAY_START_TIMEOUT", "Gateway did not become ready; inspect sanitized daemon log")
		case <-done:
			return "", fault.New("GATEWAY_START_FAILED", "Gateway exited before readiness; no foreign process was stopped")
		case <-tick.C:
			if i, _, e := s.Probe(ctx); e == nil {
				return "http://" + i.Address + "/v1", nil
			}
		}
	}
}
func (s *Supervisor) Stop(ctx context.Context) error {
	i, h, e := s.Probe(ctx)
	if e != nil {
		return e
	}
	if h.Active > 0 || h.Queued > 0 {
		return fault.New("GATEWAY_BUSY", "Active CLI requests are still using the gateway")
	}
	challenge := storage.NewID() + storage.NewID()
	req, e := http.NewRequestWithContext(ctx, "POST", "http://"+i.Address+"/control/shutdown?challenge="+url.QueryEscape(challenge), nil)
	if e != nil {
		return e
	}
	req.Header.Set("X-LAM-Control", proof(i, "stop:"+challenge))
	c := localHTTP()
	defer c.CloseIdleConnections()
	res, e := c.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 202 {
		return fault.New("GATEWAY_STOP_FAILED", "Gateway refused shutdown")
	}
	return nil
}

// StopAndWait only controls an authenticated daemon belonging to this build.
// It never sends a signal to a PID or an unrelated process on the same port.
func (s *Supervisor) StopAndWait(ctx context.Context) error {
	if e := storage.EnsurePrivateDir(filepath.Join(s.Root, "gateway")); e != nil {
		return e
	}
	_, _, e := s.Probe(ctx)
	if e == nil {
		if e = s.Stop(ctx); e != nil {
			return e
		}
	} else {
		if fault.Code(e) == "GATEWAY_IDENTITY_INVALID" || fault.Code(e) == "GATEWAY_VERSION_CONFLICT" {
			return e
		}
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	unlock, e := storage.Lock(wait, filepath.Join(s.Root, "gateway", "daemon.lock"))
	if e != nil {
		return fault.Wrap("GATEWAY_BUSY", "Could not confirm that the owned gateway stopped", e)
	}
	defer unlock()
	return nil
}

func (s *Server) control(w http.ResponseWriter, r *http.Request) {
	challenge := r.URL.Query().Get("challenge")
	if r.Method != "POST" || len(challenge) < 32 || len(challenge) > 128 || !hmac.Equal([]byte(r.Header.Get("X-LAM-Control")), []byte(proof(s.Identity, "stop:"+challenge))) {
		WriteError(w, 403, fault.New("GATEWAY_UNAUTHORIZED", "Control proof is invalid"))
		return
	}
	cap := s.Limiter.Snapshot()
	if cap.Active > 0 || cap.Queued > 0 {
		WriteError(w, 409, fault.New("GATEWAY_BUSY", "Requests are still active"))
		return
	}
	if s.Shutdown == nil {
		WriteError(w, 503, fault.New("GATEWAY_STOP_FAILED", "Shutdown handler unavailable"))
		return
	}
	w.WriteHeader(202)
	go s.Shutdown()
}
func RunDaemon(ctx context.Context, root, exe, address string, server *Server) error {
	if err := storage.EnsurePrivateDir(filepath.Join(root, "gateway")); err != nil {
		return err
	}
	claimCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	unlock, e := storage.Lock(claimCtx, filepath.Join(root, "gateway", "daemon.lock"))
	if e != nil {
		return fault.Wrap("GATEWAY_ALREADY_RUNNING", "Another gateway owns this data directory", e)
	}
	defer unlock()
	listener, e := Listen(address)
	if e != nil {
		return fault.Wrap("GATEWAY_PORT_BUSY", "Cannot bind gateway port; no other process was stopped", e)
	}
	defer listener.Close()
	hash, e := storage.FileHash(exe)
	if e != nil {
		return e
	}
	server.Identity = Identity{PID: os.Getpid(), ExecutableHash: hash, Nonce: storage.NewID() + storage.NewID(), Address: listener.Addr().String(), StartedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	statePath := filepath.Join(root, "gateway", "identity.json")
	raw, _ := json.Marshal(server.Identity)
	if e = storage.AtomicWrite(ctx, statePath, raw, 0600); e != nil {
		return e
	}
	stopped := make(chan struct{}, 1)
	server.Shutdown = func() {
		select {
		case stopped <- struct{}{}:
		default:
		}
	}
	httpServer := &http.Server{Handler: server, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	done := make(chan error, 1)
	go func() { done <- httpServer.Serve(listener) }()
	select {
	case <-ctx.Done():
	case <-stopped:
	case e = <-done:
	}
	shutdownCtx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	_ = httpServer.Shutdown(shutdownCtx)
	server.HTTP.CloseIdleConnections()
	// Keep address metadata for restart but delete only an identity owned by us.
	if current, err := storage.ReadFile(statePath, 16384); err == nil && string(current) == string(raw) {
		_ = os.Remove(statePath)
	}
	if e != nil && !errors.Is(e, http.ErrServerClosed) {
		return e
	}
	return nil
}
