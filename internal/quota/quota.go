// Package quota keeps server quota separate from locally observed token usage.
package quota

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"lam.local/lam2/internal/codex"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
)

type quotaWindow struct {
	UsedPercent   *float64 `json:"usedPercent"`
	WindowMinutes int64    `json:"windowDurationMins"`
	ResetsAt      *int64   `json:"resetsAt"`
}
type quotaBucket struct {
	PlanType           string `json:"planType"`
	Primary, Secondary *quotaWindow
}
type quotaResponse struct {
	RateLimits   *quotaBucket           `json:"rateLimits"`
	ByID         map[string]quotaBucket `json:"rateLimitsByLimitId"`
	ResetCredits *struct {
		Count   *int `json:"availableCount"`
		Credits []struct {
			Expires *int64 `json:"expiresAt"`
			Status  string `json:"status"`
		} `json:"credits"`
	} `json:"rateLimitResetCredits"`
}

func ParseCodex(profileID string, raw []byte, now time.Time, ttl time.Duration) (model.QuotaSnapshot, error) {
	q := model.QuotaSnapshot{ProfileID: profileID, Source: "codex-app-server", Status: "unavailable", Windows: []model.QuotaWindow{}, FetchedAt: now, ExpiresAt: now.Add(ttl)}
	var r quotaResponse
	if json.Unmarshal(raw, &r) != nil {
		return q, fault.New("QUOTA_INVALID", "Malformed quota response")
	}
	buckets := r.ByID
	if len(buckets) == 0 && r.RateLimits != nil {
		buckets = map[string]quotaBucket{"codex": *r.RateLimits}
	}
	keys := []string{}
	for id := range buckets {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		b := buckets[id]
		if b.PlanType != "" {
			q.PlanType = b.PlanType
		}
		for i, w := range []*quotaWindow{b.Primary, b.Secondary} {
			if w == nil {
				continue
			}
			name := "primary"
			if i == 1 {
				name = "secondary"
			}
			out := model.QuotaWindow{ID: id + "/" + name, UsedPercent: w.UsedPercent, WindowMinutes: w.WindowMinutes}
			if w.UsedPercent != nil {
				if *w.UsedPercent < 0 || *w.UsedPercent > 100 {
					return q, fault.New("QUOTA_INVALID", "Quota percentage is outside its valid range")
				}
				remaining := 100 - *w.UsedPercent
				out.RemainingPercent = &remaining
			}
			if w.ResetsAt != nil {
				t := time.Unix(*w.ResetsAt, 0).UTC()
				out.ResetsAt = &t
			}
			q.Windows = append(q.Windows, out)
		}
	}
	if r.ResetCredits != nil {
		q.Credits = &model.Credits{AvailableCount: r.ResetCredits.Count}
		if q.Credits.AvailableCount != nil && *q.Credits.AvailableCount < 0 {
			return q, fault.New("QUOTA_INVALID", "Reset credit count is invalid")
		}
		for _, c := range r.ResetCredits.Credits {
			if c.Status == "available" && c.Expires != nil {
				t := time.Unix(*c.Expires, 0).UTC()
				if q.Credits.ExpiresAt == nil || t.Before(*q.Credits.ExpiresAt) {
					q.Credits.ExpiresAt = &t
				}
			}
		}
	}
	if len(q.Windows) == 0 && q.Credits == nil {
		return q, fault.New("QUOTA_UNAVAILABLE", "The account did not report quota windows")
	}
	q.Status = "ready"
	return q, nil
}

type flight struct {
	done  chan struct{}
	value model.QuotaSnapshot
	err   error
}
type Service struct {
	LookupProfile  func(context.Context, string) (model.Profile, error)
	PrepareProfile func(context.Context, model.Profile) (model.Profile, error)
	Store          *storage.Store
	Codex          codex.Caller
	mu             sync.Mutex
	cache          map[string]model.QuotaSnapshot
	inflight       map[string]*flight
	TTL            time.Duration
}

func New(s *storage.Store, c codex.Caller) *Service {
	return &Service{Store: s, Codex: c, cache: map[string]model.QuotaSnapshot{}, inflight: map[string]*flight{}, TTL: 3 * time.Minute}
}
func (s *Service) Read(ctx context.Context, id string) (model.QuotaSnapshot, error) {
	s.mu.Lock()
	q, ok := s.cache[id]
	s.mu.Unlock()
	if !ok && storage.ValidID(id) {
		raw, e := storage.ReadFile(filepath.Join(s.Store.Root, "quota", id+".json"), 1<<20)
		if e == nil {
			if json.Unmarshal(raw, &q) == nil && q.ProfileID == id && len(q.Windows) > 0 {
				ok = true
				s.mu.Lock()
				s.cache[id] = q
				s.mu.Unlock()
			}
		}
	}
	if !ok {
		if leg, err := loadLegacyQuotaCache(id); err == nil && len(leg.Windows) > 0 {
			q = leg
			ok = true
			s.mu.Lock()
			s.cache[id] = q
			s.mu.Unlock()
			if storage.ValidID(id) {
				if raw, err := json.Marshal(q); err == nil {
					_ = storage.AtomicWrite(ctx, filepath.Join(s.Store.Root, "quota", id+".json"), raw, 0600)
				}
			}
		}
	}
	if !ok {
		return model.QuotaSnapshot{ProfileID: id, Source: "codex-app-server", Status: "unavailable", Windows: []model.QuotaWindow{}}, nil
	}
	if time.Now().After(q.ExpiresAt) {
		q.Stale = true
		q.Status = "stale"
	}
	return q, ctx.Err()
}

type legacyQuotaSnapshot struct {
	ProfileID                   string   `json:"profileId"`
	Source                      string   `json:"source"`
	PlanType                    string   `json:"planType"`
	PrimaryUsedPercent          *float64 `json:"primaryUsedPercent"`
	PrimaryWindowDurationMins   int64    `json:"primaryWindowDurationMins"`
	SecondaryUsedPercent        *float64 `json:"secondaryUsedPercent"`
	SecondaryWindowDurationMins int64    `json:"secondaryWindowDurationMins"`
	RemainingPercent            *float64 `json:"remainingPercent"`
	ResetAt                     any      `json:"resetAt"`
	SecondaryResetAt            any      `json:"secondaryResetAt"`
	ResetCreditCount            *int     `json:"resetCreditCount"`
	ResetCreditExpiresAt        *string  `json:"resetCreditExpiresAt"`
	FetchedAt                   any      `json:"fetchedAt"`
}

func parseTimeVal(v any) *time.Time {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case float64:
		if val > 0 {
			t := time.Unix(int64(val), 0).Local()
			return &t
		}
	case string:
		if n, err := strconv.ParseInt(val, 10, 64); err == nil && n > 0 {
			t := time.Unix(n, 0).Local()
			return &t
		}
		if t, err := time.Parse(time.RFC3339, val); err == nil {
			loc := t.Local()
			return &loc
		}
	}
	return nil
}

func loadLegacyQuotaCache(id string) (model.QuotaSnapshot, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return model.QuotaSnapshot{}, os.ErrNotExist
	}
	dirs := []string{
		filepath.Join(home, ".config", "agent-workspace", "quota-cache"),
		filepath.Join(home, ".lam", "config", "quota-cache"),
		filepath.Join(home, ".lam", "quota-cache"),
	}

	cleanID := strings.TrimPrefix(id, "codex-")
	candidates := []string{
		id + ".json",
		strings.ToLower(id) + ".json",
		cleanID + ".json",
		strings.ToLower(cleanID) + ".json",
	}

	for _, dir := range dirs {
		for _, name := range candidates {
			p := filepath.Join(dir, name)
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			var leg legacyQuotaSnapshot
			if err := json.Unmarshal(data, &leg); err != nil {
				continue
			}
			q := model.QuotaSnapshot{
				ProfileID: id,
				Source:    leg.Source,
				Status:    "ready",
				PlanType:  leg.PlanType,
				Windows:   make([]model.QuotaWindow, 0, 2),
				FetchedAt: time.Now(),
				ExpiresAt: time.Now().Add(3 * time.Minute),
			}
			if q.Source == "" {
				q.Source = "codex-app-server"
			}

			// Primary Window: Session (5h)
			if leg.PrimaryUsedPercent != nil || leg.RemainingPercent != nil {
				rem := 100.0
				if leg.RemainingPercent != nil {
					rem = *leg.RemainingPercent
				} else if leg.PrimaryUsedPercent != nil {
					rem = 100.0 - *leg.PrimaryUsedPercent
				}
				winMins := leg.PrimaryWindowDurationMins
				if winMins == 0 {
					winMins = 300
				}
				q.Windows = append(q.Windows, model.QuotaWindow{
					ID:               "Session (5h)",
					UsedPercent:      leg.PrimaryUsedPercent,
					RemainingPercent: &rem,
					WindowMinutes:    winMins,
					ResetsAt:         parseTimeVal(leg.ResetAt),
				})
			}

			// Secondary Window: Weekly (7d)
			if leg.SecondaryUsedPercent != nil {
				rem := 100.0 - *leg.SecondaryUsedPercent
				winMins := leg.SecondaryWindowDurationMins
				if winMins == 0 {
					winMins = 10080
				}
				q.Windows = append(q.Windows, model.QuotaWindow{
					ID:               "Weekly (7d)",
					UsedPercent:      leg.SecondaryUsedPercent,
					RemainingPercent: &rem,
					WindowMinutes:    winMins,
					ResetsAt:         parseTimeVal(leg.SecondaryResetAt),
				})
			}

			// Credits
			if leg.ResetCreditCount != nil {
				var exp *time.Time
				if leg.ResetCreditExpiresAt != nil && *leg.ResetCreditExpiresAt != "" {
					if t, err := time.Parse(time.RFC3339, *leg.ResetCreditExpiresAt); err == nil {
						loc := t.Local()
						exp = &loc
					}
				}
				q.Credits = &model.Credits{
					AvailableCount: leg.ResetCreditCount,
					ExpiresAt:      exp,
				}
			}

			if len(q.Windows) > 0 {
				return q, nil
			}
		}
	}
	return model.QuotaSnapshot{}, os.ErrNotExist
}
func (s *Service) Refresh(ctx context.Context, id string) (model.QuotaSnapshot, error) {
	s.mu.Lock()
	if pending, ok := s.inflight[id]; ok {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return model.QuotaSnapshot{}, ctx.Err()
		case <-pending.done:
			return pending.value, pending.err
		}
	}
	f := &flight{done: make(chan struct{})}
	s.inflight[id] = f
	s.mu.Unlock()
	q, e := s.fetch(ctx, id)
	if e != nil {
		old, _ := s.Read(context.Background(), id)
		if !old.FetchedAt.IsZero() {
			q = old
			q.Stale = true
			q.Status = "stale"
		} else {
			q = model.QuotaSnapshot{ProfileID: id, Source: "codex-app-server", Status: "unavailable", Windows: []model.QuotaWindow{}}
		}
		q.ErrorCode = fault.Code(e)
	}
	s.mu.Lock()
	s.cache[id] = q
	f.value = q
	f.err = e
	delete(s.inflight, id)
	close(f.done)
	s.mu.Unlock()
	if e == nil && storage.ValidID(id) {
		raw, _ := json.Marshal(q)
		if err := storage.AtomicWrite(ctx, filepath.Join(s.Store.Root, "quota", id+".json"), raw, 0600); err != nil {
			return q, err
		}
	}
	return q, e
}
func (s *Service) fetch(ctx context.Context, id string) (model.QuotaSnapshot, error) {
	st, e := s.Store.Read(ctx)
	if e != nil {
		return model.QuotaSnapshot{}, e
	}
	p, ok := st.Profiles[id]
	if !ok && s.LookupProfile != nil {
		if found, err := s.LookupProfile(ctx, id); err == nil {
			p = found
			ok = true
		}
	}
	if !ok {
		return model.QuotaSnapshot{}, fault.New("PROFILE_NOT_FOUND", "Profile not found")
	}
	if s.PrepareProfile != nil {
		p, e = s.PrepareProfile(ctx, p)
		if e != nil {
			return model.QuotaSnapshot{}, e
		}
	}
	if p.AuthMode == "pat" && st.ActivePAT != p.ID {
		return model.QuotaSnapshot{}, fault.New("PAT_INACTIVE", "Only the active PAT can query a shared home")
	}
	if s.Codex == nil {
		return model.QuotaSnapshot{}, fault.New("CODEX_UNAVAILABLE", "Codex App Server is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var raw json.RawMessage
	if e = s.Codex.Call(ctx, p, "account/rateLimits/read", nil, &raw); e != nil {
		return model.QuotaSnapshot{}, fault.Wrap("QUOTA_UNAVAILABLE", "Could not refresh account limits", e)
	}
	return ParseCodex(id, raw, time.Now().UTC(), s.TTL)
}
func (s *Service) AccountUsage(ctx context.Context, id string) (json.RawMessage, error) {
	st, e := s.Store.Read(ctx)
	if e != nil {
		return nil, e
	}
	p, ok := st.Profiles[id]
	if !ok && s.LookupProfile != nil {
		if found, err := s.LookupProfile(ctx, id); err == nil {
			p = found
			ok = true
		}
	}
	if !ok {
		return nil, fault.New("PROFILE_NOT_FOUND", "Profile not found")
	}
	if s.PrepareProfile != nil {
		p, e = s.PrepareProfile(ctx, p)
		if e != nil {
			return nil, e
		}
	}
	if p.AuthMode == "pat" && st.ActivePAT != id {
		return nil, fault.New("PAT_INACTIVE", "The PAT is not active")
	}
	if s.Codex == nil {
		return nil, fault.New("CODEX_UNAVAILABLE", "Codex App Server is unavailable")
	}
	var raw json.RawMessage
	e = s.Codex.Call(ctx, p, "account/usage/read", nil, &raw)
	if e != nil {
		return nil, fault.Wrap("ACCOUNT_USAGE_UNAVAILABLE", "Account usage could not be read", e)
	}
	if len(raw) > 8<<20 {
		return nil, fault.New("RESPONSE_LIMIT", "Account usage response exceeds maximum")
	}
	return raw, nil
}

type resetOperation struct {
	ID, ProfileID, CreditID, Outcome string
	CreatedAt                        time.Time
}

func (s *Service) PrepareReset(ctx context.Context, profileID, creditID string) (string, error) {
	st, e := s.Store.Read(ctx)
	if e != nil {
		return "", e
	}
	_, ok := st.Profiles[profileID]
	if !ok && s.LookupProfile != nil {
		if _, err := s.LookupProfile(ctx, profileID); err == nil {
			ok = true
		}
	}
	if !ok {
		return "", fault.New("PROFILE_NOT_FOUND", "Profile not found")
	}
	op := resetOperation{ID: storage.NewID(), ProfileID: profileID, CreditID: creditID, CreatedAt: time.Now().UTC()}
	raw, _ := json.Marshal(op)
	e = storage.AtomicWrite(ctx, filepath.Join(s.Store.Root, "resets", op.ID+".json"), raw, 0600)
	return op.ID, e
}
func (s *Service) ConsumeReset(ctx context.Context, id string, confirm bool) (string, error) {
	if !confirm {
		return "", fault.New("CONFIRMATION_REQUIRED", "Consuming a reset requires confirmation")
	}
	if !storage.ValidID(id) {
		return "", fault.New("OPERATION_INVALID", "Invalid reset operation")
	}
	unlock, e := storage.Lock(ctx, filepath.Join(s.Store.Root, "reset.lock"))
	if e != nil {
		return "", e
	}
	defer unlock()
	path := filepath.Join(s.Store.Root, "resets", id+".json")
	raw, e := storage.ReadFile(path, 1<<20)
	if e != nil {
		return "", e
	}
	var op resetOperation
	if json.Unmarshal(raw, &op) != nil || op.ID != id {
		return "", fault.New("OPERATION_INVALID", "Invalid reset operation")
	}
	st, e := s.Store.Read(ctx)
	if e != nil {
		return "", e
	}
	p, ok := st.Profiles[op.ProfileID]
	if !ok && s.LookupProfile != nil {
		if found, err := s.LookupProfile(ctx, op.ProfileID); err == nil {
			p = found
			ok = true
		}
	}
	if !ok {
		return "", fault.New("PROFILE_NOT_FOUND", "Profile not found")
	}
	if s.PrepareProfile != nil {
		p, e = s.PrepareProfile(ctx, p)
		if e != nil {
			return "", e
		}
	}
	if p.AuthMode == "pat" && st.ActivePAT != p.ID {
		return "", fault.New("PAT_INACTIVE", "The PAT is not active")
	}
	if s.Codex == nil {
		return "", fault.New("CODEX_UNAVAILABLE", "Codex App Server is unavailable")
	}
	if op.Outcome == "" {
		params := map[string]any{"idempotencyKey": op.ID}
		if op.CreditID != "" {
			params["creditId"] = op.CreditID
		}
		var out struct {
			Outcome string `json:"outcome"`
		}
		if e = s.Codex.Call(ctx, p, "account/rateLimitResetCredit/consume", params, &out); e != nil {
			return "", fault.Wrap("RESET_UNAVAILABLE", "Reset could not be confirmed; retry the same operation", e)
		}
		switch out.Outcome {
		case "reset", "alreadyRedeemed", "nothingToReset", "noCredit":
		default:
			return "", fault.New("RESET_INVALID", "Unexpected reset result")
		}
		op.Outcome = out.Outcome
		raw, _ = json.Marshal(op)
		if e = storage.AtomicWrite(ctx, path, raw, 0600); e != nil {
			return op.Outcome, e
		}
	}
	_, e = s.Refresh(ctx, p.ID)
	return op.Outcome, e
}

// PendingReset allows an interrupted UI flow to retry the original idempotency key.
func (s *Service) PendingReset(ctx context.Context, profileID string) ([]string, error) {
	files, e := os.ReadDir(filepath.Join(s.Store.Root, "resets"))
	if errors.Is(e, os.ErrNotExist) {
		return []string{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []string{}
	for _, f := range files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		raw, e := storage.ReadFile(filepath.Join(s.Store.Root, "resets", f.Name()), 1<<20)
		if e != nil {
			continue
		}
		var op resetOperation
		if json.Unmarshal(raw, &op) == nil && op.ProfileID == profileID && op.Outcome == "" {
			out = append(out, op.ID)
		}
	}
	return out, nil
}
