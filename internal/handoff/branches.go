package handoff

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
)

const branchReadLimit = 256 << 20

func normalizePolicy(v string) string {
	switch v {
	case "", "stop_and_ask":
		return "stop"
	case "prefer_source":
		return "replace"
	}
	return v
}
func validPolicy(v string) bool {
	switch v {
	case "stop", "replace", "fork", "prefer_target", "timeline_merge_to_fork", "summarize_fork_with_target_account":
		return true
	}
	return false
}
func branchPolicy(v string) bool {
	return v == "prefer_target" || v == "timeline_merge_to_fork" || v == "summarize_fork_with_target_account"
}
func relation(source, target []byte) string {
	if bytes.HasPrefix(target, source) {
		return "already_current"
	}
	if bytes.HasPrefix(source, target) {
		return "extended"
	}
	return "diverged"
}
func branchLines(raw []byte) []string {
	s := strings.TrimSuffix(strings.ToValidUTF8(string(raw), "\uFFFD"), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
func commonPrefix(a, b []string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}
func timelineMerge(a, b []byte) []byte {
	aa, bb := branchLines(a), branchLines(b)
	n := commonPrefix(aa, bb)
	type entry struct {
		line, stamp string
		index       int
	}
	entries := []entry{}
	for _, line := range append(append([]string{}, aa...), bb[n:]...) {
		var obj map[string]any
		_ = json.Unmarshal([]byte(line), &obj)
		stamp := ""
		for _, key := range []string{"timestamp", "created_at", "createdAt"} {
			if s, ok := obj[key].(string); ok {
				stamp = s
				break
			}
		}
		entries = append(entries, entry{line, stamp, len(entries)})
	}
	// Timestamped runs are stable; metadata lines retain their positions. This
	// avoids the non-transitive mixed timestamp comparator of the old program.
	for start := 0; start < len(entries); {
		if entries[start].stamp == "" {
			start++
			continue
		}
		end := start
		for end < len(entries) && entries[end].stamp != "" {
			end++
		}
		sort.SliceStable(entries[start:end], func(i, j int) bool { return entries[start+i].stamp < entries[start+j].stamp })
		start = end
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.line
	}
	if len(out) == 0 {
		return nil
	}
	return []byte(strings.Join(out, "\n") + "\n")
}
func branchExcerpt(lines []string) string {
	if len(lines) > 80 {
		lines = lines[len(lines)-80:]
	}
	return strings.Join(lines, "\n")
}
func (s *Service) executeBranch(ctx context.Context, p Plan, src, dst model.Profile, source, target []byte) (Result, error) {
	r := Result{Path: p.TargetPath, SessionID: p.TargetSessionID, Action: p.Policy}
	backup := filepath.Join(s.Store.Root, "handoff-backups", p.ID+".jsonl")
	if _, e := storage.CopyChecked(ctx, p.TargetPath, backup, p.TargetHash, storage.MissingHash); e != nil {
		return r, e
	}
	r.BackupPath = backup
	var path string
	var data []byte
	if p.Policy == "summarize_fork_with_target_account" {
		path = filepath.Join(dst.Home, ".lam-handoffs", "session-"+p.Source.ID+"-"+p.ID+".md")
		a, b := branchLines(source), branchLines(target)
		n := commonPrefix(a, b)
		data = []byte(fmt.Sprintf("# Diverged Session Handoff\n\nSession: %s\nSource account: %s\nTarget account: %s\nCommon prefix lines: %d\nSource branch lines: %d\nTarget branch lines: %d\n\n## Source branch excerpt\n\n```jsonl\n%s\n```\n\n## Target branch excerpt\n\n```jsonl\n%s\n```\n", p.Source.ID, src.ID, dst.ID, n, len(a)-n, len(b)-n, branchExcerpt(a[n:]), branchExcerpt(b[n:])))
		r.HandoffPath = path
		r.ResumePrompt = fmt.Sprintf("A diverged branch handoff was written at %s. Read it, summarize the source branch into this target-account session context, preserve the target branch as the active timeline, then state that the handoff has been incorporated.", path)
	} else {
		path = p.TargetPath + ".fork-" + p.ID
		data = source
		if p.Policy == "timeline_merge_to_fork" {
			data = timelineMerge(source, target)
		}
		r.ForkPath = path
	}
	if !storage.Within(path, []string{dst.Home}) {
		return r, fault.New("PATH_UNSAFE", "Branch output escaped target home")
	}
	if h, e := storage.FileHash(p.TargetPath); e != nil || h != p.TargetHash {
		return r, fault.New("PLAN_STALE", "Target changed during branch preparation")
	}
	if h, e := storage.FileHash(p.Source.Path); e != nil || h != p.SourceHash {
		return r, fault.New("PLAN_STALE", "Source changed during branch preparation")
	}
	if e := storage.AtomicWrite(ctx, path, data, 0600); e != nil {
		return r, e
	}
	// Auxiliary branches intentionally do not become a second active .jsonl
	// session with the same ID; this matches old .jsonl.fork-* preservation.
	return r, nil
}
