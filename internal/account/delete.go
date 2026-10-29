package account

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
)

// Delete removes the active account, its wrapper and protected credential
// snapshots. LAM-owned homes are moved to a recoverable archive, never erased.
// User-managed external homes and shared ~/.codex are never recursively removed.
func (s *Service) Delete(ctx context.Context, id string, expected uint64, confirmed bool) error {
	if !confirmed {
		return fault.New("CONFIRMATION_REQUIRED", "Confirm removal of the account and its managed home")
	}
	unlock, e := storage.Lock(ctx, filepath.Join(s.Store.Root, "account-delete.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	st, e := s.Store.Read(ctx)
	if e != nil {
		return e
	}
	p, ok := st.Profiles[id]
	if !ok {
		return fault.New("PROFILE_NOT_FOUND", "Profile not found")
	}
	if st.Revision != expected {
		return fault.New("REVISION_CONFLICT", "Refresh before deleting")
	}
	if id == "main" {
		return fault.New("MAIN_ACCOUNT_PROTECTED", "The default Codex account cannot be deleted")
	}
	if _, bound := st.Bindings[id]; bound {
		return fault.New("PROFILE_BOUND", "Detach the provider first")
	}
	if st.ActivePAT == id {
		return fault.New("PAT_ACTIVE", "Restore shared authentication before deleting this PAT")
	}
	if s.OnAuthChanged != nil {
		s.OnAuthChanged(p.Home)
		s.OnAuthChanged(filepath.Join(s.Store.Root, "quota-homes", id))
	}
	archive := ""
	if p.Managed && p.Home != s.SharedHome {
		if !storage.Within(p.Home, []string{filepath.Join(s.Store.Root, "profiles")}) {
			return fault.New("PATH_UNSAFE", "Managed home is outside the application profile directory")
		}
		raw, e := storage.ReadFile(filepath.Join(p.Home, ".managed-by-lam2.json"), 1<<20)
		if e != nil {
			return e
		}
		var marker struct {
			ProfileID string `json:"profileId"`
		}
		if json.Unmarshal(raw, &marker) != nil || marker.ProfileID != id {
			return fault.New("PROFILE_OWNERSHIP_INVALID", "Managed profile marker does not match")
		}
		if e = storage.RejectSymlinks(p.Home); e != nil {
			return e
		}
		archive = filepath.Join(s.Store.Root, "removed-profiles", id+"-"+storage.NewID())
		if e = storage.EnsurePrivateDir(filepath.Dir(archive)); e != nil {
			return e
		}
		manifest, _ := json.Marshal(map[string]any{"profile": p, "archive": archive, "removedAt": time.Now().UTC()})
		if e = storage.AtomicWrite(ctx, archive+".json", manifest, 0600); e != nil {
			return e
		}
		if e = os.Rename(p.Home, archive); e != nil {
			return e
		}
	}
	_, e = s.Store.Update(ctx, expected, func(v *model.State) error { delete(v.Profiles, id); return nil })
	if e != nil {
		if archive != "" {
			if _, stat := os.Lstat(p.Home); os.IsNotExist(stat) {
				return errors.Join(e, os.Rename(archive, p.Home))
			}
		}
		return e
	}
	for _, credential := range []string{p.CredentialID, p.QuotaCredentialID} {
		if credential != "" && s.Vault != nil {
			if e = s.Vault.Delete(ctx, credential); e != nil {
				return e
			}
		}
	}
	wrapper := filepath.Join(s.Store.Root, "bin", "lam-"+id)
	if e = storage.RejectSymlinks(wrapper); e != nil {
		return e
	}
	if e = os.Remove(wrapper); e != nil && !os.IsNotExist(e) {
		return e
	}
	quotaHome := filepath.Join(s.Store.Root, "quota-homes", id)
	if e = storage.RejectSymlinks(quotaHome); e != nil {
		return e
	}
	return os.RemoveAll(quotaHome)
}
