package account

import (
	"context"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
)

// RuntimeProfile separates the identity's data home from the global PAT launch
// home. Listing, handoff and private quota reads continue to use the stored home.
func (s *Service) RuntimeProfile(ctx context.Context, id string) (model.Profile, error) {
	st, e := s.Store.Read(ctx)
	if e != nil {
		return model.Profile{}, e
	}
	p, ok := st.Profiles[id]
	if !ok {
		return p, fault.New("PROFILE_NOT_FOUND", "Profile not found")
	}
	if _, bound := st.Bindings[id]; bound {
		return p, nil
	}
	legacyShared := p.AuthMode == "pat" && p.Home == s.SharedHome
	sharedMode := st.Settings.AccountMode == "pat" && p.Home != s.SharedHome && id != "main"
	if legacyShared || sharedMode {
		if st.ActivePAT != id {
			return p, fault.New("PAT_INACTIVE", "Activate this account before using the shared Codex home")
		}
		p.Home = s.SharedHome
		p.AuthMode = "pat_runtime"
	}
	return p, nil
}
