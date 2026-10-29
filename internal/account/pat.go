package account

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
)

type patFile struct {
	Missing     bool   `json:"missing"`
	Baseline    string `json:"baseline"`
	AppliedHash string `json:"appliedHash"`
	BeforeHash  string `json:"beforeHash"`
}

// Primary fields remain compatible with recovery markers from the development build.
type patMarker struct {
	Missing     bool     `json:"missing"`
	Baseline    string   `json:"baseline"`
	AppliedHash string   `json:"appliedHash"`
	BeforeHash  string   `json:"beforeHash,omitempty"`
	Secondary   *patFile `json:"secondary,omitempty"`
	Profile     string   `json:"profile"`
	Stage       string   `json:"stage"`
}

func optionalAuth(path string) ([]byte, error) {
	b, e := storage.ReadFile(path, 4<<20)
	if errors.Is(e, fs.ErrNotExist) {
		return nil, nil
	}
	return b, e
}
func authHash(b []byte) string {
	if b == nil {
		return ""
	}
	return storage.Hash(b)
}
func replaceAuth(ctx context.Context, path string, b []byte) error {
	if b != nil {
		return storage.AtomicWrite(ctx, path, b, 0600)
	}
	if e := storage.RejectSymlinks(path); e != nil {
		return e
	}
	e := os.Remove(path)
	if errors.Is(e, fs.ErrNotExist) {
		return nil
	}
	return e
}
func (s *Service) ActivatePAT(ctx context.Context, id string, confirmed bool) error {
	if !confirmed {
		return fault.New("CONFIRMATION_REQUIRED", "PAT activation replaces shared Codex authentication")
	}
	if s.Vault == nil {
		return fault.New("KEYCHAIN_UNAVAILABLE", "Credential vault unavailable")
	}
	unlock, e := storage.Lock(ctx, filepath.Join(s.Store.Root, "pat.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	st, e := s.Store.Read(ctx)
	if e != nil {
		return e
	}
	p, ok := st.Profiles[id]
	if !ok || id == "main" || (p.Home == s.SharedHome && p.AuthMode != "pat") {
		return fault.New("PROFILE_NOT_FOUND", "Select a non-main account")
	}
	if _, bound := st.Bindings[id]; bound {
		return fault.New("PAT_PROVIDER_BOUND", "API-provider accounts use their isolated launch configuration")
	}
	plain, e := s.readCredential(ctx, p, false)
	if e != nil {
		return e
	}
	defer clear(plain)
	secondary, e := s.readCredential(ctx, p, true)
	if errors.Is(e, fs.ErrNotExist) {
		secondary = nil
	} else if e != nil {
		return e
	}
	defer clear(secondary)
	if e = storage.EnsurePrivateDir(s.SharedHome); e != nil {
		return e
	}
	path, side := filepath.Join(s.SharedHome, "auth.json"), filepath.Join(s.SharedHome, "auth-f.json")
	old, e := optionalAuth(path)
	if e != nil {
		return e
	}
	defer clear(old)
	oldSide, e := optionalAuth(side)
	if e != nil {
		return e
	}
	defer clear(oldSide)
	mp := filepath.Join(s.Store.Root, "pat-state.json")
	previous, e := optionalAuth(mp)
	if e != nil {
		return e
	}
	var m patMarker
	created := []string{}
	if previous == nil {
		m = patMarker{Missing: old == nil, Baseline: "pat-baseline-" + storage.NewID()}
		if old != nil {
			if e = s.Vault.Put(ctx, m.Baseline, old); e != nil {
				return e
			}
			created = append(created, m.Baseline)
		}
	} else {
		if json.Unmarshal(previous, &m) != nil || m.Stage != "active" {
			return fault.New("PAT_RECOVERY_REQUIRED", "Restore an interrupted PAT switch before switching again")
		}
		if authHash(old) != m.AppliedHash || (m.Secondary != nil && authHash(oldSide) != m.Secondary.AppliedHash) {
			return fault.New("AUTH_CONFLICT", "Shared authentication changed externally")
		}
	}
	if m.Secondary == nil {
		m.Secondary = &patFile{Missing: oldSide == nil, Baseline: "pat-quota-baseline-" + storage.NewID()}
		if oldSide != nil {
			if e = s.Vault.Put(ctx, m.Secondary.Baseline, oldSide); e != nil {
				for _, id := range created {
					_ = s.Vault.Delete(context.Background(), id)
				}
				return e
			}
			created = append(created, m.Secondary.Baseline)
		}
	}
	committed := false
	defer func() {
		if !committed {
			for _, id := range created {
				_ = s.Vault.Delete(context.Background(), id)
			}
		}
	}()
	m.AppliedHash, m.BeforeHash = authHash(plain), authHash(old)
	m.Secondary.AppliedHash, m.Secondary.BeforeHash = authHash(secondary), authHash(oldSide)
	m.Profile, m.Stage = id, "prepared"
	raw, _ := json.Marshal(m)
	if e = storage.AtomicWrite(ctx, mp, raw, 0600); e != nil {
		return e
	}
	rollback := func() error {
		return errors.Join(replaceAuth(context.Background(), path, old), replaceAuth(context.Background(), side, oldSide), replaceAuth(context.Background(), mp, previous))
	}
	if s.OnAuthChanged != nil {
		s.OnAuthChanged(s.SharedHome)
	}
	if e = replaceAuth(ctx, path, plain); e != nil {
		return errors.Join(e, rollback())
	}
	if e = replaceAuth(ctx, side, secondary); e != nil {
		return errors.Join(e, rollback())
	}
	_, e = s.Store.Update(ctx, st.Revision, func(v *model.State) error { v.ActivePAT = id; recordPATSwitch(v, id, time.Now().UTC()); return nil })
	if e != nil {
		return errors.Join(e, rollback())
	}
	committed = true
	m.Stage = "active"
	raw, _ = json.Marshal(m)
	return storage.AtomicWrite(ctx, mp, raw, 0600)
}
func (s *Service) RestorePAT(ctx context.Context, confirmed bool) error {
	if !confirmed {
		return fault.New("CONFIRMATION_REQUIRED", "Restoring changes shared Codex authentication")
	}
	if s.Vault == nil {
		return fault.New("KEYCHAIN_UNAVAILABLE", "Credential vault unavailable")
	}
	unlock, e := storage.Lock(ctx, filepath.Join(s.Store.Root, "pat.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	mp := filepath.Join(s.Store.Root, "pat-state.json")
	raw, e := storage.ReadFile(mp, 1<<20)
	if e != nil {
		return fault.Wrap("PAT_RECOVERY_MISSING", "No PAT baseline is available", e)
	}
	var m patMarker
	if json.Unmarshal(raw, &m) != nil {
		return fault.New("PAT_RECOVERY_REQUIRED", "Invalid PAT recovery marker")
	}
	path, side := filepath.Join(s.SharedHome, "auth.json"), filepath.Join(s.SharedHome, "auth-f.json")
	applied, e := optionalAuth(path)
	if e != nil {
		return e
	}
	defer clear(applied)
	appliedSide, e := optionalAuth(side)
	if e != nil {
		return e
	}
	defer clear(appliedSide)
	primary := patFile{Missing: m.Missing, Baseline: m.Baseline, AppliedHash: m.AppliedHash, BeforeHash: m.BeforeHash}
	baseline := func(f patFile) ([]byte, error) {
		if f.Missing {
			return nil, nil
		}
		return s.Vault.Get(ctx, f.Baseline)
	}
	original, e := baseline(primary)
	if e != nil {
		return e
	}
	defer clear(original)
	var origSide []byte
	if m.Secondary != nil {
		origSide, e = baseline(*m.Secondary)
		if e != nil {
			return e
		}
		defer clear(origSide)
	}
	compatible := func(current []byte, f patFile, base []byte) bool {
		h := authHash(current)
		return h == f.AppliedHash || (m.Stage == "prepared" && h == f.BeforeHash) || (m.Stage == "restoring" && h == authHash(base))
	}
	if !compatible(applied, primary, original) || (m.Secondary != nil && !compatible(appliedSide, *m.Secondary, origSide)) {
		return fault.New("AUTH_CONFLICT", "Shared authentication changed externally; refusing to overwrite it")
	}
	st, e := s.Store.Read(ctx)
	if e != nil {
		return e
	}
	m.Stage = "restoring"
	prepared, _ := json.Marshal(m)
	if e = storage.AtomicWrite(ctx, mp, prepared, 0600); e != nil {
		return e
	}
	rollback := func() error {
		return errors.Join(replaceAuth(context.Background(), path, applied), replaceAuth(context.Background(), side, appliedSide), storage.AtomicWrite(context.Background(), mp, raw, 0600))
	}
	if s.OnAuthChanged != nil {
		s.OnAuthChanged(s.SharedHome)
	}
	if e = replaceAuth(ctx, path, original); e != nil {
		return errors.Join(e, rollback())
	}
	if m.Secondary != nil {
		if e = replaceAuth(ctx, side, origSide); e != nil {
			return errors.Join(e, rollback())
		}
	}
	_, e = s.Store.Update(ctx, st.Revision, func(v *model.State) error { v.ActivePAT = ""; recordPATSwitch(v, "", time.Now().UTC()); return nil })
	if e != nil {
		return errors.Join(e, rollback())
	}
	if e = os.Remove(mp); e != nil {
		return e
	}
	if !m.Missing {
		_ = s.Vault.Delete(context.Background(), m.Baseline)
	}
	if m.Secondary != nil && !m.Secondary.Missing {
		_ = s.Vault.Delete(context.Background(), m.Secondary.Baseline)
	}
	return nil
}

func recordPATSwitch(st *model.State, id string, now time.Time) {
	for i := range st.PATTimeline {
		if st.PATTimeline[i].EndedAt == nil {
			t := now
			st.PATTimeline[i].EndedAt = &t
		}
	}
	if id != "" {
		p := st.Profiles[id]
		st.PATTimeline = append(st.PATTimeline, model.PATPeriod{ProfileID: id, Label: p.Name, StartedAt: now})
	}
}
