package session

import (
	"context"
	"encoding/json"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
	"time"
)

type DeletePlan struct {
	ID        string            `json:"id"`
	Items     []model.Session   `json:"items"`
	Blocked   map[string]string `json:"blocked"`
	Hashes    map[string]string `json:"hashes"`
	CreatedAt time.Time         `json:"createdAt"`
}

func (s *Service) planPath(id string) (string, error) {
	if !storage.ValidID(id) {
		return "", fault.New("INVALID_PLAN", "Invalid plan identifier")
	}
	return storage.SafeJoin(s.Store.Root, filepath.Join("plans", "delete-"+id+".json"))
}
func (s *Service) PlanDelete(ctx context.Context, refs []Ref) (DeletePlan, error) {
	plan := DeletePlan{ID: storage.NewID(), Items: []model.Session{}, Blocked: map[string]string{}, Hashes: map[string]string{}, CreatedAt: s.now().UTC()}

	seen := map[string]bool{}
	for _, r := range refs {
		key := r.ProfileID + "/" + r.SessionID
		if seen[key] {
			continue
		}
		seen[key] = true
		item, err := s.Resolve(ctx, r.ProfileID, r.SessionID)
		if err != nil {
			return plan, err
		}
		if item.Protected {
			plan.Blocked[key] = item.ProtectionReason
			continue
		}
		h, err := storage.FileHash(item.Path)
		if err != nil {
			return plan, err
		}
		plan.Items = append(plan.Items, item)
		plan.Hashes[item.Path] = h
	}
	path, _ := s.planPath(plan.ID)
	b, _ := json.MarshalIndent(plan, "", "  ")
	if len(b) > 8<<20 {
		return plan, fault.New("DELETE_BATCH_TOO_LARGE", "Deletion preview exceeds 8 MiB; select a narrower date range")
	}
	return plan, storage.AtomicWrite(ctx, path, b, 0600)
}

type archiveEntry struct {
	Session model.Session `json:"session"`
	Backup  string        `json:"backup"`
	Hash    string        `json:"hash"`
}
type archiveManifest struct {
	ID      string         `json:"id"`
	Stage   string         `json:"stage"`
	Entries []archiveEntry `json:"entries"`
	Indexes []archiveIndex `json:"indexes,omitempty"`
}

func (s *Service) Delete(ctx context.Context, id string, confirmed bool) error {
	if !confirmed {
		return fault.New("CONFIRMATION_REQUIRED", "Confirm the reviewed session archive plan")
	}
	path, err := s.planPath(id)
	if err != nil {
		return err
	}
	raw, err := storage.ReadFile(path, 8<<20)
	if err != nil {
		return err
	}
	var plan DeletePlan
	if json.Unmarshal(raw, &plan) != nil || plan.ID != id {
		return fault.New("INVALID_PLAN", "Invalid deletion plan")
	}
	if s.now().Sub(plan.CreatedAt) > 30*time.Minute {
		return fault.New("PLAN_STALE", "The deletion plan expired")
	}
	unlock, err := storage.Lock(ctx, filepath.Join(s.Store.Root, "session-archive.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	dir, err := storage.SafeJoin(s.Store.Root, filepath.Join("archive", id))
	if err != nil {
		return err
	}
	manifest := archiveManifest{ID: id, Stage: "prepared"}
	for _, old := range plan.Items {
		current, err := s.Resolve(ctx, old.ProfileID, old.ID)
		if err != nil {
			return err
		}
		if current.Path != old.Path || current.Protected {
			return fault.New("PLAN_STALE", "The session is now protected or moved")
		}
		h, err := storage.FileHash(current.Path)
		if err != nil {
			return err
		}
		if h != plan.Hashes[current.Path] {
			return fault.New("PLAN_STALE", "The session changed after the preview")
		}
		backup := filepath.Join(dir, current.ProfileID, filepath.Base(current.Path))
		manifest.Entries = append(manifest.Entries, archiveEntry{current, backup, h})
	}
	manifest.Indexes, err = s.prepareIndexes(ctx, manifest.Entries)
	if err != nil {
		return err
	}
	if err = storage.EnsurePrivateDir(dir); err != nil {
		return err
	}
	save := func() error {
		b, e := json.MarshalIndent(manifest, "", "  ")
		if e != nil {
			return e
		}
		if len(b) > 128<<20 {
			return fault.New("DELETE_BATCH_TOO_LARGE", "Archive index snapshot exceeds 128 MiB; select fewer profiles")
		}
		return storage.AtomicWrite(context.Background(), filepath.Join(dir, "manifest.json"), b, 0600)
	}
	if err = save(); err != nil {
		return err
	}
	for _, e := range manifest.Entries {
		if _, err = storage.CopyChecked(ctx, e.Session.Path, e.Backup, e.Hash, storage.MissingHash); err != nil {
			return err
		}
	}
	manifest.Stage = "backed_up"
	if err = save(); err != nil {
		return err
	}
	removed := []archiveEntry{}
	rollback := func() {
		for _, e := range removed {
			_, _ = storage.CopyChecked(context.Background(), e.Backup, e.Session.Path, e.Hash, storage.MissingHash)
		}
	}
	for _, e := range manifest.Entries {
		if err = ctx.Err(); err != nil {
			rollback()
			return err
		}
		h, err := storage.FileHash(e.Session.Path)
		if err != nil || h != e.Hash {
			rollback()
			return fault.New("PLAN_STALE", "A session changed before archiving")
		}
		if err = os.Remove(e.Session.Path); err != nil {
			rollback()
			return err
		}
		_ = storage.SyncDir(filepath.Dir(e.Session.Path))
		removed = append(removed, e)
	}
	for _, index := range manifest.Indexes {
		if err = writeIndex(ctx, index.Path, index.BeforeHash, index.After); err != nil {
			rollback()
			if re := s.restoreIndexes(context.Background(), manifest); re != nil {
				return fault.Wrap("ARCHIVE_RECOVERY_REQUIRED", "Session files restored; index restoration requires review", re)
			}
			return err
		}
	}
	manifest.Stage = "archived"
	s.Invalidate()
	return save()
}
func (s *Service) Restore(ctx context.Context, id string, confirmed bool) error {
	return s.restoreArchive(ctx, id, confirmed, false)
}
func (s *Service) restoreArchive(ctx context.Context, id string, confirmed, interruptedOnly bool) error {
	if !confirmed {
		return fault.New("CONFIRMATION_REQUIRED", "Confirm archive restoration")
	}
	if !storage.ValidID(id) {
		return fault.New("INVALID_PLAN", "Invalid archive ID")
	}
	unlock, err := storage.Lock(ctx, filepath.Join(s.Store.Root, "session-archive.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	mp, err := storage.SafeJoin(s.Store.Root, filepath.Join("archive", id, "manifest.json"))
	if err != nil {
		return err
	}
	raw, err := storage.ReadFile(mp, 128<<20)
	if err != nil {
		return err
	}
	var m archiveManifest
	if json.Unmarshal(raw, &m) != nil || m.ID != id {
		return fault.New("ARCHIVE_CORRUPT", "Invalid archive manifest")
	}
	if interruptedOnly && m.Stage != "prepared" && m.Stage != "backed_up" {
		return nil
	}
	st, err := s.Store.Read(ctx)
	if err != nil {
		return err
	}
	for _, e := range m.Entries {
		p, ok := st.Profiles[e.Session.ProfileID]
		if !ok || !storage.Within(e.Session.Path, []string{filepath.Join(p.Home, "sessions")}) || !storage.Within(e.Backup, []string{filepath.Join(s.Store.Root, "archive", id)}) {
			return fault.New("PATH_UNSAFE", "Archive restoration path is no longer approved")
		}
		h, err := storage.FileHash(e.Session.Path)
		if err != nil {
			return err
		}
		if h != storage.MissingHash && h != e.Hash {
			return fault.New("RESTORE_CONFLICT", "A different session now occupies the original path")
		}
	}
	for _, e := range m.Entries {
		h, _ := storage.FileHash(e.Session.Path)
		if h == e.Hash {
			continue
		}
		if _, err = storage.CopyChecked(ctx, e.Backup, e.Session.Path, e.Hash, storage.MissingHash); err != nil {
			return err
		}
	}
	if err = s.restoreIndexes(ctx, m); err != nil {
		return err
	}
	m.Stage = "restored"
	raw, _ = json.MarshalIndent(m, "", "  ")
	s.Invalidate()
	return storage.AtomicWrite(ctx, mp, raw, 0600)
}

// RecoverArchives rolls back incomplete local deletion transactions on startup;
// completed user archives remain archived. Restore rechecks the stage under lock.
func (s *Service) RecoverArchives(ctx context.Context) ([]string, error) {
	recovered := []string{}
	root := filepath.Join(s.Store.Root, "archive")
	if e := storage.RejectSymlinks(root); e != nil {
		return recovered, e
	}
	entries, e := os.ReadDir(root)
	if os.IsNotExist(e) {
		return recovered, nil
	}
	if e != nil {
		return recovered, e
	}
	for _, entry := range entries {
		if e = ctx.Err(); e != nil {
			return recovered, e
		}
		if !entry.IsDir() || !storage.ValidID(entry.Name()) {
			continue
		}
		path, e := storage.SafeJoin(root, filepath.Join(entry.Name(), "manifest.json"))
		if e != nil {
			return recovered, e
		}
		raw, e := storage.ReadFile(path, 128<<20)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return recovered, e
		}
		var m archiveManifest
		if json.Unmarshal(raw, &m) != nil || m.ID != entry.Name() {
			return recovered, fault.New("ARCHIVE_CORRUPT", "Invalid pending archive")
		}
		if m.Stage != "prepared" && m.Stage != "backed_up" {
			continue
		}
		if e = s.restoreArchive(ctx, m.ID, true, true); e != nil {
			return recovered, e
		}
		recovered = append(recovered, "restored interrupted session archive "+m.ID)
	}
	return recovered, nil
}
