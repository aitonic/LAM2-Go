package provider

import (
	"context"
	"errors"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
	"time"
)

// savePreparedProvider updates the attached model settings before committing
// metadata. Each external config write has its own before/after recovery record.
// The final store write commits all binding hashes and the provider together.
func (s *Service) savePreparedProvider(ctx context.Context, st model.State, next, old model.Provider) (committed bool, err error) {
	var affected []model.Binding
	for _, b := range st.Bindings {
		if b.ProviderID == next.ID {
			affected = append(affected, b)
		}
	}
	if len(affected) == 0 {
		_, err = s.Store.Update(ctx, st.Revision, func(v *model.State) error { v.Providers[next.ID] = next; return nil })
		return err == nil, err
	}
	if s.Config == nil {
		return false, fault.New("CONFIG_UNAVAILABLE", "Codex configuration editor is unavailable")
	}
	catalog, e := model.ModelsCatalog(next.Models)
	if e != nil {
		return false, e
	}
	records := []bindJournal{}
	updated := map[string]model.Binding{}
	createdTokens := []model.CredentialRef{}
	defer func() {
		if committed {
			return
		}
		for _, ref := range createdTokens {
			if s.Secrets != nil {
				_ = s.Secrets.Delete(context.Background(), ref)
			}
		}
		var recovery []error
		for i := len(records) - 1; i >= 0; i-- {
			j := records[i]
			p := st.Profiles[j.ProfileID]
			if e := s.rollback(context.Background(), j, p); e != nil {
				recovery = append(recovery, e)
			} else {
				s.removeCatalog(model.Binding{CatalogPath: j.CatalogPath, CatalogHash: j.CatalogHash})
			}
		}
		if len(recovery) > 0 {
			err = fault.Wrap("CONFIG_RECOVERY_REQUIRED", "A failed provider update needs recovery; original backups were retained", errors.Join(recovery...))
		}
	}()
	for _, b := range affected {
		p, ok := st.Profiles[b.ProfileID]
		if !ok {
			return false, fault.New("PROFILE_NOT_FOUND", "Bound profile missing")
		}
		path := filepath.Join(p.Home, "config.toml")
		hash, e := storage.FileHash(path)
		if e != nil {
			return false, e
		}
		if hash != b.ConfigAfterHash {
			return false, fault.New("CONFIG_CONFLICT", "Bound configuration changed outside LAM")
		}
		before, e := storage.ReadFile(path, 4<<20)
		if e != nil {
			return false, e
		}
		id := storage.NewID()
		j := bindJournal{ID: id, ProfileID: p.ID, BindingID: b.ID, ProviderID: next.ID, ProviderRevision: next.Revision, Stage: "prepared", BeforeHash: hash, Backup: filepath.Join(s.Store.Root, "config-backups", id+".toml"), CreatedAt: time.Now().UTC(), CatalogPath: filepath.Join(s.Store.Root, "catalogs", b.ID+"-"+id+".json"), CatalogHash: storage.Hash(catalog)}
		if e = storage.AtomicWrite(ctx, j.Backup, before, 0600); e != nil {
			return false, e
		}
		if e = storage.AtomicWrite(ctx, j.CatalogPath, catalog, 0600); e != nil {
			return false, e
		}
		desired := "direct"
		if next.Protocol == "chat_completions" || next.Codex.RouteViaGateway {
			desired = "gateway"
		}
		b.Generation++
		if desired != b.RouteKind {
			b.RouteKind = desired
			if desired == "gateway" {
				if s.Gateway == nil || s.Secrets == nil {
					return false, fault.New("GATEWAY_UNAVAILABLE", "Gateway and secret store are required")
				}
				b.TokenRef = model.CredentialRef{Kind: "keychain", Service: "lam2.gateway", Account: b.ID + "/" + id, Version: b.Generation}
				if e = s.Secrets.Set(ctx, b.TokenRef, storage.NewID()+storage.NewID()); e != nil {
					return false, e
				}
				createdTokens = append(createdTokens, b.TokenRef)
				j.NewTokenRef = b.TokenRef
				b.GatewayURL, e = s.Gateway.Ensure(ctx, b)
				if e != nil {
					return false, e
				}
			} else {
				b.TokenRef = model.CredentialRef{}
				b.GatewayURL = ""
			}
		}
		b.CatalogPath = j.CatalogPath
		b.CatalogHash = j.CatalogHash
		if e = s.writeJournal(ctx, j); e != nil {
			_ = os.Remove(j.CatalogPath)
			return false, e
		}
		records = append(records, j)
		if e = s.Config.Apply(ctx, p, hash, s.configEdits(next, b)); e != nil {
			// Unknown RPC outcome is not evidence that this process owns a changed file.
			current, _ := storage.FileHash(path)
			if current != hash {
				j.Stage = "needs_review"
				_ = s.writeJournal(context.Background(), j)
			}
			return false, e
		}
		j.AfterHash, e = storage.FileHash(path)
		if e != nil {
			return false, e
		}
		j.Stage = "applied"
		records[len(records)-1] = j
		if e = s.writeJournal(ctx, j); e != nil {
			return false, e
		}
		b.ConfigAfterHash = j.AfterHash
		b.CatalogPath = j.CatalogPath
		b.CatalogHash = j.CatalogHash
		updated[p.ID] = b
	}
	_, err = s.Store.Update(ctx, st.Revision, func(v *model.State) error {
		v.Providers[next.ID] = next
		for id, b := range updated {
			v.Bindings[id] = b
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	committed = true
	for _, j := range records {
		j.Stage = "verified"
		if e = s.writeJournal(ctx, j); e != nil {
			return true, e
		}
	}
	for _, b := range affected {
		if updated[b.ProfileID].TokenRef != b.TokenRef && b.TokenRef.Kind != "" && s.Secrets != nil {
			_ = s.Secrets.Delete(context.Background(), b.TokenRef)
		}
		s.removeCatalog(b)
	}
	return true, nil
}
