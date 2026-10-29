package quota

import (
	"context"
	"encoding/json"
	"errors"
	"lam.local/lam2/internal/account"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestParseCodexQuotaNoInventedWindow(t *testing.T) {
	raw := []byte(`{"rateLimits":{"limitId":"codex","primary":{"usedPercent":25,"windowDurationMins":300,"resetsAt":1790000000},"secondary":null,"planType":"plus"},"rateLimitResetCredits":{"availableCount":2,"credits":null}}`)
	q, e := ParseCodex("p", raw, time.Now(), time.Minute)
	if e != nil || len(q.Windows) != 1 || *q.Windows[0].RemainingPercent != 75 || q.Credits.ExpiresAt != nil || *q.Credits.AvailableCount != 2 {
		t.Fatal(q, e)
	}
}
func TestMultipleBucketsDoNotDoubleCount(t *testing.T) {
	q, e := ParseCodex("p", []byte(`{"rateLimits":{"primary":{"usedPercent":1}},"rateLimitsByLimitId":{"codex":{"primary":{"usedPercent":30}},"other":{"secondary":{"usedPercent":60}}}}`), time.Now(), time.Minute)
	if e != nil || len(q.Windows) != 2 {
		t.Fatal(q, e)
	}
}
func TestUnknownQuotaNotZero(t *testing.T) {
	q, e := ParseCodex("p", []byte(`{"rateLimits":{"primary":null,"secondary":null}}`), time.Now(), time.Minute)
	if e == nil || q.Status == "ready" {
		t.Fatal(q, e)
	}
	_, e = ParseCodex("p", []byte(`{"rateLimits":{"primary":{"usedPercent":101}}}`), time.Now(), time.Minute)
	if e == nil {
		t.Fatal("invalid percent")
	}
}

type caller struct {
	mu    sync.Mutex
	calls int
	fail  bool
	keys  []string
}

func (c *caller) Call(ctx context.Context, p model.Profile, method string, params any, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if method == "account/rateLimitResetCredit/consume" {
		m := params.(map[string]any)
		c.keys = append(c.keys, m["idempotencyKey"].(string))
		if c.fail {
			return errors.New("private token")
		}
		return json.Unmarshal([]byte(`{"outcome":"alreadyRedeemed"}`), out)
	}
	if c.fail {
		return errors.New("private token")
	}
	time.Sleep(10 * time.Millisecond)
	return json.Unmarshal([]byte(`{"rateLimits":{"primary":{"usedPercent":20}}}`), out)
}
func qsetup(t *testing.T) (*Service, *caller, model.Profile) {
	st, _ := storage.Open(t.TempDir())
	a := account.Service{Store: st}
	p, e := a.Create(context.Background(), "a", "profile", 1)
	if e != nil {
		t.Fatal(e)
	}
	c := &caller{}
	return New(st, c), c, p
}
func TestRefreshSingleFlight(t *testing.T) {
	s, c, p := qsetup(t)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := s.Refresh(context.Background(), p.ID); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.calls != 1 {
		t.Fatal(c.calls)
	}
}
func TestFailurePreservesStaleSnapshot(t *testing.T) {
	s, c, p := qsetup(t)
	q, e := s.Refresh(context.Background(), p.ID)
	if e != nil {
		t.Fatal(e)
	}
	c.fail = true
	stale, e := s.Refresh(context.Background(), p.ID)
	if e == nil || !stale.Stale || stale.FetchedAt != q.FetchedAt || len(stale.Windows) != 1 {
		t.Fatal(stale, e)
	}
	if stale.ErrorCode == "private token" {
		t.Fatal("error leak")
	}
}
func TestResetIdempotencyPersistsAcrossRetry(t *testing.T) {
	s, c, p := qsetup(t)
	ctx := context.Background()
	key, e := s.PrepareReset(ctx, p.ID, "credit-one")
	if e != nil {
		t.Fatal(e)
	}
	c.fail = true
	if _, e = s.ConsumeReset(ctx, key, false); fault.Code(e) != "CONFIRMATION_REQUIRED" {
		t.Fatal(e)
	}
	if _, e = s.ConsumeReset(ctx, key, true); e == nil {
		t.Fatal("failure hidden")
	}
	c.fail = false
	if _, e = s.ConsumeReset(ctx, key, true); e != nil {
		t.Fatal(e)
	}
	if len(c.keys) != 2 || c.keys[0] != c.keys[1] {
		t.Fatal(c.keys)
	}
}
func TestAntigravityGroupingAndUnavailable(t *testing.T) {
	raw := []byte(`{"userStatus":{"cascadeModelConfigData":{"clientModelConfigs":[{"label":"model-x","quotaInfo":{"remainingFraction":0.25,"resetTime":"2026-10-28T00:00:00Z"}},{"label":"unknown-model","quotaInfo":{}}]}}}`)
	q, e := ParseAntigravity(raw, nil, time.Now(), time.Minute)
	if e != nil || len(q.Windows) != 2 || *q.Windows[0].RemainingPercent != 25 || q.Windows[1].RemainingPercent != nil {
		t.Fatal(q, e)
	}
	_, e = ParseAntigravity([]byte(`{}`), nil, time.Now(), time.Minute)
	if e == nil {
		t.Fatal("invented quota")
	}
}
func TestAntigravityLoopbackOnly(t *testing.T) {
	a := Antigravity{URL: "http://example.test:1234"}
	if _, e := a.Read(context.Background()); e == nil {
		t.Fatal("remote accepted")
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer up.Close()
	a.URL = up.URL
	if _, e := a.Read(context.Background()); e == nil {
		t.Fatal("authentication ignored")
	}
}

// A cached profile quota must be observable without contacting App Server and
// must not mistake unavailable for zero remaining.
func TestCachedQuotaReadAndStaleTransition(t *testing.T) {
	svc, upstream, p := qsetup(t)
	ctx := context.Background()
	cold, err := svc.Read(ctx, p.ID)
	if err != nil || cold.Status != "unavailable" || len(cold.Windows) != 0 {
		t.Fatalf("cold cache: %+v %v", cold, err)
	}
	fresh, err := svc.Refresh(ctx, p.ID)
	if err != nil || fresh.ProfileID != p.ID || len(fresh.Windows) != 1 {
		t.Fatalf("refresh: %+v %v", fresh, err)
	}
	upstream.mu.Lock()
	calls := upstream.calls
	upstream.mu.Unlock()
	cached, err := svc.Read(ctx, p.ID)
	if err != nil || cached.Stale || cached.ProfileID != p.ID || len(cached.Windows) != 1 {
		t.Fatalf("cached: %+v %v", cached, err)
	}
	upstream.mu.Lock()
	again := upstream.calls
	upstream.mu.Unlock()
	if calls != again {
		t.Fatal("cache read contacted upstream")
	}
	cached.ExpiresAt = time.Now().Add(-time.Minute)
	svc.mu.Lock()
	svc.cache[p.ID] = cached
	svc.mu.Unlock()
	stale, err := svc.Read(ctx, p.ID)
	if err != nil || !stale.Stale || stale.Status != "stale" || len(stale.Windows) != 1 {
		t.Fatalf("expired: %+v %v", stale, err)
	}
}
