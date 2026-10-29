package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo/ui"
	"lam.local/lam2/internal/account"
	"lam.local/lam2/internal/app"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/session"
	"lam.local/lam2/internal/visualspec"
	"time"
)

// Overview restores the original dashboard's summary cards and account tiles.
// Advanced account actions remain fully available beneath the selected tile.
func (u *UI) overview(c *ui.Context) {
	p := paletteFor(c.Theme().Dark)
	ui.Scroll(c).Grow(1).Gap(18).Children(func() {
		width, _ := c.Size()
		metricsColumns := visualspec.OverviewStatCount
		if width <= visualspec.OriginalNarrowBreakpoint {
			metricsColumns = 1
		}
		ui.Grid(c).Columns(metricsColumns).Gap(visualspec.OverviewStatCardGap).Children(func() {
			nManaged := len(u.basic.State.Profiles)
			metricCard(c, p, "Accounts", fmt.Sprintf("%d / %d", nManaged, len(u.basic.Profiles)), "Registered / discovered", p.Accent, p.AccentSoft)
			sessionsValue := "—"
			if u.sessions.Total > 0 {
				sessionsValue = fmt.Sprint(u.sessions.Total)
			}
			if metricCard(c, p, "Sessions", sessionsValue, "View Sessions for live total", p.Green, p.GreenSoft).Clicked() {
				u.router.Push("/sessions")
				u.reload()
			}
			if metricCard(c, p, "Providers", fmt.Sprint(len(u.basic.Providers)), "Configured upstreams", p.Purple, p.PurpleSoft).Clicked() {
				u.router.Push("/providers")
				u.reload()
			}
			active := 0
			for _, q := range u.basic.Quotas {
				if !q.Stale && q.Status == "ready" {
					active++
				}
			}
			metricCard(c, p, "Quota usable", fmt.Sprint(active), "Last verified snapshots", p.Amber, p.AmberSoft)
		})
		// Restore the original Codex / Antigravity segment at the top of the
		// overview, with a source bar below it. There are no fabricated quotas.
		ui.Row(c).Gap(12).Children(func() {
			ui.Tabs(c, &u.overviewAgentTab, "Codex", "Antigravity")
			ui.Spacer(c).Grow(1)
			if u.overviewAgentTab == 0 {
				if ui.Button(c, "Refresh quotas").Disabled(u.busy).Clicked() {
					u.refreshQuotas()
				}
			} else if ui.Button(c, "Refresh Antigravity").Disabled(u.busy).Clicked() {
				u.task("Antigravity", func(ctx context.Context) (any, error) {
					return u.core.Antigravity(ctx)
				}, func(v any) { u.quotaExtra = v.(model.QuotaSnapshot) })
			}
		})
		if u.overviewAgentTab == 1 {
			card(c, p).Children(func() {
				cardHeader(c, p, "Antigravity", "Quota data from the locally discovered source")
				if u.quotaExtra.Source != "" {
					u.quotaView(c, u.quotaExtra)
				} else {
					infoLine(c, p, "No Antigravity quota snapshot; refresh to query the local process")
				}
			})
			return
		}
		ui.Row(c).Gap(10).Children(func() {
			sectionHeading(c, p, "Accounts", "")
			ui.Spacer(c).Grow(1)
			pill(c, p, fmt.Sprintf("%d discovered", len(u.basic.Profiles)), p.Accent, p.AccentSoft)
		})
		ui.Row(c).Padding(10, 12).Radius(9).Background(p.SurfaceAlt).
			Border(1, p.Border).Children(func() {
			ui.Text(c, "Active source").FontSize(12).TextColor(p.Muted)
			ui.Spacer(c).Width(9)
			source := "Codex profile mode"
			if u.basic.State.ActivePAT != "" {
				source = "PAT · " + u.basic.State.ActivePAT
			}
			ui.Text(c, source).FontSize(12).Bold().TextColor(p.Ink)
		})
		if len(u.basic.Profiles) == 0 {
			softCard(c, p).Children(func() {
				ui.Text(c, "No Codex accounts found").Bold().TextColor(p.Ink)
				infoLine(c, p, "Create a profile or import an existing Codex home to see quota cards here.")
			})
		} else {
			width, _ := c.Size()
			// CSS auto-fill minmax(280px,1fr), not hand-picked breakpoints.
			columns := visualspec.AccountColumns(int(width) - 2*visualspec.PageSidePadding)
			ui.Grid(c).Columns(columns).Gap(visualspec.AccountGap).Children(func() {
				for i := range u.basic.Profiles {
					u.accountTile(c, p, i)
				}
			})
		}

	})
}

func (u *UI) accountTile(c *ui.Context, p palette, i int) {
	acct := u.basic.Profiles[i]
	q, qok := u.basic.Quotas[acct.ID]
	_, registered := u.basic.State.Profiles[acct.ID]
	// Preserve the legacy 152-degree three-stop gradient in both appearance
	// modes, rather than a two-stop 105-degree approximation.
	gradient := ui.LinearGradient{Angle: 152, Stops: []ui.GradientStop{
		{Color: ui.Hex("#ffffff"), At: 0},
		{Color: ui.Hex("#f4f9ff"), At: .42},
		{Color: ui.Hex("#f2f7ff"), At: 1},
	}}
	if p.Dark {
		gradient.Stops = []ui.GradientStop{
			{Color: ui.Hex("#171e2b"), At: 0},
			{Color: ui.Hex("#182436"), At: .48},
			{Color: ui.Hex("#191f2e"), At: 1},
		}
	}
	card := ui.Column(c).Padding(visualspec.AccountCardPadding).Gap(visualspec.CardGap).
		Radius(visualspec.AccountCardRadius).LinearGradient(gradient).Border(1, accountCardBorder(p, false))
	if card.Hovered() {
		card.BorderColor(accountCardBorder(p, true)).Transform(ui.Translation(0, -2))
		if p.Dark {
			card.LinearGradient(ui.LinearGradient{Angle: 152, Stops: []ui.GradientStop{
				{Color: ui.Hex("#1b2233"), At: 0},
				{Color: ui.Hex("#1b2e48"), At: .5},
				{Color: ui.Hex("#181e2c"), At: 1},
			}}).Shadow(0, 4, 10, 0, ui.RGBA(0, 0, 0, .4)).
				Shadow(0, 10, 28, 0, ui.RGBA(0, 0, 0, .45)).
				Shadow(0, 18, 44, 0, ui.RGBA(59, 130, 246, .2))
		} else {
			card.LinearGradient(ui.LinearGradient{Angle: 152, Stops: []ui.GradientStop{
				{Color: ui.Hex("#ffffff"), At: 0},
				{Color: ui.Hex("#f1f7ff"), At: .45},
				{Color: ui.Hex("#eaf1ff"), At: 1},
			}}).Shadow(0, 2, 6, 0, ui.RGBA(15, 23, 42, .06)).
				Shadow(0, 8, 22, 0, ui.RGBA(15, 23, 42, .1)).
				Shadow(0, 16, 40, 0, ui.RGBA(37, 99, 235, .12))
		}
	} else if p.Dark {
		card.Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, .35)).
			Shadow(0, 6, 18, 0, ui.RGBA(0, 0, 0, .4)).
			Shadow(0, 12, 32, 0, ui.RGBA(37, 99, 235, .15))
	} else {
		card.Shadow(0, 1, 2, 0, ui.RGBA(15, 23, 42, .04)).
			Shadow(0, 4, 14, 0, ui.RGBA(15, 23, 42, .07)).
			Shadow(0, 10, 28, 0, ui.RGBA(37, 99, 235, .08))
	}
	card.Children(func() {
		ui.Row(c).Gap(7).Children(func() {
			ui.Text(c, acct.Name).Bold().FontSize(15).TextColor(p.Ink).Grow(1).SingleLine()
			if acct.ID == "main" {
				pill(c, p, "MAIN", p.Accent, p.AccentSoft)
			}
			if !registered {
				pill(c, p, "DISCOVERED", p.Amber, p.AmberSoft)
			}
			anchor := ui.Button(c, "⋮").Tooltip("Account actions").Padding(2, 6)
			if anchor.Clicked() {
				if u.profileRow == i {
					u.accountMenuOpen = !u.accountMenuOpen
				} else {
					u.profileRow = i
					u.accountMenuOpen = true
				}
			}
			if u.profileRow == i {
				ui.PopoverBase(c, anchor, &u.accountMenuOpen, func(panel ui.Element) {
					panel.Margin(6, 0, 0, 0).Padding(10).Radius(12).
						Background(p.Surface).Border(1, p.Border).Shadow(0, 8, 24, 0, p.Shadow)
					ui.Scroll(c).Width(440).Height(280).Gap(10).Children(func() {
						cardHeader(c, p, "Account actions · "+acct.Name, "Select a command for this profile")
						u.accountOperations(c)
					})
				})
			}
		})
		ui.Row(c).Gap(6).Wrap().Children(func() {
			pill(c, p, acct.AuthMode, p.Purple, p.PurpleSoft)
			auth := acct.AuthStatus
			if auth == "" {
				auth = "Unknown auth"
			}
			col, bg := p.Green, p.GreenSoft
			if auth == "unknown" || !registered {
				col, bg = p.Amber, p.AmberSoft
			}
			pill(c, p, auth, col, bg)
		})
		infoLine(c, p, acct.Home)
		if binding, ok := u.basic.State.Bindings[acct.ID]; ok {
			infoLine(c, p, binding.ProviderID+" · "+binding.Model)
		} else {
			infoLine(c, p, "Codex default provider")
		}
		if acct.TokenExpiration != nil {
			expiry := "Expires " + acct.TokenExpiration.Local().Format("Jan 2 · 15:04")
			if acct.TokenExpiration.Before(time.Now()) {
				expiry = "Token expired"
			}
			infoLine(c, p, expiry)
		}
		if qok {
			u.quotaView(c, q)
		} else {
			infoLine(c, p, "Quota not available — refresh to retry")
		}
		// The original card has Relay Latest (or Switch Model for an API
		// account) plus Handoff. The management entry is the header ellipsis.
		ui.Row(c).Gap(8).Children(func() {
			_, isAPI := u.basic.State.Bindings[acct.ID]
			action := "▷ Relay Latest"
			if isAPI {
				action = "Switch Model"
			}
			if ui.PrimaryButton(c, action).Disabled(!registered || u.busy).Grow(1).Clicked() {
				u.profileRow = i
				if isAPI {
					b := u.basic.State.Bindings[acct.ID]
					u.bindProfile = acct.ID
					u.bindProvider = b.ProviderID
					u.bindModel = b.Model
					u.bindOpen = true
				} else {
					u.relayLatestTo(acct.ID)
				}
			}
			if ui.Button(c, "▷ Handoff").Disabled(!registered || u.busy).Grow(1).Clicked() {
				u.profileRow = i
				u.queryProfile = acct.ID
				u.router.Push("/sessions")
				u.reload()
			}
		})
		if u.settings.ExpandedCardActions {
			ui.Row(c).Gap(8).Wrap().Children(func() {
				if ui.Button(c, "View & Edit").Disabled(!registered || u.busy).Clicked() {
					u.profileRow = i
					u.profileDraft = acct
					u.profileEdit = true
					u.profileOpen = true
				}
				if ui.Button(c, "More account actions").Clicked() {
					u.profileRow = i
					u.accountMenuOpen = true
				}
			})
		}
	})
}

func (u *UI) accountOperations(c *ui.Context) {
	p, has := u.profile()
	_, registered := u.basic.State.Profiles[p.ID]
	ready := has && registered
	ui.Row(c).Gap(6).Wrap().Children(func() {
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
	ui.Row(c).Gap(6).Wrap().Children(func() {
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
	ui.Row(c).Gap(8).Wrap().Children(func() {
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
}

// relayLatestTo opens the existing preview/confirmation workflow; it never
// directly copies a JSONL into a different profile without user consent.
func (u *UI) relayLatestTo(targetID string) {
	u.task("Find last active session", func(ctx context.Context) (any, error) {
		return u.core.Sessions.Query(ctx, session.Query{Sort: "modified", Descending: true, Limit: 1})
	}, func(v any) {
		page := v.(model.Page[model.Session])
		if len(page.Items) == 0 {
			u.fail(fault.New("SESSION_NOT_FOUND", "No active Codex session found"))
			return
		}
		last := page.Items[0]
		if last.ProfileID == targetID {
			u.launch(targetID, last.ID, last.CWD)
			return
		}
		u.handoffSource = last.ProfileID
		u.handoffSession = last.ID
		u.handoffTarget = targetID
		u.handoffOpen = true
	})
}
