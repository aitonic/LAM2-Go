package account

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
)

func NormalizePAT(session []byte, pat, expires string) ([]byte, error) {
	if strings.TrimSpace(pat) == "" || strings.ContainsAny(pat, "\r\n\x00") {
		return nil, fault.New("AUTH_IMPORT_INVALID", "Invalid personal access token")
	}
	if _, _, e := NormalizeAuth(session); e != nil {
		return nil, e
	}
	if expires != "" {
		if _, e := time.Parse(time.RFC3339, expires); e != nil {
			return nil, fault.New("INVALID_EXPIRATION", "Use an RFC3339 expiry")
		}
	}
	return json.Marshal(map[string]any{"auth_json": json.RawMessage(session), "personal_access_token": pat, "token_expiration": expires})
}
func normalizeImport(raw []byte) (normal, secondary []byte, email string, expiration *time.Time, err error) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		err = fault.New("AUTH_IMPORT_INVALID", "Invalid JSON credentials")
		return
	}
	var pat, expiry string
	_ = json.Unmarshal(obj["personal_access_token"], &pat)
	_ = json.Unmarshal(obj["token_expiration"], &expiry)
	if expiry != "" {
		var t time.Time
		t, err = time.Parse(time.RFC3339, expiry)
		if err != nil {
			err = fault.New("INVALID_EXPIRATION", "Use an RFC3339 expiry")
			return
		}
		expiration = &t
	}
	if payload := obj["auth_json"]; len(payload) > 0 {
		if pat != "" {
			secondary, email, err = NormalizeAuth(payload)
			if err != nil {
				return
			}
			r, _ := json.Marshal(map[string]any{"personal_access_token": pat})
			normal, _, err = NormalizeAuth(r)
			return
		}
		normal, email, err = NormalizeAuth(payload)
		return
	}
	normal, email, err = NormalizeAuth(raw)
	return
}
func (s *Service) readCredential(ctx context.Context, p model.Profile, secondary bool) ([]byte, error) {
	id, path := p.CredentialID, filepath.Join(p.Home, "auth.json")
	if secondary {
		id = p.QuotaCredentialID
		path = filepath.Join(p.Home, "auth-f.json")
	}
	// Native CLI refreshes the isolated auth file. Prefer that current value
	// over a stale import snapshot; shared PAT homes must never leak another identity.
	if p.Home != s.SharedHome {
		if b, e := storage.ReadFile(path, 4<<20); e == nil {
			return b, nil
		} else if !errors.Is(e, os.ErrNotExist) {
			return nil, e
		}
	}
	if id != "" {
		if s.Vault == nil {
			return nil, fault.New("KEYCHAIN_UNAVAILABLE", "Credential vault unavailable")
		}
		return s.Vault.Get(ctx, id)
	}
	if p.AuthMode == "pat" && p.Home == s.SharedHome {
		return nil, os.ErrNotExist
	}
	return storage.ReadFile(path, 4<<20)
}

// QuotaProfile gives every identity a private App Server auth home, even while
// another PAT is active in ~/.codex. Runtime PAT and quota Session stay separate.
func (s *Service) QuotaProfile(ctx context.Context, p model.Profile) (model.Profile, error) {
	raw, e := s.readCredential(ctx, p, true)
	if errors.Is(e, os.ErrNotExist) {
		raw, e = s.readCredential(ctx, p, false)
	}
	if e != nil {
		return p, e
	}
	defer clear(raw)
	// Ordinary isolated profiles can use their actual home without copying.
	if p.AuthMode != "pat" && p.QuotaCredentialID == "" {
		if _, e := os.Stat(filepath.Join(p.Home, "auth-f.json")); os.IsNotExist(e) {
			return p, nil
		}
	}
	home := filepath.Join(s.Store.Root, "quota-homes", p.ID)
	if e = storage.EnsurePrivateDir(home); e != nil {
		return p, e
	}
	path := filepath.Join(home, "auth.json")
	old, _ := storage.FileHash(path)
	if old != storage.Hash(raw) {
		if s.OnAuthChanged != nil {
			s.OnAuthChanged(home)
		}
		if e = storage.AtomicWrite(ctx, path, raw, 0600); e != nil {
			return p, e
		}
	}
	p.Home = home
	p.AuthMode = "quota"
	return p, nil
}
func (s *Service) UpdateQuotaAuth(ctx context.Context, id string, raw []byte, confirmed bool) error {
	if !confirmed {
		return fault.New("CONFIRMATION_REQUIRED", "Confirm replacement of the selected quota Session credential")
	}
	normal, _, e := NormalizeAuth(raw)
	if e != nil {
		return e
	}
	defer clear(normal)
	st, e := s.Store.Read(ctx)
	if e != nil {
		return e
	}
	p, ok := st.Profiles[id]
	if !ok {
		return fault.New("PROFILE_NOT_FOUND", "Unknown profile")
	}
	if s.Vault == nil {
		return fault.New("KEYCHAIN_UNAVAILABLE", "Credential vault unavailable")
	}
	credential := "quota-" + storage.NewID()
	if e = s.Vault.Put(ctx, credential, normal); e != nil {
		return e
	}
	old := p.QuotaCredentialID
	var changes []storage.Change
	if p.Home != s.SharedHome {
		path := filepath.Join(p.Home, "auth-f.json")
		before, err := storage.FileHash(path)
		if err != nil {
			_ = s.Vault.Delete(context.Background(), credential)
			return err
		}
		changes = append(changes, storage.Change{Path: path, Before: before, Data: normal})
	}
	_, e = s.Store.Commit(ctx, st.Revision, func(st *model.State) ([]storage.Change, error) {
		p.QuotaCredentialID = credential
		st.Profiles[id] = p
		return changes, nil
	})
	if e != nil {
		_ = s.Vault.Delete(context.Background(), credential)
		return e
	}
	if old != "" {
		_ = s.Vault.Delete(context.Background(), old)
	}
	if s.OnAuthChanged != nil {
		s.OnAuthChanged(filepath.Join(s.Store.Root, "quota-homes", id))
	}
	return nil
}

type ExpirationStatus struct {
	ProfileID string
	ExpiresAt *time.Time
	Days      *int
	Status    string
}

func (s *Service) Expiration(ctx context.Context, id string) (ExpirationStatus, error) {
	p, e := s.Get(ctx, id)
	if e != nil {
		return ExpirationStatus{}, e
	}
	out := ExpirationStatus{ProfileID: id, ExpiresAt: p.TokenExpiration, Status: "unknown"}
	if p.TokenExpiration != nil {
		days := int(time.Until(*p.TokenExpiration).Hours() / 24)
		out.Days = &days
		out.Status = "ok"
		if p.TokenExpiration.Before(time.Now()) {
			out.Status = "expired"
		} else if days <= 7 {
			out.Status = "critical"
		} else if days <= 30 {
			out.Status = "warning"
		}
	}
	return out, nil
}

// ProviderAPIKey resolves only the selected profile's native API key. OAuth
// access tokens, PAT authorization and the secondary quota credential are not
// interchangeable with OPENAI_API_KEY (legacy CodexProfile credential source).
func (s *Service) ProviderAPIKey(ctx context.Context, id string) (string, error) {
	p, e := s.Get(ctx, id)
	if e != nil {
		return "", e
	}
	raw, e := s.readCredential(ctx, p, false)
	if e != nil {
		return "", fault.Wrap("CODEX_API_KEY_MISSING", "The selected profile has no API-key credential", e)
	}
	defer clear(raw)
	var auth struct {
		Key string `json:"OPENAI_API_KEY"`
	}
	if json.Unmarshal(raw, &auth) != nil || strings.TrimSpace(auth.Key) == "" || strings.ContainsAny(auth.Key, "\r\n\x00") {
		return "", fault.New("CODEX_API_KEY_MISSING", "The selected profile does not use a native API key")
	}
	return auth.Key, nil
}
