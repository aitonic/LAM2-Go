package account

import (
	"context"
	"lam.local/lam2/internal/fault"
	"os"
	"path/filepath"
	"testing"
)

// Original list_accounts exposes Codex homes with actual Codex signal files
// immediately, but ignores directories that happen to share the .codex- prefix.
func TestLegacyDiscoverySignalAndAutomaticVisibleListing(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	root := t.TempDir()
	for _, name := range []string{".codex", ".codex-work", ".codex-empty", ".codex-just-a-dir"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".codex", "config.toml"), []byte("model = \"a\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codex-work", "history.jsonl"), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	list, err := Discover(root)
	if err != nil || len(list) != 2 {
		t.Fatalf("discover %v %v", list, err)
	}
	if list[0].ID != "main" || list[1].Name != "codex-work" {
		t.Fatalf("legacy identity names %#v", list)
	}
	visible, err := s.ListVisible(ctx, root)
	if err != nil || len(visible) != 2 {
		t.Fatalf("visible %v %v", visible, err)
	}
	state, _ := st.Read(ctx)
	if state.Revision != 1 || len(state.Profiles) != 0 {
		t.Fatal("discovery mutated state")
	}
	_, err = s.Register(ctx, filepath.Join(root, ".codex-work"), "Work", 1)
	if err != nil {
		t.Fatal(err)
	}
	visible, err = s.ListVisible(ctx, root)
	if err != nil || len(visible) != 2 {
		t.Fatalf("registered home duplicated %v %v", visible, err)
	}
	got := false
	for _, p := range visible {
		if p.Name == "Work" && p.ID != "main" {
			got = true
		}
	}
	if !got {
		t.Fatalf("registered name not preferred %#v", visible)
	}
}

func TestDiscoveredSharedCodexIsTheProtectedMainIdentity(t *testing.T) {
	s, st := fixture(t)
	if e := os.MkdirAll(s.SharedHome, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(s.SharedHome, "auth.json"), []byte(`{"auth_mode":"chatgpt"}`), 0600); e != nil {
		t.Fatal(e)
	}
	p, e := s.Register(context.Background(), s.SharedHome, "main", 1)
	if e != nil || p.ID != "main" {
		t.Fatalf("default home must map to protected main: %#v %v", p, e)
	}
	state, _ := st.Read(context.Background())
	if e := s.Delete(context.Background(), p.ID, state.Revision, true); fault.Code(e) != "MAIN_ACCOUNT_PROTECTED" {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(s.SharedHome, "auth.json")); e != nil {
		t.Fatal("default auth was changed:", e)
	}
}
