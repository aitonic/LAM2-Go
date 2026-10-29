package migration

import (
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
)

// Preserve the exact association and selected model, but do not replay old Rust
// auth helpers or rewrite externally owned Codex files during a data import.
// The Providers page offers an explicit re-attach preview using the new Go exe.
func (s *Service) bindingMetadata(p *Plan) error {
	var env struct {
		SchemaVersion int `json:"schemaVersion"`
		SchemaSnake   int `json:"schema_version"`
		Bindings      []struct {
			ProfileID, ProviderID, SelectedModel, RouteKind string
			ConfigProjection                                struct{ AppliedHash string }
		}
	}
	found, e := readMetadata(p, "provider-hub/bindings.json", &env)
	if e != nil || !found {
		return e
	}
	v := env.SchemaVersion
	if v == 0 {
		v = env.SchemaSnake
	}
	if v != 1 {
		return fault.New("SCHEMA_UNSUPPORTED", "Unknown binding schema")
	}
	seen := map[string]bool{}
	for _, b := range env.Bindings {
		if !storage.ValidID(b.ProfileID) || !storage.ValidID(b.ProviderID) || b.SelectedModel == "" || seen[b.ProfileID] || (b.RouteKind != "direct" && b.RouteKind != "gateway") {
			return fault.New("MIGRATION_INVALID", "Invalid legacy binding")
		}
		seen[b.ProfileID] = true
		p.PendingBindings = append(p.PendingBindings, model.PendingBinding{ProfileID: b.ProfileID, ProviderID: b.ProviderID, Model: b.SelectedModel, RouteKind: b.RouteKind, LegacyConfigHash: b.ConfigProjection.AppliedHash, MigrationID: p.ID})
	}
	return nil
}
