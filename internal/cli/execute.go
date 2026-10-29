package cli

import (
	"context"
	"encoding/json"
	"io"
	"lam.local/lam2/internal/account"
	"lam.local/lam2/internal/app"
	"lam.local/lam2/internal/commands"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	run "lam.local/lam2/internal/runtime"
	"lam.local/lam2/internal/session"
	"lam.local/lam2/internal/storage"
	"lam.local/lam2/internal/usage"
	"os"
	"strings"
	"time"
)

func input(path string, in io.Reader) ([]byte, error) {
	if path != "" && path != "-" {
		return storage.ReadFile(path, 8<<20)
	}
	raw, e := io.ReadAll(io.LimitReader(in, (8<<20)+1))
	if e != nil {
		return nil, e
	}
	if len(raw) > 8<<20 {
		return nil, fault.New("INPUT_TOO_LARGE", "Input exceeds 8 MiB")
	}
	return raw, nil
}
func decode(path string, in io.Reader, out any) error {
	b, e := input(path, in)
	if e != nil {
		return e
	}
	if json.Unmarshal(b, out) != nil {
		return fault.New("INPUT_INVALID", "Input is not valid JSON for this operation")
	}
	return nil
}
func require(confirm bool) error {
	if !confirm {
		return fault.New("CONFIRMATION_REQUIRED", "Pass --confirm only after reviewing this operation")
	}
	return nil
}
func date(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	if t, e := time.Parse(time.RFC3339, raw); e == nil {
		return t, nil
	}
	t, e := time.ParseInLocation("2006-01-02", raw, time.Local)
	if e != nil {
		return t, fault.New("DATE_INVALID", "Use RFC3339 or YYYY-MM-DD")
	}
	return t, nil
}

// Execute writes only the requested result. The auth command is deliberately
// not passed through the JSON encoder, ensuring Codex receives one token only.
func Execute(ctx context.Context, c *app.Core, v commands.Invocation, in io.Reader, out, stderr io.Writer) error {
	if v.Mode == "auth" {
		token, e := c.Providers.AuthToken(ctx, v.ID)
		if e != nil {
			return e
		}
		_, e = io.WriteString(out, token+"\n")
		return e
	}
	if v.Mode == "launch" {
		p, e := c.LaunchPlan(ctx, v.Profile, v.Resume, v.CWD, v.Extra)
		if e != nil {
			return e
		}
		return run.Run(ctx, p, in, out, stderr)
	}
	if v.Mode == "gateway" {
		switch v.Action {
		case "":
			return c.RunGateway(ctx, v.Listen)
		case "stop":
			if e := require(v.Confirm); e != nil {
				return e
			}
			return c.Gateway.Stop(ctx)
		case "status":
			_, h, e := c.Gateway.Probe(ctx)
			if e != nil {
				return e
			}
			return json.NewEncoder(out).Encode(h)
		default:
			return fault.New("ACTION_INVALID", "Unknown gateway action")
		}
	}
	value, e := execute(ctx, c, v, in)
	if e != nil {
		return e
	}
	if value == nil {
		value = map[string]bool{"ok": true}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
func execute(ctx context.Context, c *app.Core, v commands.Invocation, in io.Reader) (any, error) {
	switch v.Mode {
	case "doctor":
		return c.Doctor(ctx), nil
	case "profile":
		switch v.Action {
		case "list":
			return c.Accounts.List(ctx)
		case "discover":
			home := v.Path
			if home == "" {
				home, _ = os.UserHomeDir()
			}
			return account.Discover(home)
		case "create":
			return c.CreateProfile(ctx, v.Name, v.AuthMode, v.Revision, v.Target)
		case "plan-relay":
			var request account.RelayRequest
			if e := decode(v.Input, in, &request); e != nil {
				return nil, e
			}
			return c.Accounts.PlanRelay(ctx, request)
		case "create-relay":
			if e := require(v.Confirm); e != nil {
				return nil, e
			}
			return c.CreateRelay(ctx, v.ID, v.Fingerprint)

		case "register":
			return c.Accounts.Register(ctx, v.Path, v.Name, v.Revision)
		case "edit":
			var p model.Profile
			if e := decode(v.Input, in, &p); e != nil {
				return nil, e
			}
			id := v.Profile
			if id == "" {
				id = p.ID
			}
			return nil, c.Accounts.Edit(ctx, id, p.Name, p.Note, p.RenewalDate, p.Proxy, v.Revision)
		case "delete":
			return nil, c.Accounts.Delete(ctx, v.Profile, v.Revision, v.Confirm)
		case "login":
			key := ""
			if v.SecretFile != "" {
				b, e := input(v.SecretFile, in)
				if e != nil {
					return nil, e
				}
				key = strings.TrimSpace(string(b))
			}
			return c.Login(ctx, v.Profile, v.Kind, key)
		case "read":
			return c.AccountRead(ctx, v.Profile)
		case "import":
			if e := require(v.Confirm); e != nil {
				return nil, e
			}
			b, e := input(v.Input, in)
			if e != nil {
				return nil, e
			}
			return nil, c.Accounts.ImportAuth(ctx, v.Profile, b, true)
		case "export":
			if e := require(v.Confirm); e != nil {
				return nil, e
			}
			b, e := c.Accounts.ExportCPA(ctx, v.Profile, true)
			if e != nil {
				return nil, e
			}
			if v.Output == "-" {
				return json.RawMessage(b), nil
			}
			return nil, storage.AtomicWrite(ctx, v.Output, b, 0600)
		case "activate":
			return nil, c.Accounts.ActivatePAT(ctx, v.Profile, v.Confirm)
		case "restore-pat":
			return nil, c.Accounts.RestorePAT(ctx, v.Confirm)
		case "repair-wrapper":
			return nil, c.RepairWrapper(ctx, v.Profile)
		case "threads":
			return c.Threads(ctx, v.Profile, v.Cursor)
		}
	case "provider":
		switch v.Action {
		case "test":
			return c.Providers.TestConnection(ctx, v.ID)
		case "list":
			return c.Providers.List(ctx)
		case "save":
			var p model.Provider
			if e := decode(v.Input, in, &p); e != nil {
				return nil, e
			}
			secret := ""
			if v.SecretFile != "" {
				b, e := input(v.SecretFile, in)
				if e != nil {
					return nil, e
				}
				secret = strings.TrimSpace(string(b))
			}
			return c.Providers.Save(ctx, p, secret, v.Revision)
		case "delete":
			return nil, c.Providers.Delete(ctx, v.ID, v.Revision, v.Confirm)
		case "discover":
			st, e := c.Store.Read(ctx)
			if e != nil {
				return nil, e
			}
			p, ok := st.Providers[v.ID]
			if !ok {
				return nil, fault.New("PROVIDER_NOT_FOUND", "Provider not found")
			}
			return c.Providers.Discover(ctx, p)
		case "plan-bind":
			return c.Providers.PlanBind(ctx, v.Profile, v.Provider, v.Model)
		case "bind":
			if e := require(v.Confirm); e != nil {
				return nil, e
			}
			return c.Providers.CommitBind(ctx, v.ID, v.Fingerprint)
		case "plan-detach":
			return c.Providers.PlanDetach(ctx, v.Profile)
		case "detach":
			if e := require(v.Confirm); e != nil {
				return nil, e
			}
			if v.ID != "" {
				return nil, c.Providers.CommitDetach(ctx, v.ID, v.Fingerprint)
			}
			plan, e := c.Providers.PlanDetach(ctx, v.Profile)
			if e != nil {
				return nil, e
			}
			return nil, c.Providers.CommitDetach(ctx, plan.ID, plan.Fingerprint)
		case "approve-command":
			var a struct {
				Executable string
				Args       []string
			}
			if e := decode(v.Input, in, &a); e != nil {
				return nil, e
			}
			return c.Providers.ApproveCommand(ctx, a.Executable, a.Args, v.Revision, v.Confirm)
		}
	case "session":
		switch v.Action {
		case "list":
			a, e := date(v.After)
			if e != nil {
				return nil, e
			}
			b, e := date(v.Before)
			if e != nil {
				return nil, e
			}
			return c.Sessions.Query(ctx, session.Query{ProfileID: v.Profile, Search: v.Search, Workspace: v.Workspace, After: a, Before: b, Sort: v.Sort, Descending: true, Offset: v.Offset, Limit: v.Limit})
		case "read":
			return c.Sessions.Read(ctx, v.Profile, v.ID)
		case "plan-delete":
			var refs []session.Ref
			if v.ID != "" {
				refs = []session.Ref{{ProfileID: v.Profile, SessionID: v.ID}}
			} else if e := decode(v.Input, in, &refs); e != nil {
				return nil, e
			}
			return c.Sessions.PlanDelete(ctx, refs)
		case "archive":
			return nil, c.Sessions.Delete(ctx, v.ID, v.Confirm)
		case "restore":
			return nil, c.Sessions.Restore(ctx, v.ID, v.Confirm)
		}
	case "handoff":
		switch v.Action {
		case "plan":
			return c.Handoff.Plan(ctx, v.Profile, v.ID, v.Target, v.Policy, v.AllowProviderChange)
		case "latest":
			list, e := c.Sessions.Query(ctx, session.Query{ProfileID: v.Profile, Sort: "modified", Descending: true, Limit: 1})
			if e != nil {
				return nil, e
			}
			if len(list.Items) == 0 {
				return nil, fault.New("SESSION_NOT_FOUND", "No source session")
			}
			return c.Handoff.Plan(ctx, v.Profile, list.Items[0].ID, v.Target, v.Policy, v.AllowProviderChange)
		case "execute":
			if e := require(v.Confirm); e != nil {
				return nil, e
			}
			return c.Handoff.Execute(ctx, v.ID, v.Fingerprint)
		}
	case "quota":
		switch v.Action {
		case "read":
			return c.Quota.Read(ctx, v.Profile)
		case "refresh":
			ids := []string{}
			if v.Profile != "" {
				ids = []string{v.Profile}
			}
			return c.RefreshQuotas(ctx, ids)
		case "account-usage":
			return c.AccountUsage(ctx, v.Profile)
		case "antigravity":
			return c.Antigravity(ctx)
		case "reset-plan":
			id, e := c.Quota.PrepareReset(ctx, v.Profile, v.ID)
			return map[string]string{"idempotencyKey": id}, e
		case "reset-consume":
			return c.Quota.ConsumeReset(ctx, v.ID, v.Confirm)
		}
	case "usage":
		if v.Action == "index" {
			return c.RefreshUsage(ctx, v.Profile, nil)
		}
		if v.Action == "reset" || v.Action == "compact" || v.Action == "reprice" {
			return nil, c.MaintainUsage(ctx, v.Action, v.Confirm)
		}
		a, e := date(v.After)
		if e != nil {
			return nil, e
		}
		b, e := date(v.Before)
		if e != nil {
			return nil, e
		}
		f := usage.Filter{ProfileID: v.Profile, SessionID: v.ID, Workspace: v.Workspace, Model: v.Model, After: a, Before: b, Offset: v.Offset, Limit: v.Limit}
		d, e := c.DB(ctx)
		if e != nil {
			return nil, e
		}
		switch v.Action {
		case "summary":
			return d.Summary(ctx, f)
		case "calls":
			return d.Calls(ctx, f)
		case "threads":
			return d.Threads(ctx, f)
		case "daily":
			if f.After.IsZero() {
				f.After = time.Now().AddDate(0, 0, -30)
			}
			if f.Before.IsZero() {
				f.Before = time.Now().AddDate(0, 0, 1)
			}
			return d.Daily(ctx, f, time.Local)
		}
	case "settings":
		switch v.Action {
		case "read":
			return c.Store.Read(ctx)
		case "save":
			var s model.Settings
			if e := decode(v.Input, in, &s); e != nil {
				return nil, e
			}
			return nil, c.Settings.Save(ctx, s, v.Revision)
		case "prices":
			var rates []model.RateCard
			if e := decode(v.Input, in, &rates); e != nil {
				return nil, e
			}
			return nil, c.Settings.SavePrices(ctx, rates, v.Revision)
		}
	case "migrate":
		switch v.Action {
		case "plan":
			return c.Migration.Plan(ctx, v.Path)
		case "apply":
			if e := require(v.Confirm); e != nil {
				return nil, e
			}
			return nil, c.Migration.Apply(ctx, v.ID, v.Fingerprint)
		case "rollback":
			return nil, c.Migration.Rollback(ctx, v.ID, v.Confirm)
		}
	}
	return nil, fault.New("ACTION_INVALID", "Unknown operation")
}
