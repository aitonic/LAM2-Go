// Package handoff copies one reviewed session, never a profile or credentials.
package handoff

import (
	"bytes"
	"context"
	"encoding/json"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/session"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Service struct {
	Store    *storage.Store
	Sessions *session.Service
}
type Plan struct {
	Action          string               `json:"action"`
	Compatibility   *CompatibilityReport `json:"compatibility,omitempty"`
	ID              string               `json:"id"`
	Source          model.Session        `json:"source"`
	TargetProfileID string               `json:"targetProfileId"`
	TargetPath      string               `json:"targetPath"`
	TargetSessionID string               `json:"targetSessionId"`
	Policy          string               `json:"policy"`
	SourceHash      string               `json:"sourceHash"`
	TargetHash      string               `json:"targetHash"`
	Revision        uint64               `json:"revision"`
	CreatedAt       time.Time            `json:"createdAt"`
	Warnings        []string             `json:"warnings"`
	Blocked         []string             `json:"blocked"`
	Fingerprint     string               `json:"fingerprint"`
}
type Result struct {
	Action       string `json:"action"`
	BackupPath   string `json:"backupPath,omitempty"`
	ForkPath     string `json:"forkPath,omitempty"`
	HandoffPath  string `json:"handoffPath,omitempty"`
	ResumePrompt string `json:"resumePrompt,omitempty"`
	Path         string `json:"path"`
	SessionID    string `json:"sessionId"`
	Idempotent   bool   `json:"idempotent"`
}
type Lineage struct {
	OperationID     string    `json:"operationId"`
	SourceProfileID string    `json:"sourceProfileId"`
	SourceSessionID string    `json:"sourceSessionId"`
	TargetProfileID string    `json:"targetProfileId"`
	TargetSessionID string    `json:"targetSessionId"`
	TargetPath      string    `json:"targetPath"`
	CopiedBytes     int64     `json:"copiedBytes"`
	CopiedHash      string    `json:"copiedHash"`
	Stage           string    `json:"stage"`
	CreatedAt       time.Time `json:"createdAt"`
}

func fingerprint(p Plan) string { p.Fingerprint = ""; b, _ := json.Marshal(p); return storage.Hash(b) }
func (s *Service) planPath(id string) (string, error) {
	if !storage.ValidID(id) {
		return "", fault.New("INVALID_PLAN", "Invalid handoff plan identifier")
	}
	return storage.SafeJoin(s.Store.Root, filepath.Join("plans", "handoff-"+id+".json"))
}
func (s *Service) Plan(ctx context.Context, sourceID, sessionID, targetID, policy string, allowProviderChange bool) (Plan, error) {
	var p Plan
	if sourceID == targetID {
		return p, fault.New("HANDOFF_SAME_PROFILE", "Source and target must be different profiles")
	}
	policy = normalizePolicy(policy)
	if !validPolicy(policy) {
		return p, fault.New("HANDOFF_POLICY_INVALID", "Choose a supported divergence strategy")
	}
	st, err := s.Store.Read(ctx)
	if err != nil {
		return p, err
	}
	src, ok := st.Profiles[sourceID]
	if !ok {
		return p, fault.New("PROFILE_NOT_FOUND", "Source profile not found")
	}
	dst, ok := st.Profiles[targetID]
	if !ok {
		return p, fault.New("PROFILE_NOT_FOUND", "Target profile not found")
	}
	if src.Home == dst.Home {
		return p, fault.New("HANDOFF_SHARED_HOME", "Shared PAT profiles cannot hand off into the same Codex home")
	}
	item, err := s.Sessions.Resolve(ctx, sourceID, sessionID)
	if err != nil {
		return p, err
	}
	sourceRoot := filepath.Join(src.Home, "sessions")
	rel, err := filepath.Rel(sourceRoot, item.Path)
	if err != nil {
		return p, err
	}
	if _, err = storage.SafeJoin(sourceRoot, rel); err != nil {
		return p, err
	}
	targetSessionID := item.ID
	if policy == "fork" {
		targetSessionID = storage.NewID()
		rel = filepath.Join(time.Now().Format("2006/01/02"), "rollout-"+time.Now().Format("2006-01-02T15-04-05")+"-"+targetSessionID+".jsonl")
	}
	targetPath, err := storage.SafeJoin(filepath.Join(dst.Home, "sessions"), rel)
	if err != nil {
		return p, err
	}
	srcHash, err := storage.FileHash(item.Path)
	if err != nil {
		return p, err
	}
	dstHash, err := storage.FileHash(targetPath)
	if err != nil {
		return p, err
	}
	p = Plan{ID: storage.NewID(), Source: item, TargetProfileID: targetID, TargetPath: targetPath, TargetSessionID: targetSessionID, Policy: policy, SourceHash: srcHash, TargetHash: dstHash, Revision: st.Revision, CreatedAt: time.Now().UTC(), Warnings: []string{}, Blocked: []string{}}
	p.Action = "copied"
	if policy == "fork" {
		p.Action = "fork"
	}
	if dstHash != storage.MissingHash && policy != "fork" {
		input, e := storage.ReadFile(item.Path, branchReadLimit)
		if e != nil {
			return p, e
		}
		target, e := storage.ReadFile(targetPath, branchReadLimit)
		if e != nil {
			return p, e
		}
		if storage.Hash(input) != srcHash || storage.Hash(target) != dstHash {
			return p, fault.New("PLAN_STALE", "Session changed during preview")
		}
		p.Action = relation(input, target)
		if p.Action == "diverged" && policy == "stop" {
			p.Blocked = append(p.Blocked, "Histories diverged; choose a branch-preserving strategy")
		}
		if p.Action != "already_current" {
			p.Warnings = append(p.Warnings, "The existing target is backed up before applying the chosen strategy")
		}
	}
	from, fromOK := st.Bindings[sourceID]
	to, toOK := st.Bindings[targetID]
	if fromOK && toOK && (from.ProviderID != to.ProviderID || from.Model != to.Model) {
		provider, found := st.Providers[to.ProviderID]
		if !found {
			return p, fault.New("PROVIDER_NOT_FOUND", "Target provider missing")
		}
		input, e := storage.ReadFile(item.Path, branchReadLimit)
		if e != nil {
			return p, e
		}
		if storage.Hash(input) != srcHash {
			return p, fault.New("PLAN_STALE", "Source changed during compatibility analysis")
		}
		report := AnalyzeCompatibility(input, provider.ModelCapabilities(to.Model).FunctionTools == "supported")
		p.Compatibility = &report
		if report.Disposition == "blocked" {
			p.Blocked = append(p.Blocked, "History is not portable to the target provider; confirmation cannot override protocol loss")
		}
		p.Warnings = append(p.Warnings, "Target provider/model differs; execution behavior may change")
	} else if toOK && (item.Model != "" && item.Model != to.Model) {
		p.Warnings = append(p.Warnings, "Target model differs from recorded session model")
	}
	_ = allowProviderChange // retained for representation-only loss in future versions
	p.Fingerprint = fingerprint(p)
	path, _ := s.planPath(p.ID)
	raw, _ := json.MarshalIndent(p, "", "  ")
	return p, storage.AtomicWrite(ctx, path, raw, 0600)
}
func (s *Service) Execute(ctx context.Context, id, confirmation string) (Result, error) {
	var result Result
	path, err := s.planPath(id)
	if err != nil {
		return result, err
	}
	raw, err := storage.ReadFile(path, 4<<20)
	if err != nil {
		return result, err
	}
	var p Plan
	if json.Unmarshal(raw, &p) != nil || p.ID != id {
		return result, fault.New("INVALID_PLAN", "Invalid handoff plan")
	}
	if p.Fingerprint != confirmation || fingerprint(p) != p.Fingerprint {
		return result, fault.New("CONFIRMATION_REQUIRED", "Confirm the exact handoff preview fingerprint")
	}
	if len(p.Blocked) > 0 {
		return result, fault.New("HANDOFF_CONFLICT", "The handoff preview contains unresolved conflicts")
	}
	if time.Since(p.CreatedAt) > 30*time.Minute {
		return result, fault.New("PLAN_STALE", "The handoff preview expired")
	}
	unlock, err := storage.Lock(ctx, filepath.Join(s.Store.Root, "handoff.lock"))
	if err != nil {
		return result, err
	}
	defer unlock()
	st, err := s.Store.Read(ctx)
	if err != nil {
		return result, err
	}
	if st.Revision != p.Revision {
		return result, fault.New("PLAN_STALE", "Profile or provider state changed")
	}
	src, ok := st.Profiles[p.Source.ProfileID]
	if !ok {
		return result, fault.New("PLAN_STALE", "Source profile changed")
	}
	dst, ok := st.Profiles[p.TargetProfileID]
	if !ok {
		return result, fault.New("PLAN_STALE", "Target profile changed")
	}
	if !storage.Within(p.Source.Path, []string{filepath.Join(src.Home, "sessions")}) || !storage.Within(p.TargetPath, []string{filepath.Join(dst.Home, "sessions")}) || filepath.Ext(p.Source.Path) != ".jsonl" || filepath.Ext(p.TargetPath) != ".jsonl" {
		return result, fault.New("PATH_UNSAFE", "Handoff paths must remain under their sessions roots")
	}
	sh, err := storage.FileHash(p.Source.Path)
	if err != nil {
		return result, err
	}
	dh, err := storage.FileHash(p.TargetPath)
	if err != nil {
		return result, err
	}
	if sh != p.SourceHash || dh != p.TargetHash {
		return result, fault.New("PLAN_STALE", "Source or target changed after preview")
	}
	result = Result{Path: p.TargetPath, SessionID: p.TargetSessionID, Action: p.Action}
	if p.Policy != "fork" && (sh == dh || p.Action == "already_current") {
		result.Idempotent = true
		return result, nil
	}
	if p.Action == "diverged" && branchPolicy(p.Policy) {
		input, e := storage.ReadFile(p.Source.Path, branchReadLimit)
		if e != nil {
			return result, e
		}
		target, e := storage.ReadFile(p.TargetPath, branchReadLimit)
		if e != nil {
			return result, e
		}
		if storage.Hash(input) != sh || storage.Hash(target) != dh {
			return result, fault.New("PLAN_STALE", "Session changed before branch write")
		}
		return s.executeBranch(ctx, p, src, dst, input, target)
	}
	lineage := Lineage{OperationID: id, SourceProfileID: src.ID, SourceSessionID: p.Source.ID, TargetProfileID: dst.ID, TargetSessionID: p.TargetSessionID, TargetPath: p.TargetPath, CopiedBytes: p.Source.Size, CopiedHash: sh, Stage: "prepared", CreatedAt: time.Now().UTC()}
	lp := filepath.Join(s.Store.Root, "handoffs", id+".json")
	saveLineage := func() error {
		b, _ := json.MarshalIndent(lineage, "", "  ")
		return storage.AtomicWrite(context.Background(), lp, b, 0600)
	}
	if dh != storage.MissingHash {
		backup := filepath.Join(s.Store.Root, "handoff-backups", id+".jsonl")
		result.BackupPath = backup
		if _, err = storage.CopyChecked(ctx, p.TargetPath, backup, dh, storage.MissingHash); err != nil {
			return result, err
		}
	}
	if p.Policy == "fork" {
		b, err := storage.ReadFile(p.Source.Path, 32<<20)
		if err != nil {
			return result, err
		}
		if storage.Hash(b) != sh {
			return result, fault.New("PLAN_STALE", "Source changed before fork")
		}
		forked, err := fork(b, p.Source.ID, p.TargetSessionID)
		if err != nil {
			return result, err
		}
		lineage.CopiedBytes = int64(len(forked))
		lineage.CopiedHash = storage.Hash(forked)
		if err = saveLineage(); err != nil {
			return result, err
		}
		current, err := storage.FileHash(p.TargetPath)
		if err != nil || current != dh {
			return result, fault.New("PLAN_STALE", "Target changed before fork commit")
		}
		if err = storage.AtomicWrite(ctx, p.TargetPath, forked, 0600); err != nil {
			return result, err
		}
	} else {
		if err = saveLineage(); err != nil {
			return result, err
		}
		if _, err = storage.CopyChecked(ctx, p.Source.Path, p.TargetPath, sh, dh); err != nil {
			return result, err
		}
	}
	lineage.Stage = "complete"
	if err = saveLineage(); err != nil {
		return result, fault.Wrap("HANDOFF_RECOVERY_REQUIRED", "The session was copied; its provenance marker needs recovery", err)
	}
	s.Sessions.Invalidate()
	return result, nil
}
func fork(raw []byte, oldID, newID string) ([]byte, error) {
	lines := bytes.SplitAfter(raw, []byte{'\n'})
	found := false
	for i, line := range lines {
		trim := bytes.TrimSpace(line)
		if len(trim) == 0 {
			continue
		}
		var v map[string]json.RawMessage
		if json.Unmarshal(trim, &v) != nil {
			return nil, fault.New("SESSION_CORRUPT", "A fork requires valid JSONL; the original was not modified")
		}
		var typ string
		_ = json.Unmarshal(v["type"], &typ)
		if typ != "session_meta" {
			continue
		}
		var meta map[string]any
		if json.Unmarshal(v["payload"], &meta) != nil || meta["id"] != oldID {
			return nil, fault.New("SESSION_CORRUPT", "Unexpected session identity")
		}
		meta["id"] = newID
		meta["forked_from_id"] = oldID
		payload, _ := json.Marshal(meta)
		v["payload"] = payload
		b, _ := json.Marshal(v)
		if strings.HasSuffix(string(line), "\n") {
			b = append(b, '\n')
		}
		lines[i] = b
		found = true
		break
	}
	if !found {
		return nil, fault.New("SESSION_CORRUPT", "Session metadata is missing")
	}
	return bytes.Join(lines, nil), nil
}

// Provenance returns only completed or checksum-recoverable copy records. An
// indexer uses CopiedBytes to attribute imported history to its source profile.
func (s *Service) Provenance(ctx context.Context) ([]Lineage, error) {
	root := filepath.Join(s.Store.Root, "handoffs")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Lineage
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := storage.ReadFile(filepath.Join(root, e.Name()), 1<<20)
		if err != nil {
			return nil, err
		}
		var l Lineage
		if json.Unmarshal(b, &l) != nil {
			continue
		}
		if l.Stage == "complete" {
			out = append(out, l)
		} else if l.Stage == "prepared" {
			h, _ := storage.FileHash(l.TargetPath)
			if h == l.CopiedHash {
				out = append(out, l)
			}
		}
	}
	return out, nil
}
