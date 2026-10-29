package migration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"lam.local/lam2/internal/account"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/settings"
	"lam.local/lam2/internal/storage"
)

// Only metadata is stored in the preview. Credential files are neither copied
// nor embedded in a manifest; existing isolated homes remain usable in place.
func readMetadata(p *Plan, rel string, out any) (bool, error) {
	path, e := storage.SafeJoin(p.Root, rel)
	if e != nil {
		return false, e
	}
	raw, e := storage.ReadFile(path, 16<<20)
	if errors.Is(e, os.ErrNotExist) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	if e = json.Unmarshal(raw, out); e != nil {
		return false, fault.New("MIGRATION_INVALID", "Invalid metadata in "+rel)
	}
	p.Sources = append(p.Sources, Source{path, storage.Hash(raw)})
	return true, nil
}
func (s *Service) metadata(ctx context.Context, p *Plan, base model.Settings) error {
	var raw map[string]json.RawMessage
	has, e := readMetadata(p, "config/settings.json", &raw)
	if e != nil {
		return e
	}
	if has {
		next := base
		integer := func(k string, target *int) error {
			if v, ok := raw[k]; ok {
				if e := json.Unmarshal(v, target); e != nil {
					return fault.New("MIGRATION_INVALID", "Invalid setting: "+k)
				}
			}
			return nil
		}
		for k, target := range map[string]*int{"gatewayFirstResponseTimeoutSeconds": &next.Gateway.FirstResponseSeconds, "gatewayRequestTimeoutSeconds": &next.Gateway.RequestSeconds, "antigravityPort": &next.AntigravityPort} {
			if e = integer(k, target); e != nil {
				return e
			}
		}
		if v, ok := raw["hideDockIcon"]; ok {
			var hidden bool
			if json.Unmarshal(v, &hidden) != nil {
				return fault.New("MIGRATION_INVALID", "Invalid hideDockIcon")
			}
			next.ShowDock = !hidden
		}
		var terminal, preset, mode string
		_ = json.Unmarshal(raw["terminalTargetId"], &terminal)
		if terminal == "" {
			_ = json.Unmarshal(raw["selectedTerminalTarget"], &terminal)
		}
		switch strings.ToLower(terminal) {
		case "terminal", "terminal.app":
			next.Terminal = "terminal"
		case "ghostty", "cmux":
			next.Terminal = strings.ToLower(terminal)
		case "":
		default:
			p.Warnings = append(p.Warnings, "Unsupported legacy terminal retained in migration manifest: "+terminal)
		}
		_ = json.Unmarshal(raw["codexLaunchPermissionPreset"], &preset)
		switch preset {
		case "approveForMe":
			next.ApprovalPolicy = "never"
			next.SandboxMode = "workspace-write"
		case "fullAccess":
			next.ApprovalPolicy = "never"
			next.SandboxMode = "danger-full-access"
		case "askForApproval", "":
		default:
			return fault.New("MIGRATION_INVALID", "Unknown Codex permission preset")
		}
		_ = json.Unmarshal(raw["authMode"], &mode)
		if mode == "pat" {
			next.AccountMode = "pat"
		} else if mode == "oauth" {
			next.AccountMode = "profile"
		}
		if e = settings.Validate(next); e != nil {
			return e
		}
		p.Settings = &next
		p.LegacySettings = raw
	}
	var notes struct {
		Accounts map[string]struct{ Note, RenewalDate string }
	}
	if _, e = readMetadata(p, "config/account-notes.json", &notes); e != nil {
		return e
	}
	for i := range p.Profiles {
		if e = ctx.Err(); e != nil {
			return e
		}
		pr := &p.Profiles[i]
		if !storage.ValidID(pr.ID) {
			return fault.New("MIGRATION_INVALID", "Unsafe profile ID")
		}
		if note, ok := notes.Accounts[pr.ID]; ok {
			pr.Note, pr.RenewalDate = note.Note, note.RenewalDate
		}
		var proxy struct {
			Enabled bool
			URL     string
		}
		if found, e := readMetadata(p, "config/account-proxies/"+pr.ID+".json", &proxy); e != nil {
			return e
		} else if found {
			pr.Proxy = model.Proxy{Mode: "disabled"}
			if proxy.Enabled {
				pr.Proxy = model.Proxy{Mode: "manual", All: proxy.URL}
			}
			if e = account.ValidateProxy(pr.Proxy); e != nil {
				return e
			}
		}
		var auth struct{ TokenExpiration *time.Time }
		if _, e = readMetadata(p, "config/auth-metadata/"+pr.ID+".json", &auth); e != nil {
			return e
		}
		pr.TokenExpiration = auth.TokenExpiration
	}
	var timeline struct{ Events []model.PATPeriod }
	if _, e = readMetadata(p, "config/pat-usage-timeline.json", &timeline); e != nil {
		return e
	}
	for _, v := range timeline.Events {
		if !storage.ValidID(v.ProfileID) || v.StartedAt.IsZero() || (v.EndedAt != nil && v.EndedAt.Before(v.StartedAt)) {
			return fault.New("MIGRATION_INVALID", "Invalid PAT attribution period")
		}
	}
	p.PATTimeline = timeline.Events
	return nil
}
