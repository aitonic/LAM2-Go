package codex

import (
	"context"
	"encoding/json"
	"errors"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClient struct {
	home   string
	closed atomic.Bool
	calls  atomic.Int32
}

func (c *fakeClient) Call(ctx context.Context, method string, params any, out any) error {
	if c.closed.Load() {
		return errors.New("closed")
	}
	c.calls.Add(1)
	b, _ := json.Marshal(map[string]string{"home": c.home, "method": method})
	return json.Unmarshal(b, out)
}
func (c *fakeClient) Close() error { c.closed.Store(true); return nil }
func TestPoolIsolatesProfilesAndBoundsProcesses(t *testing.T) {
	var mu sync.Mutex
	made := map[string]*fakeClient{}
	pool := NewPool(2, time.Minute, func(ctx context.Context, p model.Profile) (Client, error) {
		mu.Lock()
		defer mu.Unlock()
		c := &fakeClient{home: p.Home}
		made[p.ID] = c
		return c, nil
	})
	defer pool.Close()
	a := model.Profile{ID: "a", Home: t.TempDir()}
	b := model.Profile{ID: "b", Home: t.TempDir()}
	var wg sync.WaitGroup
	for _, p := range []model.Profile{a, b} {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out map[string]string
			if err := pool.Call(context.Background(), p, "thread/list", nil, &out); err != nil {
				t.Error(err)
			}
			if out["home"] != p.Home {
				t.Error("profile leakage")
			}
		}()
	}
	wg.Wait()
	if len(made) != 2 {
		t.Fatal(len(made))
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	for _, c := range made {
		if !c.closed.Load() {
			t.Fatal("leaked client")
		}
	}
}
func TestPoolRejectsMissingHomeAndReturnsSafeErrors(t *testing.T) {
	p := NewPool(1, time.Minute, func(context.Context, model.Profile) (Client, error) { return nil, errors.New("secret-token") })
	defer p.Close()
	err := p.Call(context.Background(), model.Profile{ID: "a", Home: t.TempDir()}, "account/read", nil, &map[string]any{})
	if fault.Code(err) != "CODEX_START_FAILED" {
		t.Fatal(err)
	}
}
func TestPoolConcurrentSameProfileCreatesOnce(t *testing.T) {
	var n atomic.Int32
	p := NewPool(2, time.Minute, func(context.Context, model.Profile) (Client, error) {
		n.Add(1)
		time.Sleep(10 * time.Millisecond)
		return &fakeClient{}, nil
	})
	defer p.Close()
	profile := model.Profile{ID: "a", Home: t.TempDir()}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out any
			if err := p.Call(context.Background(), profile, "account/read", nil, &out); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n.Load() != 1 {
		t.Fatal(n.Load())
	}
}
