package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"lam.local/lam2/internal/account"
	"lam.local/lam2/internal/app"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/session"
	"lam.local/lam2/internal/visualspec"
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
			accountsWithData := 0
			usableQuotaCount := 0
			totalSessions := 0
			for _, prof := range u.basic.Profiles {
				totalSessions += prof.SessionCount
				if q, ok := u.basic.Quotas[prof.ID]; ok {
					hasData := len(q.Windows) > 0 || q.PlanType != "" || q.Status == "ready"
					if hasData {
						accountsWithData++
					}
					hasAvail := false
					allAvail := true
					for _, w := range q.Windows {
						hasAvail = true
						rem := 0.0
						if w.RemainingPercent != nil {
							rem = *w.RemainingPercent
						} else if w.UsedPercent != nil {
							rem = 100 - *w.UsedPercent
						}
						if rem <= 0 {
							allAvail = false
							break
						}
					}
					if hasAvail && allAvail {
						usableQuotaCount++
					}
				}
			}

			metricCard(c, p, "Accounts", fmt.Sprintf("%d/%d", accountsWithData, len(u.basic.Profiles)), p.Accent, p.AccentSoft)
			if metricCard(c, p, "Sessions", fmt.Sprint(totalSessions), p.Green, p.GreenSoft).Clicked() {
				u.router.Push("/sessions")
				u.reload()
			}
			if metricCard(c, p, "Providers", fmt.Sprint(len(u.basic.Providers)), p.Purple, p.PurpleSoft).Clicked() {
				u.router.Push("/providers")
				u.reload()
			}
			metricCard(c, p, "Quota usable", fmt.Sprint(usableQuotaCount), p.Amber, p.AmberSoft)
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
		ui.Row(c).Gap(8).Center().Children(func() {
			ui.Text(c, "Accounts").Bold().FontSize(18).TextColor(p.Ink)
			toggleBtn := ui.Button(c, "").Tooltip("Toggle compact/expanded card actions").Padding(3, 5)
			toggleBtn.Children(func() {
				originalIcon(c, "sliders", 14, p.Ink)
			})
			if toggleBtn.Clicked() {
				u.settings.ExpandedCardActions = !u.settings.ExpandedCardActions
			}
		})
		isPATMode := u.settings.AccountMode == "pat" || u.basic.State.Settings.AccountMode == "pat"
		ui.Row(c).Padding(9, 11).Radius(8).Background(p.SurfaceAlt).Border(1, p.Border).Gap(8).Children(func() {
			bannerLabel := "Active source"
			bannerName := "Codex profile mode"
			bannerPath := "No session found"
			if isPATMode {
				bannerLabel = "Active auth"
				bannerName = "PAT mode · No active account"
				bannerPath = "No unique tokens.account_id match"
				if u.basic.State.ActivePAT != "" {
					bannerName = "PAT · " + u.basic.State.ActivePAT
					for _, prof := range u.basic.Profiles {
						if prof.ID == u.basic.State.ActivePAT {
							bannerPath = shortenPath(prof.Home)
							break
						}
					}
				}
			} else if len(u.sessions.Items) > 0 {
				bannerName = "Codex profile mode"
				bannerPath = u.sessions.Items[0].ProfileID + " · " + u.sessions.Items[0].ID
			}
			ui.Text(c, bannerLabel).FontSize(12).TextColor(p.Muted)
			ui.Text(c, bannerName).FontSize(12).Bold().TextColor(p.Ink)
			ui.Text(c, bannerPath).FontSize(12).TextColor(p.Muted).Font("monospace").Grow(1).SingleLine()
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
	isPATMode := u.settings.AccountMode == "pat" || u.basic.State.Settings.AccountMode == "pat"
	activePAT := u.basic.State.ActivePAT
	isActiveAccount := false
	if isPATMode {
		isActiveAccount = (activePAT == acct.ID)
	} else if len(u.sessions.Items) > 0 {
		isActiveAccount = (u.sessions.Items[0].ProfileID == acct.ID)
	}

	_, isAPI := u.basic.State.Bindings[acct.ID]
	canResetQuota := qok && q.Credits != nil && q.Credits.AvailableCount != nil && *q.Credits.AvailableCount > 0

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
		// 1. Header row
		ui.Row(c).Gap(7).Center().Children(func() {
			ui.Row(c).Gap(6).Center().Grow(1).Children(func() {
				ui.Text(c, acct.Name).Bold().FontSize(15).TextColor(p.Ink).SingleLine()
				if qok && q.Credits != nil && q.Credits.AvailableCount != nil && *q.Credits.AvailableCount > 0 {
					dotColor := p.Accent
					if q.Credits.ExpiresAt != nil && q.Credits.ExpiresAt.Before(time.Now()) {
						dotColor = p.Amber
					}
					ui.Text(c, "●").FontSize(9).TextColor(dotColor)
					if q.Credits.ExpiresAt != nil {
						ui.Text(c, q.Credits.ExpiresAt.Local().Format("Jan 2 15:04")).FontSize(11).TextColor(p.Muted)
					}
				}
			})

			refreshBtn := ui.Button(c, "↻").Tooltip("Refresh " + acct.Name + " quota").Padding(2, 6)
			if refreshBtn.Clicked() {
				u.task("Refresh "+acct.Name, func(ctx context.Context) (any, error) {
					return u.core.Quota.Refresh(ctx, acct.ID)
				}, func(v any) {
					u.basic.Quotas[acct.ID] = v.(model.QuotaSnapshot)
				})
			}

			if !u.settings.ExpandedCardActions {
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
			}
		})

		// 2. Tags row
		ui.Row(c).Gap(6).Wrap().Children(func() {
			if !isAPI && qok && q.PlanType != "" {
				planTypeBadge(c, p, q.PlanType)
			}
			if isActiveAccount {
				pill(c, p, "ACTIVE", ui.Hex("#059669"), ui.Hex("#ecfdf5"))
			} else if acct.AuthStatus == "ready" || registered {
				pill(c, p, "Logged in", ui.Hex("#059669"), ui.Hex("#ecfdf5"))
			} else if !isAPI {
				pill(c, p, "Login needed", ui.Hex("#d97706"), ui.Hex("#fffbeb"))
			}
			if isAPI {
				pill(c, p, "External API", p.Accent, p.AccentSoft)
			} else {
				modeLabel := acct.AuthMode
				if modeLabel == "" || modeLabel == "profile" {
					modeLabel = "Auth"
				} else if modeLabel == "pat" {
					modeLabel = "PAT"
				}
				pill(c, p, modeLabel, p.Muted, p.SurfaceAlt)
			}
		})

		// 3. Path row
		ui.Text(c, shortenPath(acct.Home)).FontSize(12).TextColor(p.Muted).Font("monospace").SingleLine()

		// 4. Meta text row
		metaParts := []string{fmt.Sprintf("%d sessions", acct.SessionCount)}
		if binding, ok := u.basic.State.Bindings[acct.ID]; ok && binding.ProviderID != "" && binding.ProviderID != "unknown" {
			metaParts = append(metaParts, "Provider: "+binding.ProviderID)
		}
		modelName := "unknown"
		if binding, ok := u.basic.State.Bindings[acct.ID]; ok && binding.Model != "" {
			modelName = binding.Model
		}
		metaParts = append(metaParts, modelName)
		ui.Text(c, strings.Join(metaParts, " · ")).FontSize(12).TextColor(p.Muted).SingleLine()

		// 5. Quota horizontal bars
		if !isAPI {
			if qok && len(q.Windows) > 0 {
				ui.Column(c).Gap(8).Children(func() {
					for _, w := range q.Windows {
						label, isSession := quotaWindowDisplayLabel(w.ID, w.WindowMinutes)
						resetText := ""
						if w.ResetsAt != nil {
							if isSession {
								resetText = w.ResetsAt.Local().Format("3:04 PM")
							} else {
								resetText = w.ResetsAt.Local().Format("Jan 2, 2006 at 3:04 PM")
							}
						}
						var rem *float64
						if w.RemainingPercent != nil {
							rem = w.RemainingPercent
						} else if w.UsedPercent != nil {
							val := 100 - *w.UsedPercent
							rem = &val
						}
						quotaWindowBar(c, p, label, rem, resetText)
					}
				})
			} else if qok {
				infoLine(c, p, "Quota snapshot ready (no windows)")
			} else {
				infoLine(c, p, "Quota not available — refresh to retry")
			}
		}

		// 6. Action buttons
		if isAPI {
			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "Switch Model").Disabled(u.busy).Grow(1).Clicked() {
					u.profileRow = i
					b := u.basic.State.Bindings[acct.ID]
					u.bindProfile = acct.ID
					u.bindProvider = b.ProviderID
					u.bindModel = b.Model
					u.bindOpen = true
				}
				if ui.Button(c, "▷ Handoff").Disabled(u.busy || len(u.basic.Profiles) < 2).Grow(1).Clicked() {
					u.profileRow = i
					u.queryProfile = acct.ID
					u.router.Push("/sessions")
					u.reload()
				}
			})
			if u.settings.ExpandedCardActions {
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "View & Edit").Disabled(u.busy).Grow(1).Clicked() {
						u.profileRow = i
						u.profileDraft = acct
						u.profileEdit = true
						u.profileOpen = true
					}
					if ui.Button(c, "Rename").Disabled(acct.ID == "main" || u.busy).Grow(1).Clicked() {
						u.profileDraft = acct
						u.profileEdit = true
						u.profileHome = ""
						u.profileOpen = true
					}
				})
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "Delete").TextColor(p.Danger).Disabled(acct.ID == "main" || u.busy).Grow(1).Clicked() {
						id := acct.ID
						u.confirm("Delete account", "Remove API account "+acct.Name+"?", func() {
							u.task("Delete API account", func(ctx context.Context) (any, error) {
								return nil, u.core.APIAccounts.Delete(ctx, id, true)
							}, u.changed)
						})
					}
				})
			}
		} else if isPATMode {
			// PAT mode buttons
			// Row 1: Switch PAT (Primary) + Login/Update (Default)
			ui.Row(c).Gap(8).Children(func() {
				switchDisabled := acct.ID == "main" || isActiveAccount || u.busy
				if ui.PrimaryButton(c, "Switch PAT").Disabled(switchDisabled).Grow(1).Clicked() {
					id := acct.ID
					u.confirm("Switch PAT", "Replace the shared Codex authentication; other running clients may be affected.", func() {
						u.task("Switch PAT", func(ctx context.Context) (any, error) {
							return nil, u.core.Accounts.ActivatePAT(ctx, id, true)
						}, u.changed)
					})
				}
				loginLabel := "⚿ Login"
				if acct.AuthStatus == "ready" {
					loginLabel = "⚿ Update"
				}
				if ui.Button(c, loginLabel).Disabled(u.busy).Grow(1).Clicked() {
					u.clearCredentialDraft()
					u.authProfile = acct.ID
					u.authPurpose = "quota-session"
					u.credentialOpen = true
				}
			})
			if u.settings.ExpandedCardActions {
				// Row 2: Export CPA + Reset Quota
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "☁ Export CPA").Disabled(u.busy).Grow(1).Clicked() {
						u.exportAuth(acct.ID)
					}
					if ui.Button(c, "▷ Reset Quota").Disabled(!canResetQuota || u.busy).Grow(1).Clicked() {
						u.accountResetQuota(acct.ID, acct.Name)
					}
				})
				// Row 3: Rename (full width)
				ui.Row(c).Gap(8).Children(func() {
					renameDisabled := acct.ID == "main" || isActiveAccount || u.busy
					if ui.Button(c, "✎ Rename").Disabled(renameDisabled).Grow(1).Clicked() {
						u.profileDraft = acct
						u.profileEdit = true
						u.profileHome = ""
						u.profileOpen = true
					}
				})
			}
		} else {
			// Profile mode buttons
			// Row 1: Relay Latest (Primary) + Handoff (Default)
			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "▷ Relay Latest").Disabled(u.busy).Grow(1).Clicked() {
					u.relayLatestTo(acct.ID)
				}
				if ui.Button(c, "▷ Handoff").Disabled(u.busy || len(u.basic.Profiles) < 2).Grow(1).Clicked() {
					u.profileRow = i
					u.queryProfile = acct.ID
					u.router.Push("/sessions")
					u.reload()
				}
			})
			if u.settings.ExpandedCardActions {
				// Row 2: Reset Quota + Login
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "▷ Reset Quota").Disabled(!canResetQuota || u.busy).Grow(1).Clicked() {
						u.accountResetQuota(acct.ID, acct.Name)
					}
					if ui.Button(c, "⚿ Login").Disabled(u.busy).Grow(1).Clicked() {
						u.loginKey = ""
						u.loginOpen = true
						u.bindProfile = acct.ID
					}
				})
				// Row 3: Rename + Delete
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "✎ Rename").Disabled(acct.ID == "main" || u.busy).Grow(1).Clicked() {
						u.profileDraft = acct
						u.profileEdit = true
						u.profileHome = ""
						u.profileOpen = true
					}
					if ui.Button(c, "🗑 Delete").TextColor(p.Danger).Disabled(acct.ID == "main" || u.busy).Grow(1).Clicked() {
						id, rev := acct.ID, u.basic.State.Revision
						u.confirm("Delete account", "Remove profile "+acct.Name+"?", func() {
							u.task("Delete account", func(ctx context.Context) (any, error) {
								return nil, u.core.Accounts.Delete(ctx, id, rev, true)
							}, u.changed)
						})
					}
				})
			}
		}
	})
}

func quotaWindowDisplayLabel(id string, durationMins int64) (string, bool) {
	if durationMins == 300 || strings.Contains(id, "primary") || strings.Contains(id, "session") || strings.Contains(id, "5h") {
		return "Session (5h)", true
	}
	if durationMins == 10080 || strings.Contains(id, "secondary") || strings.Contains(id, "weekly") || strings.Contains(id, "7d") {
		return "Weekly (7d)", false
	}
	return id, false
}

func (u *UI) accountResetQuota(profileID, profileName string) {
	u.confirm("Reset Quota", "Reset quota for "+profileName+" using 1 rate limit reset credit?", func() {
		u.task("Reset Quota", func(ctx context.Context) (any, error) {
			key, err := u.core.Quota.PrepareReset(ctx, profileID, "")
			if err != nil {
				return nil, err
			}
			return u.core.Quota.ConsumeReset(ctx, key, true)
		}, func(_ any) {
			u.refreshQuotas()
		})
	})
}

func (u *UI) accountOperations(c *ui.Context) {
	p, has := u.profile()
	_, registered := u.basic.State.Profiles[p.ID]
	ready := has
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
		if ui.Button(c, "Update Session Quota").Disabled(!ready || u.busy).Clicked() {
			u.clearCredentialDraft()
			u.authProfile = p.ID
			u.authPurpose = "quota-session"
			u.credentialOpen = true
		}
		if ui.Button(c, "Export CPA").Disabled(!ready || u.busy).Clicked() {
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
