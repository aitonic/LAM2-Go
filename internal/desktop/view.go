package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
	"lam.local/lam2/internal/account"
	"lam.local/lam2/internal/app"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/session"
	"lam.local/lam2/internal/usage"
)

// All elements are built for this frame; no widget is retained in UI state.
func (u *UI) view(c *ui.Context) {
	ui.Row(c).Fill().Gap(0).Children(func() {
		page := u.router.Path()
		ui.Sidebar(c, &page, func() {
			for _, key := range []string{"overview", "usage", "sessions", "providers", "settings"} {
				ui.SidebarItem(c, "/"+key, nil, u.t(key))
			}
		}).Width(190).OnChange(func() { u.router.Push(page); u.reload() })
		ui.Column(c).Fill().Padding(20).Gap(12).Children(func() {
			ui.Row(c).Gap(10).Children(func() {
				ui.Text(c, "LAM 2").Bold().FontSize(23).Grow(1)
				if ui.Button(c, u.t("refresh")).Disabled(u.busy).Clicked() {
					u.reload()
				}
			})
			if u.errorText != "" {
				ui.Text(c, u.errorText).TextColor(c.Theme().TextMuted)
			}
			if u.reading {
				ui.Progress(c, -1).Label(u.t("refresh"))
			}
			if u.busy {
				ui.Row(c).Gap(8).Children(func() {
					ui.Progress(c, -1).Grow(1).Label(u.status)
					if ui.Button(c, u.t("cancel")).Clicked() && u.jobCancel != nil {
						u.jobCancel()
					}
				})
			}
			u.router.View(c, func(r *ui.Route) {
				switch {
				case r.Match("/overview"):
					r.Title(u.t("overview"))
					u.overview(c)
				case r.Match("/usage"):
					r.Title(u.t("usage"))
					u.usagePage(c)
				case r.Match("/sessions"):
					r.Title(u.t("sessions"))
					u.sessionsPage(c)
				case r.Match("/providers"):
					r.Title(u.t("providers"))
					u.providersPage(c)
				case r.Match("/settings"):
					r.Title(u.t("settings"))
					u.settingsPage(c)
				}
			})
			if u.status != "" {
				ui.Text(c, u.status).FontSize(11).TextColor(c.Theme().TextMuted)
			}
		})
	})
	u.dialogs(c)
}
func (u *UI) profileIDs(all bool) []string {
	out := []string{}
	if all {
		out = append(out, "")
	}
	for _, p := range u.basic.Profiles {
		out = append(out, p.ID)
	}
	return out
}
func (u *UI) overview(c *ui.Context) {
	p, has := u.profile()
	_, registered := u.basic.State.Profiles[p.ID]
	ready := has && registered
	ui.Row(c).Gap(6).Children(func() {
		if ui.Button(c, u.t("newProfile")).Disabled(u.busy).Clicked() {
			u.newProfile()
		}
		if ui.Button(c, u.t("discover")).Disabled(u.busy).Clicked() {
			u.reload()
		}
		if ui.Button(c, "Register discovered account").Disabled(!has || registered || u.busy).Clicked() {
			id, home, name, rev := p.ID, p.Home, p.Name, u.basic.State.Revision
			u.confirm("Register external Codex home", "Register "+name+" ("+id+") as a read-only-owned external profile. Existing authentication files remain untouched.", func() {
				u.task("Register Codex home", func(ctx context.Context) (any, error) {
					return u.core.Accounts.Register(ctx, home, name, rev)
				}, u.changed)
			})
		}
		if ui.Button(c, u.t("edit")).Disabled(!ready || u.busy).Clicked() {
			u.profileDraft = p
			u.profileEdit = true
			u.profileHome = ""
			u.profileOpen = true
		}
		if ui.Button(c, u.t("login")).Disabled(!ready || u.busy).Clicked() {
			u.loginKey = ""
			u.loginOpen = true
			u.bindProfile = p.ID
		}
		if ui.Button(c, u.t("launch")).Disabled(!ready || u.busy).Clicked() {
			u.launch(p.ID, "", "")
		}
		if ui.Button(c, u.t("refreshQuota")).Disabled(u.busy).Clicked() {
			u.refreshQuotas()
		}
	})
	ui.Row(c).Gap(6).Children(func() {
		if ui.Button(c, u.t("import")).Disabled(!ready || u.busy).Clicked() {
			u.importAuth(p.ID)
		}
		if ui.Button(c, "Paste Session / PAT").Disabled(!ready || u.busy).Clicked() {
			u.clearCredentialDraft()
			u.authProfile = p.ID
			u.authPurpose = "runtime"
			u.credentialOpen = true
		}
		if ui.Button(c, u.t("export")).Disabled(!ready || u.busy).Clicked() {
			u.exportAuth(p.ID)
		}
		if ui.Button(c, u.t("activate")).Disabled(!ready || p.ID == "main" || u.busy).Clicked() {
			id := p.ID
			u.confirm(u.t("activate"), "This replaces the shared Codex authentication. Other running clients may be affected.", func() {
				u.task(u.t("activate"), func(ctx context.Context) (any, error) { return nil, u.core.Accounts.ActivatePAT(ctx, id, true) }, u.changed)
			})
		}
		if ui.Button(c, u.t("restorePAT")).Disabled(u.basic.State.ActivePAT == "" || u.busy).Clicked() {
			u.confirm(u.t("restorePAT"), "Restore the saved shared authentication only if it has not been externally modified.", func() {
				u.task(u.t("restorePAT"), func(ctx context.Context) (any, error) { return nil, u.core.Accounts.RestorePAT(ctx, true) }, u.changed)
			})
		}
		if ui.Button(c, u.t("remove")).Disabled(!ready || u.busy).Clicked() {
			id, rev := p.ID, u.basic.State.Revision
			_, bound := u.basic.State.Bindings[id]
			u.confirm(u.t("remove"), "Remove this profile. A LAM-owned home is moved to a recoverable archive; external homes stay unchanged. Active PAT profiles must be restored first.", func() {
				u.task(u.t("remove"), func(ctx context.Context) (any, error) {
					if bound {
						return nil, u.core.APIAccounts.Delete(ctx, id, true)
					}
					return nil, u.core.Accounts.Delete(ctx, id, rev, true)
				}, u.changed)
			})
		}
	})
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, "Create relay profile").Disabled(!ready || u.busy).Clicked() {
			u.editJSON("Relay profile metadata", account.RelayRequest{SourceProfileID: p.ID, RuntimeProfileID: p.ID, ProviderPolicy: "keep_target"}, func(raw string) {
				var request account.RelayRequest
				if e := json.Unmarshal([]byte(raw), &request); e != nil {
					u.fail(e)
					return
				}
				u.jsonOpen = false
				u.task("Preview relay profile", func(ctx context.Context) (any, error) { return u.core.Accounts.PlanRelay(ctx, request) }, func(v any) {
					plan := v.(account.RelayPlan)
					u.confirm("Create relay profile", "Create a separate, unauthenticated profile. No credentials or sessions are copied. Login and select an individual Handoff separately.\n"+app.Pretty(plan), func() {
						u.task("Create relay profile", func(ctx context.Context) (any, error) { return u.core.CreateRelay(ctx, plan.ID, plan.Fingerprint) }, u.changed)
					})
				})
			})
		}
		b, bound := u.basic.State.Bindings[p.ID]
		if ui.Button(c, "API connection / model").Disabled(!ready || !bound || u.busy).Clicked() {
			u.bindProfile = p.ID
			u.bindProvider = b.ProviderID
			u.bindModel = b.Model
			u.bindOpen = true
		}
		if ui.Button(c, "Edit API connection").Disabled(!ready || !bound || u.busy).Clicked() {
			if pr, ok := u.basic.State.Providers[b.ProviderID]; ok {
				u.openProvider(pr)
			}
		}
		if ui.Button(c, "Credential expiration").Disabled(!ready || u.busy).Clicked() {
			id := p.ID
			u.task("Credential expiration", func(ctx context.Context) (any, error) { return u.core.Accounts.Expiration(ctx, id) }, func(v any) { u.info("Credential expiration", v) })
		}
	})
	u.profileTable.Selected = &u.profileRow
	u.profileTable.Key = func(i int) any { return u.basic.Profiles[i].ID }
	cols := []ui.TableColumn{{Title: u.t("name")}, {Title: "ID", Width: 160}, {Title: "Mode", Width: 75}, {Title: "Authentication", Width: 140}, {Title: "Provider", Width: 170}}
	ui.Table(c, &u.profileTable, cols, len(u.basic.Profiles), func(row, col int) {
		p := u.basic.Profiles[row]
		v := ""
		switch col {
		case 0:
			v = p.Name
		case 1:
			v = p.ID
		case 2:
			v = p.AuthMode
		case 3:
			v = p.AuthStatus
			if _, ok := u.basic.State.Profiles[p.ID]; !ok {
				v = "Discovered (register to use)"
			}
			if v == "" {
				v = u.t("unknown")
			}
		case 4:
			if b, ok := u.basic.State.Bindings[p.ID]; ok {
				v = b.ProviderID + " / " + b.Model
			}
		}
		ui.Text(c, v).SingleLine()
	}).Height(235)
	if len(u.basic.Profiles) == 0 {
		ui.Text(c, u.t("empty"))
	}
	ui.Scroll(c).Grow(1).Gap(10).Children(func() {
		for _, p := range u.basic.Profiles {
			q, ok := u.basic.Quotas[p.ID]
			ui.Column(c).Padding(12).Gap(5).Children(func() {
				ui.Text(c, p.Name).Bold()
				if p.TokenExpiration != nil {
					remaining := time.Until(*p.TokenExpiration)
					if remaining <= 0 {
						ui.Text(c, "Token expired").FontSize(11)
					} else {
						ui.Text(c, fmt.Sprintf("Token expires in %d h %d m", int(remaining.Hours()), int(remaining.Minutes())%60)).FontSize(11)
					}
				}
				if !ok {
					ui.Text(c, u.t("unknown"))
					return
				}
				u.quotaView(c, q)
			})
		}
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "Antigravity").Disabled(u.busy).Clicked() {
				u.task("Antigravity", func(ctx context.Context) (any, error) {
					q, e := u.core.Antigravity(ctx)
					if e != nil && q.Source == "" {
						return nil, e
					}
					return q, nil
				}, func(v any) { u.quotaExtra = v.(model.QuotaSnapshot) })
			}
			if ui.Button(c, u.t("accountUsage")).Disabled(!ready || u.busy).Clicked() {
				id := p.ID
				u.task(u.t("accountUsage"), func(ctx context.Context) (any, error) { return u.core.AccountUsage(ctx, id) }, func(v any) { u.info(u.t("accountUsage"), v) })
			}
			if ui.Button(c, u.t("resetCredit")).Disabled(!ready || u.busy).Clicked() {
				u.resetCredit(p.ID)
			}
		})
		if u.quotaExtra.Source != "" {
			u.quotaView(c, u.quotaExtra)
		}
	})
}
func (u *UI) quotaView(c *ui.Context, q model.QuotaSnapshot) {
	status := q.Status
	if q.Stale || (!q.ExpiresAt.IsZero() && time.Now().After(q.ExpiresAt)) {
		status += " / stale"
	}
	ui.Text(c, q.Source+" | "+q.PlanType+" | "+status).FontSize(12)
	for _, w := range q.Windows {
		label := w.ID + ": " + QuotaLabel(w.RemainingPercent)
		if w.ResetsAt != nil {
			label += " | " + w.ResetsAt.Local().Format("01-02 15:04")
		}
		ui.Text(c, label)
		if w.RemainingPercent != nil {
			ui.Progress(c, math.Max(0, math.Min(1, *w.RemainingPercent/100))).Label(w.ID)
		}
	}
	for _, group := range q.Groups {
		ui.Text(c, group.Name).Bold()
		for _, b := range group.Buckets {
			ui.Text(c, app.Pretty(b)).FontSize(12)
		}
	}
	if q.Credits != nil {
		if q.Credits.AvailableCount != nil {
			ui.Text(c, fmt.Sprintf("Reset credits: %d", *q.Credits.AvailableCount))
		} else {
			ui.Text(c, "Reset credits: unknown")
		}
		if q.Credits.ExpiresAt != nil {
			ui.Text(c, "Credit expiry: "+q.Credits.ExpiresAt.Local().Format("2006-01-02 15:04")).FontSize(11)
		}
	}
	if q.ErrorCode != "" {
		ui.Text(c, q.ErrorCode)
	}
	if !q.FetchedAt.IsZero() {
		ui.Text(c, q.FetchedAt.Local().Format("2006-01-02 15:04:05")).FontSize(11)
	}
}
func (u *UI) sessionsPage(c *ui.Context) {
	ui.Row(c).Gap(8).Children(func() {
		ui.TextInput(c, &u.query.Search).Label(u.t("search")).Grow(1)
		ui.Select(c, &u.queryProfile, u.profileIDs(true)).Label(u.t("allProfiles")).Width(200)
		ui.Select(c, &u.query.Sort, []string{"modified", "size", "title"}).Label("Sort").Width(110)
		if ui.Button(c, u.t("search")).Disabled(u.busy).Clicked() {
			u.query.Offset = 0
			u.reload()
		}
	})
	s, has := u.selectedSession()
	ui.Row(c).Gap(6).Children(func() {
		if ui.Button(c, u.t("details")).Disabled(!has || u.busy).Clicked() {
			u.task(u.t("details"), func(ctx context.Context) (any, error) { return u.core.Sessions.Read(ctx, s.ProfileID, s.ID) }, func(v any) { u.info(u.t("details"), v) })
		}
		if ui.Button(c, u.t("resume")).Disabled(!has || u.busy).Clicked() {
			u.launch(s.ProfileID, s.ID, s.CWD)
		}
		if ui.Button(c, "Copy resume command").Disabled(!has || u.busy).Clicked() {
			u.copyResume(s.ProfileID, s.ID, s.CWD)
		}
		if ui.Button(c, u.t("handoff")).Disabled(!has || u.busy).Clicked() {
			u.handoffSource = s.ProfileID
			u.handoffSession = s.ID
			u.handoffTarget = ""
			u.handoffOpen = true
		}
		if ui.Button(c, u.t("selectPage")).Clicked() {
			for _, s := range u.sessions.Items {
				u.selected[s.ProfileID+"/"+s.ID] = session.Ref{ProfileID: s.ProfileID, SessionID: s.ID}
			}
		}
		if ui.Button(c, "Select all filtered deletable").Disabled(u.busy).Clicked() {
			q := u.query
			q.ProfileID = u.queryProfile
			u.task("Select filtered sessions", func(ctx context.Context) (any, error) { return u.core.Sessions.SelectFiltered(ctx, q, true) }, func(v any) {
				u.selected = map[string]session.Ref{}
				for _, r := range v.([]session.Ref) {
					u.selected[r.ProfileID+"/"+r.SessionID] = r
				}
			})
		}
		if ui.Button(c, "Storage summary").Disabled(u.busy).Clicked() {
			id := u.queryProfile
			u.task("Session storage", func(ctx context.Context) (any, error) { return u.core.Sessions.StorageSummary(ctx, id) }, func(v any) { u.info("Session storage", v) })
		}
		if ui.Button(c, u.t("clearSelection")).Clicked() {
			u.selected = map[string]session.Ref{}
		}
		if ui.Button(c, u.t("archive")).Disabled(len(u.selected) == 0 || u.busy).Clicked() {
			u.archiveSelected()
		}
	})
	u.sessionTable.Selected = &u.sessionRow
	u.sessionTable.Key = func(i int) any { return u.sessions.Items[i].ProfileID + "/" + u.sessions.Items[i].ID }
	cols := []ui.TableColumn{{Title: "", Width: 32}, {Title: "Title"}, {Title: "Profile", Width: 140}, {Title: "Updated", Width: 145}, {Title: "KiB", Width: 80}, {Title: "Protection", Width: 130}}
	ui.Table(c, &u.sessionTable, cols, len(u.sessions.Items), func(row, col int) {
		s := u.sessions.Items[row]
		key := s.ProfileID + "/" + s.ID
		if col == 0 {
			_, checked := u.selected[key]
			if ui.Checkbox(c, &checked, "").Changed() {
				if checked {
					u.selected[key] = session.Ref{ProfileID: s.ProfileID, SessionID: s.ID}
				} else {
					delete(u.selected, key)
				}
			}
			return
		}
		v := ""
		switch col {
		case 1:
			v = s.Title
			if v == "" {
				v = s.ID
			}
		case 2:
			v = s.ProfileID
		case 3:
			v = s.ModifiedAt.Local().Format("01-02 15:04:05")
		case 4:
			v = fmt.Sprintf("%.1f", float64(s.Size)/1024)
		case 5:
			v = s.ProtectionReason
		}
		ui.Text(c, v).SingleLine()
	}).Grow(1)
	u.pagination(c, u.sessions.Total, u.query.Offset, u.query.Limit, func(n int) { u.query.Offset = n; u.reload() })
}
func (u *UI) pagination(c *ui.Context, total, offset, limit int, change func(int)) {
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, fmt.Sprintf("%d-%d / %d", min(offset+1, total), min(offset+limit, total), total)).Grow(1)
		if ui.Button(c, u.t("previous")).Disabled(offset == 0 || u.busy).Clicked() {
			change(max(0, offset-limit))
		}
		if ui.Button(c, u.t("next")).Disabled(offset+limit >= total || u.busy).Clicked() {
			change(offset + limit)
		}
	})
}
func (u *UI) providersPage(c *ui.Context) {
	p, has := u.provider()
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, u.t("newProvider")).Disabled(u.busy).Clicked() {
			u.openProvider(model.Provider{Protocol: "responses", AuthKind: "bearer", Credential: model.CredentialRef{Kind: "keychain"}, Capabilities: model.UnknownCapabilities()})
		}
		if ui.Button(c, u.t("edit")).Disabled(!has || u.busy).Clicked() {
			u.openProvider(p)
		}
		if ui.Button(c, "Test connection").Disabled(!has || u.busy).Clicked() {
			id := p.ID
			u.task("Test connection", func(ctx context.Context) (any, error) { return u.core.Providers.TestConnection(ctx, id) }, func(v any) { u.info("Upstream health", v) })
		}
		if ui.Button(c, u.t("models")).Disabled(!has || u.busy).Clicked() {
			u.task(u.t("models"), func(ctx context.Context) (any, error) { return u.core.Providers.Discover(ctx, p) }, func(v any) { u.openProvider(p); u.setModelDraft(v.([]model.ProviderModel)) })
		}
		if ui.Button(c, u.t("bind")).Disabled(!has || u.busy).Clicked() {
			u.bindProvider = p.ID
			u.bindModel = p.DefaultModel
			u.bindProfile = ""
			u.bindOpen = true
		}
		if ui.Button(c, u.t("remove")).Disabled(!has || u.busy).Clicked() {
			id, rev := p.ID, u.basic.State.Revision
			u.confirm(u.t("remove"), "Bound providers must be detached before removal.", func() {
				u.task(u.t("remove"), func(ctx context.Context) (any, error) { return nil, u.core.Providers.Delete(ctx, id, rev, true) }, u.changed)
			})
		}
		if ui.Button(c, u.t("gatewayStatus")).Disabled(u.busy).Clicked() {
			u.task(u.t("gatewayStatus"), func(ctx context.Context) (any, error) { _, h, e := u.core.Gateway.Probe(ctx); return h, e }, func(v any) { u.info(u.t("gatewayStatus"), v) })
		}
	})
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, "New API account").Disabled(!has || u.busy).Clicked() {
			u.apiAccountName = ""
			u.bindProvider = p.ID
			u.bindModel = p.DefaultModel
			u.apiAccountOpen = true
		}
	})
	if len(u.basic.State.PendingBindings) > 0 {
		ui.Text(c, "Imported bindings awaiting explicit reconnect").Bold()
		ui.Scroll(c).Height(110).Gap(4).Children(func() {
			for _, b := range u.basic.State.PendingBindings {
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, b.ProfileID+" -> "+b.ProviderID+" / "+b.Model).Grow(1)
					if ui.Button(c, "Reconnect "+b.ProfileID).Disabled(u.busy).Clicked() {
						u.bindProfile = b.ProfileID
						u.bindProvider = b.ProviderID
						u.bindModel = b.Model
						u.bindOpen = true
					}
				})
			}
		})
	}
	u.providerTable.Selected = &u.providerRow
	u.providerTable.Key = func(i int) any { return u.basic.Providers[i].ID }
	cols := []ui.TableColumn{{Title: "Provider"}, {Title: "Protocol", Width: 155}, {Title: "Default model", Width: 160}, {Title: "Endpoint"}, {Title: "Models", Width: 65}}
	ui.Table(c, &u.providerTable, cols, len(u.basic.Providers), func(row, col int) {
		p := u.basic.Providers[row]
		v := ""
		switch col {
		case 0:
			v = p.Name
		case 1:
			v = p.Protocol
		case 2:
			v = p.DefaultModel
		case 3:
			v = p.BaseURL
		case 4:
			v = fmt.Sprint(len(p.Models))
		}
		ui.Text(c, v).SingleLine()
	}).Grow(1)
	ui.Text(c, "Unknown capabilities are not treated as supported. Edit capabilities only after verifying the provider.").FontSize(12)
}
func (u *UI) usagePage(c *ui.Context) {
	ui.Row(c).Gap(8).Children(func() {
		if ui.Select(c, &u.usageSource, []string{"local", "account"}).Label("Usage source").Changed() {
			u.reload()
		}
		ui.Text(c, "Account-reported activity and local JSONL usage are separate data sources.").FontSize(12)
	})
	ui.Row(c).Gap(8).Children(func() {
		if ui.Select(c, &u.usageLoadLimit, []string{"5000", "10000", "20000", "all"}).Label("Detail load limit").Changed() {
			u.filter.Offset, u.threadOffset = 0, 0
			u.reload()
		}
	})
	ui.Row(c).Gap(8).Children(func() {
		if ui.Select(c, &u.usagePreset, []string{"all", "today", "this-week", "last-7-days", "this-month", "custom"}).Label("Time preset").Changed() {
			if u.usagePreset != "custom" {
				start, end, err := usage.PresetWindow(u.usagePreset, time.Now())
				if err != nil {
					u.fail(err)
				} else {
					u.after, u.before = start, end
					u.filter.Offset, u.threadOffset = 0, 0
					u.reload()
				}
			}
		}
		if ui.Button(c, "Reset usage filters").Disabled(u.busy).Clicked() {
			u.filter = usage.Filter{Limit: 50, ArchiveScope: "active"}
			u.usagePreset = "all"
			u.usageLoadLimit = "5000"
			u.after, u.before = "", ""
			u.threadOffset = 0
			u.reload()
		}
	})
	ui.Row(c).Gap(8).Children(func() {
		ui.Select(c, &u.usageProfile, u.profileIDs(true)).Label(u.t("allProfiles")).Width(180)
		ui.TextInput(c, &u.after).Label(u.t("after")).Width(170)
		ui.TextInput(c, &u.before).Label(u.t("before")).Width(170)
		if ui.Button(c, u.t("refresh")).Disabled(u.busy).Clicked() {
			u.filter.Offset = 0
			u.reload()
		}
		if ui.Button(c, u.t("index")).Disabled(u.busy).Clicked() {
			u.indexUsage()
		}
	})
	ui.Row(c).Gap(8).Children(func() {
		ui.TextInput(c, &u.filter.Workspace).Label(u.t("workspace")).Grow(1)
		ui.TextInput(c, &u.filter.Model).Label(u.t("modelFilter")).Width(200)
		ui.Select(c, &u.filter.ArchiveScope, []string{"", "active", "archived"}).Label("Archive scope").Width(130)
		if u.filter.ThreadKey != "" && ui.Button(c, "Clear thread filter").Clicked() {
			u.filter.ThreadKey = ""
			u.filter.Offset = 0
			u.reload()
		}
	})
	// Original Usage includes search, effort, price-confidence and sorting,
	// not just model/workspace/date. All filters apply at the repository level.
	ui.Row(c).Gap(8).Children(func() {
		ui.TextInput(c, &u.filter.Search).Label("Search calls / threads").Grow(1)
		ui.TextInput(c, &u.filter.Effort).Label("Reasoning effort").Width(150)
		ui.TextInput(c, &u.filter.PricingConfidence).Label("Pricing confidence").Width(160)
		ui.Select(c, &u.filter.SortKey, []string{"time", "duration", "gap", "attention", "thread", "initiator", "model", "effort", "total", "cached", "uncached", "output", "reasoning", "cost", "usage", "cache", "context"}).Label("Sort").Width(130)
		ui.Checkbox(c, &u.filter.SortAscending, "Ascending")
		if ui.Button(c, "Apply filters").Disabled(u.busy).Clicked() {
			u.filter.Offset, u.threadOffset = 0, 0
			u.reload()
		}
	})
	for section, code := range u.usage.Errors {
		ui.Text(c, section+": "+code+" (other sections remain available)")
	}
	if u.usageSource == "account" {
		u.accountUsageView(c)
		return
	}
	x := u.usage.Summary
	ui.Text(c, fmt.Sprintf("%d records | %d threads | %d input / %d cached / %d output tokens", x.Records, x.Threads, x.Tokens.Input, x.Tokens.CachedInput, x.Tokens.Output)).Bold()
	ui.Text(c, fmt.Sprintf("USD %.6f (%d unpriced records) - %s", x.EstimatedUSD, x.UnknownPrices, u.t("estimated"))).FontSize(12)
	u.usageChart(c)
	previousTab := u.usageTabIndex
	ui.Tabs(c, &u.usageTabIndex, u.t("calls"), u.t("threads"), "Insights", "Activity")
	if previousTab != u.usageTabIndex {
		u.reload()
	}
	switch u.usageTabIndex {
	case 0:
		u.callTable.Selected = &u.callRow
		u.callTable.Key = func(i int) any { return u.usage.Calls.Items[i].ID }
		cols := []ui.TableColumn{{Title: "Time", Width: 145}, {Title: "Profile", Width: 130}, {Title: "Model"}, {Title: "Input", Width: 85}, {Title: "Cached", Width: 85}, {Title: "Output", Width: 85}, {Title: "USD estimate", Width: 110}}
		ui.Table(c, &u.callTable, cols, len(u.usage.Calls.Items), func(row, col int) {
			e := u.usage.Calls.Items[row]
			v := ""
			switch col {
			case 0:
				v = e.Timestamp.Local().Format("01-02 15:04:05")
			case 1:
				v = e.ProfileID
			case 2:
				v = e.Model
			case 3:
				v = fmt.Sprint(e.Tokens.Input)
			case 4:
				v = fmt.Sprint(e.Tokens.CachedInput)
			case 5:
				v = fmt.Sprint(e.Tokens.Output)
			case 6:
				v = u.t("unknown")
				if e.Cost != nil {
					v = fmt.Sprintf("%.6f", *e.Cost)
				}
			}
			ui.Text(c, v).SingleLine()
		}).Grow(1)
		if ui.Button(c, u.t("details")).Disabled(u.callRow < 0 || u.callRow >= len(u.usage.Calls.Items)).Clicked() {
			u.info(u.t("details"), u.usage.Calls.Items[u.callRow])
		}
		if ui.Button(c, "Raw call context").Disabled(u.callRow < 0 || u.callRow >= len(u.usage.Calls.Items) || u.busy).Clicked() {
			event := u.usage.Calls.Items[u.callRow]
			homes := []string{}
			for _, p := range u.basic.Profiles {
				homes = append(homes, p.Home)
			}
			u.task("Reading selected call", func(ctx context.Context) (any, error) { return usage.ReadRawCall(ctx, event, homes) }, func(v any) { u.info("Raw call context", v) })
		}
		u.pagination(c, u.usage.Calls.Total, u.filter.Offset, u.filter.Limit, func(n int) { u.filter.Offset = n; u.reload() })
	case 1:
		u.threadTable.Selected = &u.threadRow
		u.threadTable.Key = func(i int) any { return u.usage.Threads[i].Key }
		cols := []ui.TableColumn{{Title: "Thread"}, {Title: "Profile", Width: 150}, {Title: "Workspace"}, {Title: "Tokens", Width: 110}}
		ui.Table(c, &u.threadTable, cols, len(u.usage.Threads), func(row, col int) {
			e := u.usage.Threads[row]
			v := ""
			switch col {
			case 0:
				v = e.Name
				if v == "" {
					v = e.SessionID
				}
			case 1:
				v = e.ProfileID
			case 2:
				v = e.Workspace
			case 3:
				v = fmt.Sprint(e.Tokens)
			}
			ui.Text(c, v).SingleLine()
		}).Grow(1)
		u.pagination(c, u.usage.ThreadTotal, u.threadOffset, 100, func(n int) { u.threadOffset = n; u.reload() })
		if ui.Button(c, "Thread details / calls").Disabled(u.threadRow < 0 || u.threadRow >= len(u.usage.Threads)).Clicked() {
			row := u.usage.Threads[u.threadRow]
			u.filter.ThreadKey = row.Key
			u.filter.Offset = 0
			u.usageTabIndex = 0
			u.reload()
		}
	case 3:
		u.activityView(c)
	case 2:
		ui.Scroll(c).Grow(1).Gap(8).Children(func() {
			cache := "Unknown"
			if x.Tokens.Input > 0 {
				cache = fmt.Sprintf("%.1f%%", float64(x.Tokens.CachedInput)/float64(x.Tokens.Input)*100)
			}
			if ui.Button(c, "Usage diagnostics").Clicked() {
				u.info("Usage diagnostics (parser totals are index-wide)", u.usage.Diagnostics)
			}
			ui.Text(c, "Cache hit ratio: "+cache)
			ins := u.usage.Insights
			fraction := func(n *float64) string {
				if n == nil {
					return "Unknown"
				}
				return fmt.Sprintf("%.1f%%", *n*100)
			}
			ui.Text(c, "Fast mode: "+fraction(ins.FastModePercent)+" | Most used reasoning: "+ins.MostUsedReasoning+" / "+fraction(ins.MostUsedReasoningPercent))
			ui.Text(c, fmt.Sprintf("Skills explored: %d | Total skills used: %d | Threads: %d", ins.SkillsExplored, ins.TotalSkillsUsed, ins.TotalThreads))
			ui.Text(c, fmt.Sprintf("Reasoning output: %d (already included in output)", x.Tokens.ReasoningOutput))
			ui.Text(c, fmt.Sprintf("Pricing known: %d; unknown: %d", x.KnownPrices, x.UnknownPrices))
			if ui.Button(c, u.t("accountUsage")).Disabled(u.usageProfile == "" || u.busy).Clicked() {
				id := u.usageProfile
				u.task(u.t("accountUsage"), func(ctx context.Context) (any, error) { return u.core.AccountUsage(ctx, id) }, func(v any) { u.info(u.t("accountUsage"), v) })
			}
			ui.Text(c, "Account-reported usage is separate from local JSONL. No subtraction across mismatched scopes.")
		})
	}
}
func (u *UI) usageChart(c *ui.Context) {
	if len(u.usage.Daily) == 0 {
		ui.Text(c, u.t("daily")+": "+u.t("empty"))
		return
	}
	labels := make([]string, len(u.usage.Daily))
	hi := 1.0
	for i, d := range u.usage.Daily {
		labels[i] = d.Date
		hi = math.Max(hi, float64(d.Tokens))
	}
	x := chart.NewBand(labels...).Padding(.2, .1)
	y := chart.NewLinear(0, hi).Nice(4)
	// Limit axis labels, not data. The official chart positions every daily bar.
	ticks := x.Ticks()
	thin := ticks[:0]
	step := max(1, len(ticks)/8)
	for i, t := range ticks {
		if i%step == 0 {
			thin = append(thin, t)
		}
	}
	chart.Plot(c, chart.Axes{Left: y.Ticks(4), Bottom: thin}, func(area ui.Element) {
		for i, d := range u.usage.Daily {
			chart.Rect(c, x.Band(i), y.Span(0, float64(d.Tokens))).Background(c.Theme().Accent).Radius(3).Tooltip(fmt.Sprintf("%s: %d tokens", d.Date, d.Tokens))
		}
	}).Height(170).Label(u.t("daily")).Gap(5)
}
