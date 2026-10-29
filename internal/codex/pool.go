// Package codex owns per-profile SDK lifetimes, not the JSON-RPC implementation.
package codex

import (
	"context"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"path/filepath"
	"sync"
	"time"
)

type Client interface {
	Call(context.Context, string, any, any) error
	Close() error
}
type Factory func(context.Context, model.Profile) (Client, error)
type Caller interface {
	Call(context.Context, model.Profile, string, any, any) error
}
type entry struct {
	ready  chan struct{}
	client Client
	err    error
	refs   int
	last   time.Time
	home   string
}
type Pool struct {
	mu      sync.Mutex
	max     int
	idle    time.Duration
	factory Factory
	entries map[string]*entry
	closed  bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewPool(maximum int, idle time.Duration, f Factory) *Pool {
	if maximum < 1 {
		maximum = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Pool{max: maximum, idle: idle, factory: f, entries: map[string]*entry{}, ctx: ctx, cancel: cancel}
}
func (p *Pool) Call(ctx context.Context, profile model.Profile, method string, params, out any) error {
	if profile.ID == "" || !filepath.IsAbs(profile.Home) {
		return fault.New("PROFILE_HOME_INVALID", "A profile must have an absolute, isolated Codex home")
	}
	if err := storage.RejectSymlinks(profile.Home); err != nil {
		return err
	}
	key := profile.ID + "\x00" + filepath.Clean(profile.Home)
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return fault.New("CODEX_POOL_CLOSED", "Codex connections are closed")
	}
	e := p.entries[key]
	if e == nil {
		var old Client
		if len(p.entries) >= p.max {
			var victim string
			var oldest time.Time
			for k, v := range p.entries {
				select {
				case <-v.ready:
					if v.refs == 0 && (oldest.IsZero() || v.last.Before(oldest)) {
						victim = k
						oldest = v.last
					}
				default:
				}
			}
			if victim == "" {
				p.mu.Unlock()
				return fault.New("CODEX_POOL_BUSY", "All Codex connections are in use; retry shortly")
			}
			old = p.entries[victim].client
			delete(p.entries, victim)
		}
		e = &entry{ready: make(chan struct{}), home: profile.Home, last: time.Now()}
		p.entries[key] = e
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			if old != nil {
				_ = old.Close()
			}
			c, err := p.factory(p.ctx, profile)
			p.mu.Lock()
			if p.closed || p.entries[key] != e {
				p.mu.Unlock()
				if c != nil {
					_ = c.Close()
				}
				p.mu.Lock()
				e.err = fault.New("CODEX_POOL_CLOSED", "The connection was canceled")
			} else {
				e.client = c
				e.err = err
			}
			close(e.ready)
			p.mu.Unlock()
		}()
	}
	p.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-e.ready:
	}
	p.mu.Lock()
	if p.closed || p.entries[key] != e {
		p.mu.Unlock()
		return fault.New("CODEX_POOL_CLOSED", "The connection was closed")
	}
	if e.err != nil {
		delete(p.entries, key)
		err := e.err
		p.mu.Unlock()
		if fault.Code(err) != "INTERNAL_ERROR" {
			return err
		}
		return fault.Wrap("CODEX_START_FAILED", "Codex could not start; verify installation and the pinned CLI version", err)
	}
	e.refs++
	p.mu.Unlock()
	defer func() { p.mu.Lock(); e.refs--; e.last = time.Now(); p.mu.Unlock() }()
	if err := e.client.Call(ctx, method, params, out); err != nil {
		if fault.Code(err) != "INTERNAL_ERROR" {
			return err
		}
		return fault.Wrap("CODEX_RPC_FAILED", "Codex rejected the operation; check authentication and CLI compatibility", err)
	}
	return nil
}
func (p *Pool) InvalidateHome(home string) {
	var closing []Client
	p.mu.Lock()
	for k, e := range p.entries {
		if e.home == home {
			delete(p.entries, k)
			select {
			case <-e.ready:
				if e.client != nil {
					closing = append(closing, e.client)
				}
			default:
			}
		}
	}
	p.mu.Unlock()
	for _, c := range closing {
		_ = c.Close()
	}
}
func (p *Pool) Prune() {
	var closing []Client
	p.mu.Lock()
	for k, e := range p.entries {
		select {
		case <-e.ready:
			if e.refs == 0 && time.Since(e.last) > p.idle {
				delete(p.entries, k)
				if e.client != nil {
					closing = append(closing, e.client)
				}
			}
		default:
		}
	}
	p.mu.Unlock()
	for _, c := range closing {
		_ = c.Close()
	}
}
func (p *Pool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.cancel()
	var closing []Client
	for _, e := range p.entries {
		select {
		case <-e.ready:
			if e.client != nil {
				closing = append(closing, e.client)
			}
		default:
		}
	}
	p.mu.Unlock()
	for _, c := range closing {
		_ = c.Close()
	}
	p.wg.Wait()
	return nil
}
