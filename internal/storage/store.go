package storage

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"math"
	"path/filepath"
)

const maxStateBytes = 8 << 20

type Store struct{ Root string }

func Open(root string) (*Store, error) {
	root, err := ResolveHome(root)
	if err != nil {
		return nil, err
	}
	if err = EnsurePrivateDir(root); err != nil {
		return nil, err
	}
	s := &Store{Root: root}
	unlock, err := Lock(context.Background(), filepath.Join(root, "state.lock"))
	if err != nil {
		return nil, err
	}
	defer unlock()
	if _, err = s.read(); errors.Is(err, fs.ErrNotExist) {
		b, _ := json.MarshalIndent(model.NewState(), "", "  ")
		if err = AtomicWrite(context.Background(), s.Path(), b, 0600); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Store) Path() string { return filepath.Join(s.Root, "state.json") }
func (s *Store) read() (model.State, error) {
	var st model.State
	b, err := ReadFile(s.Path(), maxStateBytes)
	if err != nil {
		return st, err
	}
	if err = json.Unmarshal(b, &st); err != nil {
		return st, fault.Wrap("STATE_CORRUPT", "The state file is not valid JSON", err)
	}
	if st.SchemaVersion != model.SchemaVersion {
		return st, fault.New("SCHEMA_UNSUPPORTED", "State schema is newer or older than this implementation")
	}
	if st.Revision < 1 {
		return st, fault.New("STATE_CORRUPT", "State revision is missing")
	}
	st.Normalize()
	return st, nil
}
func (s *Store) Read(ctx context.Context) (model.State, error) {
	if err := ctx.Err(); err != nil {
		return model.State{}, err
	}
	return s.read()
}
func (s *Store) Update(ctx context.Context, expected uint64, fn func(*model.State) error) (model.State, error) {
	return s.Commit(ctx, expected, func(v *model.State) ([]Change, error) { return nil, fn(v) })
}
func (s *Store) Commit(ctx context.Context, expected uint64, fn func(*model.State) ([]Change, error)) (model.State, error) {
	unlock, err := Lock(ctx, filepath.Join(s.Root, "state.lock"))
	if err != nil {
		return model.State{}, err
	}
	defer unlock()
	st, err := s.read()
	if err != nil {
		return st, err
	}
	if expected != st.Revision {
		return st, fault.New("REVISION_CONFLICT", "State changed; refresh and retry")
	}
	if st.Revision == math.MaxUint64 {
		return st, fault.New("REVISION_OVERFLOW", "State revision exhausted")
	}
	before, err := FileHash(s.Path())
	if err != nil {
		return st, err
	}
	allowed := []string{s.Root}
	for _, p := range st.Profiles {
		allowed = append(allowed, p.Home)
	}
	changes, err := fn(&st)
	if err != nil {
		return st, err
	}
	for _, p := range st.Profiles {
		if p.Managed && Within(p.Home, []string{s.Root}) {
			allowed = append(allowed, p.Home)
		}
	}
	st.Revision++
	st.SchemaVersion = model.SchemaVersion
	st.Normalize()
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return st, err
	}
	if len(data) > maxStateBytes {
		return st, fault.New("STATE_TOO_LARGE", "State exceeds the maximum size")
	}
	changes = append(changes, Change{Path: s.Path(), Before: before, Data: data, Mode: 0600})
	j := Journal{Root: s.Root, AllowedRoots: allowed}
	_, err = j.Apply(ctx, changes)
	if err != nil {
		return st, err
	}
	return st, nil
}
func (s *Store) Recover(ctx context.Context) ([]string, error) {
	unlock, err := Lock(ctx, filepath.Join(s.Root, "state.lock"))
	if err != nil {
		return nil, err
	}
	defer unlock()
	st, err := s.read()
	if err != nil {
		return nil, err
	}
	roots := []string{s.Root}
	for _, p := range st.Profiles {
		roots = append(roots, p.Home)
	}
	j := Journal{Root: s.Root, AllowedRoots: roots}
	return j.Recover(ctx)
}
