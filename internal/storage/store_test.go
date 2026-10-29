package storage

import (
	"context"
	"errors"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCASOnlyOneWriter(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st, _ := s.Read(ctx)
	var wg sync.WaitGroup
	var mu sync.Mutex
	success, conflict := 0, 0
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Update(ctx, st.Revision, func(v *model.State) error { v.Settings.Theme = "dark"; return nil })
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				success++
			} else if fault.Code(err) == "REVISION_CONFLICT" {
				conflict++
			} else {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if success != 1 || conflict != 11 {
		t.Fatal(success, conflict)
	}
}
func TestFutureAndCorruptSchemaRejected(t *testing.T) {
	for _, data := range []string{`{"schemaVersion":999,"revision":1}`, `{bad`} {
		d := t.TempDir()
		os.WriteFile(filepath.Join(d, "state.json"), []byte(data), 0600)
		if _, err := Open(d); err == nil {
			t.Fatal("accepted", data)
		}
	}
}
func TestAtomicFailurePreservesOriginal(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a")
	os.WriteFile(p, []byte("original"), 0600)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := AtomicWrite(ctx, p, []byte("new"), 0600); err == nil {
		t.Fatal("ignored cancellation")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "original" {
		t.Fatal(string(b))
	}
}
func TestSafeJoinRejectsSymlinkAndTraversal(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(root, "link"))
	for _, p := range []string{"../auth.json", "link/auth.json", "/etc/passwd"} {
		if _, err := SafeJoin(root, p); err == nil {
			t.Fatal("accepted", p)
		}
	}
	if _, err := SafeJoin(root, "sessions/new.jsonl"); err != nil {
		t.Fatal(err)
	}
}
func TestJournalRollbackAndConflict(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	os.WriteFile(a, []byte("old-a"), 0600)
	os.WriteFile(b, []byte("old-b"), 0600)
	j := Journal{Root: root, AllowedRoots: []string{root}}
	changes := []Change{{Path: a, Before: Hash([]byte("old-a")), Data: []byte("new-a")}, {Path: b, Before: Hash([]byte("old-b")), Data: []byte("new-b")}}
	j.beforeWrite = func(i int) error {
		if i == 1 {
			return errors.New("disk full")
		}
		return nil
	}
	if _, err := j.Apply(ctx, changes); err == nil {
		t.Fatal("expected injected failure")
	}
	v, _ := os.ReadFile(a)
	if string(v) != "old-a" {
		t.Fatal("rollback failed")
	}
	j.beforeWrite = nil
	os.WriteFile(a, []byte("external"), 0600)
	if _, err := j.Apply(ctx, changes); fault.Code(err) != "FILE_CONFLICT" {
		t.Fatal(err)
	}
	v, _ = os.ReadFile(a)
	if string(v) != "external" {
		t.Fatal("overwrote external edit")
	}
}
func TestJournalSuccessfulRecoveryIdempotent(t *testing.T) {
	ctx := context.Background()
	r := t.TempDir()
	p := filepath.Join(r, "a")
	j := Journal{Root: r, AllowedRoots: []string{r}}
	id, err := j.Apply(ctx, []Change{{Path: p, Before: MissingHash, Data: []byte("hello")}})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("missing transaction id")
	}
	if _, err := j.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	v, _ := os.ReadFile(p)
	if string(v) != "hello" {
		t.Fatal(string(v))
	}
}

type testSecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *testSecrets) Get(_ context.Context, r model.CredentialRef) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[r.Service+"/"+r.Account]
	if !ok {
		return "", errors.New("missing")
	}
	return v, nil
}
func (s *testSecrets) Set(_ context.Context, r model.CredentialRef, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[r.Service+"/"+r.Account] = v
	return nil
}
func (s *testSecrets) Delete(_ context.Context, r model.CredentialRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, r.Service+"/"+r.Account)
	return nil
}
func TestVaultEncryptionAndTamper(t *testing.T) {
	ctx := context.Background()
	r := t.TempDir()
	sec := &testSecrets{m: map[string]string{}}
	v := Vault{Root: r, Secrets: sec}
	if err := v.Put(ctx, "account-a", []byte(`{"access_token":"very-secret"}`)); err != nil {
		t.Fatal(err)
	}
	b, err := v.Get(ctx, "account-a")
	if err != nil || !strings.Contains(string(b), "very-secret") {
		t.Fatal(err)
	}
	path := filepath.Join(r, "credentials", "account-a.enc")
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "very-secret") {
		t.Fatal("stored plaintext")
	}
	raw[len(raw)/2] ^= 1
	os.WriteFile(path, raw, 0600)
	if _, err := v.Get(ctx, "account-a"); err == nil {
		t.Fatal("accepted tampering")
	}
}
