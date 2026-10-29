package session

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type Query struct {
	ProfileID  string
	Search     string
	Workspace  string
	After      time.Time
	Before     time.Time
	MinSize    int64
	MaxSize    int64
	Sort       string
	Descending bool
	Offset     int
	Limit      int
}
type Ref struct {
	ProfileID string `json:"profileId"`
	SessionID string `json:"sessionId"`
}
type Detail struct {
	Session      model.Session     `json:"session"`
	Lines        []json.RawMessage `json:"lines"`
	SkippedLines int               `json:"skippedLines"`
	Truncated    bool              `json:"truncated"`
}
type cached struct {
	size     int64
	modified time.Time
	session  model.Session
}
type Service struct {
	SharedHome string
	Store      *storage.Store
	Clock      func() time.Time
	mu         sync.Mutex
	cache      map[string]cached
}

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}
func (s *Service) Invalidate() { s.mu.Lock(); s.cache = nil; s.mu.Unlock() }
func (s *Service) Query(ctx context.Context, q Query) (model.Page[model.Session], error) {
	page := model.Page[model.Session]{Items: []model.Session{}, Offset: max(q.Offset, 0), Limit: q.Limit}
	if page.Limit <= 0 {
		page.Limit = 100
	}
	page.Limit = min(page.Limit, 1000)
	st, err := s.Store.Read(ctx)
	if err != nil {
		return page, err
	}
	if q.ProfileID != "" {
		if _, ok := st.Profiles[q.ProfileID]; !ok {
			return page, fault.New("PROFILE_NOT_FOUND", "Profile not found")
		}
	}
	all := []model.Session{}
	for _, p := range st.Profiles {
		if q.ProfileID != "" && p.ID != q.ProfileID {
			continue
		}
		if p.AuthMode == "pat" && p.Home == s.SharedHome && st.ActivePAT != p.ID {
			continue
		}
		items, err := s.ListProfile(ctx, p)
		if err != nil {
			return page, err
		}
		all = append(all, items...)
	}
	needle := strings.ToLower(q.Search)
	filtered := all[:0]
	for _, item := range all {
		if needle != "" && !strings.Contains(strings.ToLower(item.Title+" "+item.ID+" "+item.CWD), needle) {
			continue
		}
		if q.Workspace != "" && item.CWD != q.Workspace {
			continue
		}
		if !q.After.IsZero() && item.ModifiedAt.Before(q.After) || !q.Before.IsZero() && !item.ModifiedAt.Before(q.Before) {
			continue
		}
		if item.Size < q.MinSize || q.MaxSize > 0 && item.Size > q.MaxSize {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.Slice(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		cmp := 0
		switch q.Sort {
		case "size":
			if a.Size < b.Size {
				cmp = -1
			} else if a.Size > b.Size {
				cmp = 1
			}
		case "title":
			cmp = strings.Compare(a.Title, b.Title)
		default:
			if a.ModifiedAt.Before(b.ModifiedAt) {
				cmp = -1
			} else if a.ModifiedAt.After(b.ModifiedAt) {
				cmp = 1
			}
		}
		if cmp == 0 {
			return a.ProfileID+"/"+a.ID+a.Path < b.ProfileID+"/"+b.ID+b.Path
		}
		if q.Descending {
			return cmp > 0
		}
		return cmp < 0
	})
	page.Total = len(filtered)
	start := min(page.Offset, len(filtered))
	end := min(start+page.Limit, len(filtered))
	page.Items = append(page.Items, filtered[start:end]...)
	return page, nil
}
func (s *Service) ListProfile(ctx context.Context, p model.Profile) ([]model.Session, error) {
	return s.listRoot(ctx, p, "sessions")
}

// ListUsageSources includes Codex-native archived JSONL without exposing archived
// sessions as resumable active sessions. It does not read LAM deletion backups.
func (s *Service) ListUsageSources(ctx context.Context, p model.Profile) ([]model.Session, error) {
	active, e := s.listRoot(ctx, p, "sessions")
	if e != nil {
		return nil, e
	}
	archived, e := s.listRoot(ctx, p, "archived_sessions")
	if e != nil {
		return nil, e
	}
	return append(active, archived...), nil
}
func (s *Service) listRoot(ctx context.Context, p model.Profile, directory string) ([]model.Session, error) {
	titles, err := indexTitles(ctx, p.Home)
	if err != nil {
		return nil, err
	}
	root, err := storage.SafeJoin(p.Home, directory)
	if err != nil {
		return nil, err
	}
	out := []model.Session{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) && path == root {
				return nil
			}
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		s.mu.Lock()
		entry, ok := s.cache[path]
		s.mu.Unlock()
		var item model.Session
		if ok && entry.size == info.Size() && entry.modified.Equal(info.ModTime()) {
			item = entry.session
		} else {
			item, err = inspect(ctx, p, path, info)
			if err != nil {
				return err
			}
			s.mu.Lock()
			if s.cache == nil || len(s.cache) > 50000 {
				s.cache = map[string]cached{}
			}
			s.cache[path] = cached{info.Size(), info.ModTime(), item}
			s.mu.Unlock()
		}
		item.ProfileID = p.ID
		if name, ok := titles[item.ID]; ok {
			item.Title = name
		}
		if item.ID != "" {
			out = append(out, item)
		}
		return nil
	})
	if err != nil {
		return nil, fault.Wrap("SESSION_SCAN_FAILED", "A session directory could not be read", err)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ModifiedAt.Equal(out[j].ModifiedAt) {
			return out[i].Path < out[j].Path
		}
		return out[i].ModifiedAt.After(out[j].ModifiedAt)
	})
	cutoff := s.now().Add(-7 * 24 * time.Hour)
	for i := range out {
		if i < 20 {
			out[i].Protected = true
			out[i].ProtectionReason = "One of the 20 newest sessions"
		} else if !out[i].ModifiedAt.Before(cutoff) {
			out[i].Protected = true
			out[i].ProtectionReason = "Modified within the last seven days"
		}
	}
	return out, nil
}
func inspect(ctx context.Context, p model.Profile, path string, info fs.FileInfo) (model.Session, error) {
	result := model.Session{ProfileID: p.ID, Path: path, Size: info.Size(), ModifiedAt: info.ModTime().UTC()}
	if err := storage.RejectSymlinks(path); err != nil {
		return result, err
	}
	f, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer f.Close()
	scanPart := func(reader io.Reader) error {
		scan := bufio.NewScanner(reader)
		scan.Buffer(make([]byte, 4096), 2<<20)
		for scan.Scan() {
			if err := ctx.Err(); err != nil {
				return err
			}
			b := scan.Bytes()
			if !utf8.Valid(b) {
				result.Diagnostics = append(result.Diagnostics, "INVALID_UTF8")
				continue
			}
			var line struct {
				Type    string                     `json:"type"`
				Payload map[string]json.RawMessage `json:"payload"`
			}
			if json.Unmarshal(b, &line) != nil {
				result.Diagnostics = append(result.Diagnostics, "INVALID_JSON")
				continue
			}
			get := func(keys ...string) string {
				for _, k := range keys {
					var v string
					if json.Unmarshal(line.Payload[k], &v) == nil && v != "" {
						return v
					}
				}
				return ""
			}
			if line.Type == "session_meta" {
				if result.ID == "" {
					result.ID = get("id")
				}
				if result.CWD == "" {
					result.CWD = get("cwd")
				}
				if result.Provider == "" {
					result.Provider = get("model_provider", "provider_id", "providerId")
				}
			}
			if line.Type == "turn_context" {
				if v := get("cwd", "workdir", "working_directory", "workingDirectory", "current_dir"); v != "" {
					result.CWD = v
				}
				if v := get("model", "original_model", "originalModel"); v != "" {
					result.Model = v
				}
				if v := get("model_provider", "provider_id", "providerId", "original_provider_id", "originalProviderId"); v != "" {
					result.Provider = v
				}
			}
			if result.Title == "" {
				result.Title = title(line.Type, line.Payload)
			}
		}
		if scan.Err() != nil {
			result.Diagnostics = append(result.Diagnostics, "METADATA_SCAN_LIMIT")
		}
		return nil
	}
	if err := scanPart(io.LimitReader(f, 2<<20)); err != nil {
		return result, err
	}
	// Like legacy LAM, inspect the end as well as the initial metadata: model and
	// workspace can change many megabytes after the first turn.
	if info.Size() > 2<<20 {
		start := info.Size() - (256 << 10)
		if _, err = f.Seek(start, io.SeekStart); err != nil {
			return result, err
		}
		tail := bufio.NewReader(io.LimitReader(f, 256<<10))
		if _, err = tail.ReadBytes('\n'); err != nil && err != io.EOF {
			return result, err
		}
		if err = scanPart(tail); err != nil {
			return result, err
		}
	}
	if result.ID == "" {
		result.ID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if result.Title == "" {
		result.Title = result.ID
	}
	return result, nil
}
func title(kind string, p map[string]json.RawMessage) string {
	var typ, role, text string
	_ = json.Unmarshal(p["type"], &typ)
	_ = json.Unmarshal(p["role"], &role)
	if kind == "event_msg" && typ == "user_message" {
		_ = json.Unmarshal(p["message"], &text)
	}
	if kind == "response_item" && typ == "message" && role == "user" {
		var content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		_ = json.Unmarshal(p["content"], &content)
		for _, c := range content {
			if c.Type == "input_text" || c.Type == "text" {
				text = c.Text
				break
			}
		}
	}
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	r := []rune(text)
	if len(r) > 160 {
		r = r[:160]
	}
	return string(r)
}
func (s *Service) Resolve(ctx context.Context, profileID, id string) (model.Session, error) {
	st, err := s.Store.Read(ctx)
	if err != nil {
		return model.Session{}, err
	}
	p, ok := st.Profiles[profileID]
	if !ok {
		return model.Session{}, fault.New("PROFILE_NOT_FOUND", "Profile not found")
	}
	if p.AuthMode == "pat" && p.Home == s.SharedHome && st.ActivePAT != p.ID {
		return model.Session{}, fault.New("PAT_INACTIVE", "Activate this PAT before reading the shared session home")
	}
	items, err := s.ListProfile(ctx, p)
	if err != nil {
		return model.Session{}, err
	}
	var found *model.Session
	for _, item := range items {
		if item.ID == id {
			if found != nil {
				return model.Session{}, fault.New("SESSION_AMBIGUOUS", "More than one file claims the same session ID")
			}
			v := item
			found = &v
		}
	}
	if found == nil {
		return model.Session{}, fault.New("SESSION_NOT_FOUND", "Session not found")
	}
	return *found, nil
}
func (s *Service) Read(ctx context.Context, profileID, id string) (Detail, error) {
	item, err := s.Resolve(ctx, profileID, id)
	if err != nil {
		return Detail{}, err
	}
	result := Detail{Session: item, Lines: []json.RawMessage{}}
	if err = storage.RejectSymlinks(item.Path); err != nil {
		return result, err
	}
	f, err := os.Open(item.Path)
	if err != nil {
		return result, err
	}
	defer f.Close()
	scan := bufio.NewScanner(io.LimitReader(f, (4<<20)+1))
	scan.Buffer(make([]byte, 4096), 1<<20)
	total := 0
	for scan.Scan() {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		line := scan.Bytes()
		total += len(line) + 1
		if total > 4<<20 {
			result.Truncated = true
			break
		}
		if !utf8.Valid(line) || !json.Valid(line) {
			result.SkippedLines++
			continue
		}
		result.Lines = append(result.Lines, append(json.RawMessage(nil), line...))
	}
	if scan.Err() != nil {
		result.Truncated = true
	}
	if item.Size > int64(total) {
		result.Truncated = true
	}
	return result, nil
}

func indexTitles(ctx context.Context, home string) (map[string]string, error) {
	out := map[string]string{}
	path := filepath.Join(home, "session_index.jsonl")
	if e := storage.RejectSymlinks(path); e != nil {
		return nil, e
	}
	f, e := os.Open(path)
	if errors.Is(e, fs.ErrNotExist) {
		return out, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 4096), 1<<20)
	for scan.Scan() {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		var v struct {
			ID   string `json:"id"`
			Name string `json:"thread_name"`
		}
		if json.Unmarshal(scan.Bytes(), &v) != nil {
			continue
		}
		v.Name = strings.TrimSpace(v.Name)
		if v.ID != "" && v.Name != "" {
			r := []rune(v.Name)
			if len(r) > 240 {
				r = r[:240]
			}
			out[v.ID] = string(r)
		}
	}
	if e = scan.Err(); e != nil {
		return nil, fault.Wrap("SESSION_INDEX_INVALID", "Session title index exceeds supported line size", e)
	}
	return out, nil
}
