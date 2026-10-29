package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
)

type archiveIndex struct {
	Path       string   `json:"path"`
	Before     []byte   `json:"before"`
	BeforeHash string   `json:"beforeHash"`
	After      []byte   `json:"after"`
	AfterHash  string   `json:"afterHash"`
	IDs        []string `json:"ids"`
}

func indexLineID(b []byte) string {
	var x struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(bytes.TrimSpace(b), &x) != nil {
		return ""
	}
	return x.ID
}
func (s *Service) prepareIndexes(ctx context.Context, entries []archiveEntry) ([]archiveIndex, error) {
	st, e := s.Store.Read(ctx)
	if e != nil {
		return nil, e
	}
	grouped := map[string][]string{}
	for _, v := range entries {
		p, ok := st.Profiles[v.Session.ProfileID]
		if !ok {
			return nil, fault.New("PROFILE_NOT_FOUND", "Profile missing")
		}
		path, e := storage.SafeJoin(p.Home, "session_index.jsonl")
		if e != nil {
			return nil, e
		}
		grouped[path] = append(grouped[path], v.Session.ID)
	}
	out := []archiveIndex{}
	for path, ids := range grouped {
		b, e := storage.ReadFile(path, 32<<20)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return nil, e
		}
		removed := map[string]bool{}
		for _, id := range ids {
			removed[id] = true
		}
		var next []byte
		for _, line := range bytes.SplitAfter(b, []byte("\n")) {
			if !removed[indexLineID(line)] {
				next = append(next, line...)
			}
		}
		out = append(out, archiveIndex{path, b, storage.Hash(b), next, storage.Hash(next), ids})
	}
	return out, nil
}
func writeIndex(ctx context.Context, path string, expected string, body []byte) error {
	if e := storage.RejectSymlinks(path); e != nil {
		return e
	}
	h, e := storage.FileHash(path)
	if e != nil {
		return e
	}
	if h != expected {
		return fault.New("SESSION_INDEX_CONFLICT", "Session index changed; preview again")
	}
	return storage.AtomicWrite(ctx, path, body, 0600)
}
func (s *Service) restoreIndexes(ctx context.Context, m archiveManifest) error {
	st, e := s.Store.Read(ctx)
	if e != nil {
		return e
	}
	allowed := map[string]bool{}
	for _, entry := range m.Entries {
		p, ok := st.Profiles[entry.Session.ProfileID]
		if ok {
			allowed[filepath.Join(p.Home, "session_index.jsonl")] = true
		}
	}
	for _, x := range m.Indexes {
		if !allowed[x.Path] || storage.Hash(x.Before) != x.BeforeHash || storage.Hash(x.After) != x.AfterHash {
			return fault.New("ARCHIVE_CORRUPT", "Invalid session index snapshot")
		}
		h, e := storage.FileHash(x.Path)
		if e != nil {
			return e
		}
		if h == x.BeforeHash {
			continue
		}
		if h == x.AfterHash {
			if e = writeIndex(ctx, x.Path, h, x.Before); e != nil {
				return e
			}
			continue
		}
		// Preserve index entries written by Codex since deletion; only recover rows
		// for the sessions restored by this operation and only if not already named.
		current := []byte{}
		if h != storage.MissingHash {
			current, e = storage.ReadFile(x.Path, 32<<20)
			if e != nil {
				return e
			}
		}
		known := map[string]bool{}
		wanted := map[string]bool{}
		for _, id := range x.IDs {
			wanted[id] = true
		}
		for _, line := range bytes.Split(current, []byte("\n")) {
			known[indexLineID(line)] = true
		}
		next := append([]byte{}, current...)
		// Keep every historical line for one recovered ID: the last name wins.
		for _, line := range bytes.SplitAfter(x.Before, []byte("\n")) {
			id := indexLineID(line)
			if !wanted[id] || known[id] {
				continue
			}
			if len(next) > 0 && next[len(next)-1] != '\n' {
				next = append(next, '\n')
			}
			next = append(next, line...)
		}
		if !bytes.Equal(next, current) {
			if e = writeIndex(ctx, x.Path, h, next); e != nil {
				return e
			}
		}
	}
	return nil
}
