package codex

import (
	"context"
	"encoding/json"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/provider"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

type configFake struct {
	path    string
	version string
	writes  int
	params  map[string]any
}

func (f *configFake) Call(ctx context.Context, p model.Profile, method string, params any, out any) error {
	if method == "config/read" {
		b, _ := json.Marshal(map[string]any{"layers": []any{map[string]any{"name": map[string]any{"type": "user", "file": f.path}, "version": f.version}}})
		return json.Unmarshal(b, out)
	}
	f.writes++
	b, _ := json.Marshal(params)
	_ = json.Unmarshal(b, &f.params)
	return nil
}
func TestConfigCASUsesServerVersion(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.toml")
	os.WriteFile(path, []byte("# keep\nmodel='m'\n"), 0600)
	hash, _ := storage.FileHash(path)
	fake := &configFake{path: path, version: "server-version"}
	e := (ConfigEditor{Caller: fake}).Apply(context.Background(), model.Profile{Home: home}, hash, []provider.Edit{{KeyPath: "model", Value: "n", MergeStrategy: "replace"}})
	if e != nil {
		t.Fatal(e)
	}
	if fake.params["expectedVersion"] != "server-version" || fake.params["filePath"] != path {
		t.Fatal(fake.params)
	}
}
func TestConfigRefusesMissingVersionAndExternalChange(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.toml")
	os.WriteFile(path, []byte("model='m'"), 0600)
	hash, _ := storage.FileHash(path)
	fake := &configFake{path: path}
	ed := ConfigEditor{Caller: fake}
	if e := ed.Apply(context.Background(), model.Profile{Home: home}, hash, nil); e == nil {
		t.Fatal("unsafe versionless write")
	}
	fake.version = "x"
	if e := ed.Apply(context.Background(), model.Profile{Home: home}, "wrong", nil); e == nil {
		t.Fatal("ignored hash conflict")
	}
	if fake.writes != 0 {
		t.Fatal("unexpected config write")
	}
}
