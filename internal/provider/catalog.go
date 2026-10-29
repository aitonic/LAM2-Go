package provider

import (
	"context"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
)

func modelSettingsEdits(p model.Provider, selected, catalog string) []Edit {
	var window, threshold, effort any
	for _, m := range p.Models {
		if m.ID == selected {
			if w := model.ModelContextWindow(m); w > 0 {
				window = w
				threshold = model.CompactThreshold(w)
			}
			break
		}
	}
	if p.ReasoningEffort != "" {
		effort = p.ReasoningEffort
	}
	return []Edit{{"model_catalog_json", catalog, "replace"}, {"model_context_window", window, "replace"}, {"model_auto_compact_token_limit", threshold, "replace"}, {"model_reasoning_effort", effort, "replace"}}
}
func (s *Service) removeCatalog(b model.Binding) {
	if b.CatalogPath == "" || b.CatalogHash == "" || !storage.Within(b.CatalogPath, []string{filepath.Join(s.Store.Root, "catalogs")}) {
		return
	}
	st, err := s.Store.Read(context.Background())
	if err != nil {
		return
	}
	for _, active := range st.Bindings {
		if active.CatalogPath == b.CatalogPath {
			return
		}
	}
	if h, e := storage.FileHash(b.CatalogPath); e == nil && h == b.CatalogHash {
		_ = os.Remove(b.CatalogPath)
	}
}
