package codex

import (
	"context"
	"encoding/json"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/provider"
	"lam.local/lam2/internal/storage"
	"path/filepath"
)

// ConfigEditor delegates TOML preservation to the official config API. A server
// version is required for existing files; a filesystem hash alone is not CAS.
type ConfigEditor struct{ Caller Caller }

func (e ConfigEditor) Apply(ctx context.Context, p model.Profile, expected string, edits []provider.Edit) error {
	path := filepath.Join(p.Home, "config.toml")
	hash, err := storage.FileHash(path)
	if err != nil {
		return err
	}
	if hash != expected {
		return fault.New("BINDING_CONFLICT", "Codex config changed after preview")
	}
	var result json.RawMessage
	if err = e.Caller.Call(ctx, p, "config/read", map[string]any{"includeLayers": true}, &result); err != nil {
		return err
	}
	version := configVersion(result, path)
	if hash != storage.MissingHash && version == "" {
		return fault.New("CONFIG_VERSION_UNAVAILABLE", "Codex did not expose a writable user-config version; no write was attempted")
	}
	if current, err := storage.FileHash(path); err != nil || current != hash {
		return fault.New("BINDING_CONFLICT", "Codex config changed during validation")
	}
	params := map[string]any{"edits": edits, "filePath": path, "reloadUserConfig": true}
	if version != "" {
		params["expectedVersion"] = version
	}
	var out json.RawMessage
	return e.Caller.Call(ctx, p, "config/batchWrite", params, &out)
}
func configVersion(raw []byte, path string) string {
	var result struct {
		Layers []struct {
			Name    struct{ Type, File, Path string }
			Source  struct{ Type, File, Path string }
			Version string
		}
	}
	if json.Unmarshal(raw, &result) != nil {
		return ""
	}
	for _, l := range result.Layers {
		for _, s := range []struct{ Type, File, Path string }{l.Name, l.Source} {
			p := s.File
			if p == "" {
				p = s.Path
			}
			if s.Type == "user" && p != "" && filepath.Clean(p) == filepath.Clean(path) {
				return l.Version
			}
		}
	}
	return ""
}
