package gateway

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/provider"
	"lam.local/lam2/internal/storage"
)

type StateResolver struct {
	Store     *storage.Store
	Secrets   storage.Secrets
	Providers *provider.Service
}

func (r *StateResolver) Resolve(ctx context.Context, token string) (Snapshot, error) {
	var out Snapshot
	if len(token) < 16 || len(token) > 512 {
		return out, fault.New("GATEWAY_UNAUTHORIZED", "Invalid binding token")
	}
	st, e := r.Store.Read(ctx)
	if e != nil {
		return out, e
	}
	wanted := sha256.Sum256([]byte(token))
	var found *model.Binding
	for _, binding := range st.Bindings {
		if binding.RouteKind != "gateway" {
			continue
		}
		key, e := r.Secrets.Get(ctx, binding.TokenRef)
		if e != nil {
			continue
		}
		got := sha256.Sum256([]byte(key))
		if subtle.ConstantTimeCompare(wanted[:], got[:]) == 1 {
			b := binding
			found = &b
		}
	}
	if found == nil {
		return out, fault.New("GATEWAY_UNAUTHORIZED", "Binding token is not active")
	}
	p, ok := st.Providers[found.ProviderID]
	if !ok {
		return out, fault.New("PROVIDER_NOT_FOUND", "Bound provider no longer exists")
	}
	if _, ok = st.Profiles[found.ProfileID]; !ok {
		return out, fault.New("PROFILE_NOT_FOUND", "Bound profile no longer exists")
	}
	headers, e := r.Providers.Headers(ctx, p)
	if e != nil {
		return out, e
	}
	out = Snapshot{Binding: *found, Provider: p, Headers: headers.Clone(), Proxy: st.Profiles[found.ProfileID].Proxy}
	return out, nil
}
