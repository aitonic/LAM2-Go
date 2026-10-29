package app

import (
	"context"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/testutil"
	"os"
	"path/filepath"
	"testing"
)

// Uses real production composition, but an isolated test-only secret store.
// Running this suite requires the pinned MyGo/Codex/SQLite dependencies.
func TestCompositionAndSelectedCredentialIsolation(t *testing.T) {
	c, e := Open(t.TempDir(), &testutil.Secrets{})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	st, e := c.Store.Read(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.Accounts.Create(context.Background(), "one", "profile", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("OPENAI_API_KEY", "foreign")
	env, e := c.ProfileEnvironment(context.Background(), p)
	if e != nil {
		t.Fatal(e)
	}
	if v := env["OPENAI_API_KEY"]; v != "" {
		t.Fatal("unbound credential leaked")
	}
	if _, e := os.Stat(filepath.Join(c.Store.Root, "state.json")); e != nil {
		t.Fatal(e)
	}
}
func TestSnapshotDoesNotInventQuota(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c, e := Open(t.TempDir(), &testutil.Secrets{})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	s, e := c.BasicSnapshot(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if len(s.Profiles) != 0 || len(s.Quotas) != 0 {
		t.Fatal("fabricated data")
	}
	if s.State.SchemaVersion != model.NewState().SchemaVersion {
		t.Fatal("schema")
	}
}
