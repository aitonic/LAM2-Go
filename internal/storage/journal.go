package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"lam.local/lam2/internal/fault"
	"os"
	"path/filepath"
)

type Change struct {
	Path   string
	Before string
	Data   []byte
	Mode   fs.FileMode
	Delete bool
}
type journalEntry struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
	Mode   uint32 `json:"mode"`
	Delete bool   `json:"delete"`
}
type journalRecord struct {
	ID      string         `json:"id"`
	Stage   string         `json:"stage"`
	Entries []journalEntry `json:"entries"`
}
type Journal struct {
	Root         string
	AllowedRoots []string
	beforeWrite  func(int) error
}

func (j *Journal) dir() string { return filepath.Join(j.Root, "transactions") }
func (j *Journal) save(dir string, r journalRecord) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(context.Background(), filepath.Join(dir, "transaction.json"), b, 0600)
}
func (j *Journal) Apply(ctx context.Context, changes []Change) (string, error) {
	if len(changes) == 0 {
		return "", nil
	}
	if len(changes) > 1024 {
		return "", fault.New("TRANSACTION_TOO_LARGE", "Too many files in one transaction")
	}
	if err := EnsurePrivateDir(j.dir()); err != nil {
		return "", err
	}
	unlock, err := Lock(ctx, filepath.Join(j.dir(), "journal.lock"))
	if err != nil {
		return "", err
	}
	defer unlock()
	rec := journalRecord{ID: NewID(), Stage: "prepared"}
	dir := filepath.Join(j.dir(), rec.ID)
	if err = EnsurePrivateDir(dir); err != nil {
		return "", err
	}
	seen := map[string]bool{}
	for i, c := range changes {
		c.Path = filepath.Clean(c.Path)
		if !filepath.IsAbs(c.Path) || !Within(c.Path, j.AllowedRoots) || seen[c.Path] {
			return "", fault.New("PATH_UNSAFE", "Transaction path is not allowed or duplicated")
		}
		seen[c.Path] = true
		h, e := FileHash(c.Path)
		if e != nil {
			return "", e
		}
		if h != c.Before {
			return "", fault.New("FILE_CONFLICT", "A file changed after the plan was prepared")
		}
		mode := c.Mode
		if mode == 0 {
			mode = 0600
		}
		if h != MissingHash {
			b, e := ReadFile(c.Path, 32<<20)
			if e != nil {
				return "", e
			}
			if e = AtomicWrite(ctx, filepath.Join(dir, fmt.Sprintf("%d.before", i)), b, 0600); e != nil {
				return "", e
			}
			info, _ := os.Stat(c.Path)
			if info != nil {
				mode = info.Mode().Perm()
			}
		}
		after := Hash(c.Data)
		if c.Delete {
			after = MissingHash
		} else if e = AtomicWrite(ctx, filepath.Join(dir, fmt.Sprintf("%d.after", i)), c.Data, 0600); e != nil {
			return "", e
		}
		rec.Entries = append(rec.Entries, journalEntry{Path: c.Path, Before: h, After: after, Mode: uint32(mode), Delete: c.Delete})
	}
	if err = j.save(dir, rec); err != nil {
		return "", err
	}
	rec.Stage = "applying"
	if err = j.save(dir, rec); err != nil {
		return "", err
	}
	for i, c := range changes {
		if err = ctx.Err(); err == nil && j.beforeWrite != nil {
			err = j.beforeWrite(i)
		}
		if err == nil {
			h, e := FileHash(c.Path)
			err = e
			if err == nil && h != rec.Entries[i].Before {
				err = fault.New("FILE_CONFLICT", "A file changed during transaction commit")
			}
		}
		if err == nil {
			if c.Delete {
				err = os.Remove(c.Path)
				if err == nil {
					err = SyncDir(filepath.Dir(c.Path))
				}
			} else {
				err = AtomicWrite(ctx, c.Path, c.Data, fs.FileMode(rec.Entries[i].Mode))
			}
		}
		if err != nil {
			original := err
			if rollbackErr := j.rollback(dir, &rec); rollbackErr != nil {
				return rec.ID, rollbackErr
			}
			return rec.ID, original
		}
	}
	rec.Stage = "committed"
	if err = j.save(dir, rec); err != nil {
		return rec.ID, fault.Wrap("TRANSACTION_RECOVERY_REQUIRED", "Files were applied; restart to recover the transaction marker", err)
	}
	return rec.ID, nil
}
func (j *Journal) rollback(dir string, r *journalRecord) error {
	for i := len(r.Entries) - 1; i >= 0; i-- {
		e := r.Entries[i]
		if !Within(e.Path, j.AllowedRoots) {
			return fault.New("PATH_UNSAFE", "Journal recovery target is not allowed")
		}
		h, err := FileHash(e.Path)
		if err != nil {
			return err
		}
		if h == e.Before {
			continue
		}
		if h != e.After {
			r.Stage = "needs_review"
			_ = j.save(dir, *r)
			return fault.New("RECOVERY_CONFLICT", "A transaction target was changed by another program; no overwrite was attempted")
		}
		if e.Before == MissingHash {
			if err = os.Remove(e.Path); err != nil {
				return err
			}
			if err = SyncDir(filepath.Dir(e.Path)); err != nil {
				return err
			}
		} else {
			b, err := ReadFile(filepath.Join(dir, fmt.Sprintf("%d.before", i)), 32<<20)
			if err != nil {
				return err
			}
			if Hash(b) != e.Before {
				return fault.New("JOURNAL_CORRUPT", "A recovery backup failed its checksum")
			}
			if err = AtomicWrite(context.Background(), e.Path, b, fs.FileMode(e.Mode)); err != nil {
				return err
			}
		}
	}
	r.Stage = "rolled_back"
	return j.save(dir, *r)
}
func (j *Journal) Recover(ctx context.Context) ([]string, error) {
	if err := EnsurePrivateDir(j.dir()); err != nil {
		return nil, err
	}
	unlock, err := Lock(ctx, filepath.Join(j.dir(), "journal.lock"))
	if err != nil {
		return nil, err
	}
	defer unlock()
	entries, err := os.ReadDir(j.dir())
	if err != nil {
		return nil, err
	}
	var result []string
	for _, entry := range entries {
		if !entry.IsDir() || !ValidID(entry.Name()) {
			continue
		}
		if err = ctx.Err(); err != nil {
			return result, err
		}
		dir := filepath.Join(j.dir(), entry.Name())
		b, e := ReadFile(filepath.Join(dir, "transaction.json"), 4<<20)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return result, e
		}
		var r journalRecord
		if e = json.Unmarshal(b, &r); e != nil {
			return result, fault.New("JOURNAL_CORRUPT", "Invalid transaction journal")
		}
		if r.Stage == "committed" || r.Stage == "rolled_back" {
			continue
		}
		if r.Stage == "needs_review" {
			return result, fault.New("RECOVERY_CONFLICT", "A transaction requires explicit conflict resolution")
		}
		allAfter := len(r.Entries) > 0
		for _, v := range r.Entries {
			if !Within(v.Path, j.AllowedRoots) {
				return result, fault.New("PATH_UNSAFE", "Journal path is outside allowed roots")
			}
			h, e := FileHash(v.Path)
			if e != nil {
				return result, e
			}
			if h != v.After {
				allAfter = false
			}
		}
		if allAfter {
			r.Stage = "committed"
			e = j.save(dir, r)
		} else {
			e = j.rollback(dir, &r)
		}
		if e != nil {
			return result, e
		}
		result = append(result, r.ID+":"+r.Stage)
	}
	return result, nil
}
