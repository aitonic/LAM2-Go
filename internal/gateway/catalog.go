package gateway

import (
	"encoding/json"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
)

// BoundModelCatalog uses the legacy conservative catalog defaults only for a
// registered provider model. An unregistered model still cannot be fabricated.
func BoundModelCatalog(sn Snapshot) (json.RawMessage, error) {
	for i, m := range sn.Provider.Models {
		if m.ID == sn.Binding.Model {
			b, e := model.ModelCatalogEntry(m, i+1)
			if e != nil {
				return nil, catalogError()
			}
			return b, nil
		}
	}
	return nil, catalogError()
}
func ProviderModelCatalog(sn Snapshot) ([]json.RawMessage, error) {
	if len(sn.Provider.Models) == 0 {
		return nil, catalogError()
	}
	out := make([]json.RawMessage, 0, len(sn.Provider.Models))
	for i, m := range sn.Provider.Models {
		b, e := model.ModelCatalogEntry(m, i+1)
		if e != nil {
			return nil, catalogError()
		}
		out = append(out, b)
	}
	return out, nil
}
func catalogError() error {
	return fault.New("MODEL_CATALOG_UNAVAILABLE", "No valid registered Codex model catalog is available")
}
