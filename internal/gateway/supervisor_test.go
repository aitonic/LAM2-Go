package gateway

import (
	"context"
	"encoding/json"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/provider"
	"lam.local/lam2/internal/storage"
	"lam.local/lam2/internal/testutil"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateResolverRejectsCrossBindingAndFreezesCredential(t *testing.T) {
	ctx := context.Background()
	store, e := storage.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	secrets := &testutil.Secrets{}
	st, _ := store.Read(ctx)
	ref := model.CredentialRef{Kind: "keychain", Service: "test", Account: "token"}
	key := model.CredentialRef{Kind: "keychain", Service: "test", Account: "upstream"}
	secrets.Set(ctx, ref, "private-binding-token")
	secrets.Set(ctx, key, "secret-one")
	_, e = store.Update(ctx, st.Revision, func(s *model.State) error {
		s.Profiles["p"] = model.Profile{ID: "p"}
		s.Providers["up"] = model.Provider{ID: "up", AuthKind: "bearer", Credential: key}
		s.Bindings["p"] = model.Binding{ID: "b", ProfileID: "p", ProviderID: "up", TokenRef: ref, Model: "m", RouteKind: "gateway"}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	svc := &provider.Service{Store: store, Secrets: secrets}
	r := &StateResolver{Store: store, Secrets: secrets, Providers: svc}
	snap, e := r.Resolve(ctx, "private-binding-token")
	if e != nil {
		t.Fatal(e)
	}
	secrets.Set(ctx, key, "secret-two")
	if snap.Headers.Get("Authorization") != "Bearer secret-one" {
		t.Fatal("snapshot mutated")
	}
	if _, e = r.Resolve(ctx, "different-token"); e == nil {
		t.Fatal("bad token accepted")
	}
}
func TestSupervisorHealthDetectsStranger(t *testing.T) {
	root := t.TempDir()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	hash, e := storage.FileHash(exe)
	if e != nil {
		t.Fatal(e)
	}
	s := NewServer(model.DefaultLimits(), nil, nil)
	s.Identity = Identity{PID: os.Getpid(), ExecutableHash: hash, Nonce: "private-identity-nonce-123456789"}
	server := httptest.NewServer(s)
	defer server.Close()
	s.Identity.Address = server.Listener.Addr().String()
	raw, _ := json.Marshal(s.Identity)
	storage.AtomicWrite(context.Background(), filepath.Join(root, "gateway", "identity.json"), raw, 0600)
	super := Supervisor{Root: root, Executable: exe}
	_, _, e = super.Probe(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	s.Identity.Nonce = "wrong-nonce-that-is-not-trusted"
	_, _, e = super.Probe(context.Background())
	if e == nil {
		t.Fatal("stranger trusted")
	}
}
func TestRunDaemonCancellationRemovesOwnedIdentity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	root := t.TempDir()
	exe, _ := os.Executable()
	s := NewServer(model.DefaultLimits(), nil, nil)
	done := make(chan error, 1)
	go func() { done <- RunDaemon(ctx, root, exe, "127.0.0.1:0", s) }()
	super := Supervisor{Root: root, Executable: exe}
	deadline := time.Now().Add(3 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		if _, _, e := super.Probe(context.Background()); e == nil {
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if !ready {
		t.Fatal("not ready")
	}
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("daemon leaked")
	}
	if _, e := os.Stat(filepath.Join(root, "gateway", "identity.json")); !os.IsNotExist(e) {
		t.Fatal("identity left behind")
	}
}

func TestStopAndWaitLeavesNoDaemonAndNoForeignKill(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := t.TempDir()
	exe, _ := os.Executable()
	s := NewServer(model.DefaultLimits(), nil, nil)
	done := make(chan error, 1)
	go func() { done <- RunDaemon(ctx, root, exe, "127.0.0.1:0", s) }()
	super := Supervisor{Root: root, Executable: exe}
	ready := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if _, _, e := super.Probe(ctx); e == nil {
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		t.Fatal("not ready")
	}
	if e := super.StopAndWait(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("still running")
	}
	if e := super.StopAndWait(ctx); e != nil {
		t.Fatal("idempotent stopped check", e)
	}
}
