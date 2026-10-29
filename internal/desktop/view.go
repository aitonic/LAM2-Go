package desktop

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/session"
	"lam.local/lam2/internal/usage"
	"lam.local/lam2/internal/visualspec"
)

// All elements are built for this frame; no widget is retained in UI state.
// The shell follows the original LAM titlebar / main canvas / floating dock
// hierarchy rather than MyGo's unstyled default Sidebar demo.
func (u *UI) view(c *ui.Context) {
	p := applyLAMTheme(c)
	ui.Column(c).Fill().Gap(0).LineHeight(visualspec.RootLineHeight).LinearGradient(originalCanvasGradient(p.Dark)).Children(func() {
		originalCanvas(c, p.Dark)
		u.titlebar(c, p)
		// Original CSS: .content occupies the canvas while .bottomDock is an
		// overlay, not a row taking vertical space away from content.
		ui.Box(c).Grow(1).Children(func() {
			content := ui.Column(c).Fill().Gap(12)
			if u.router.Path() != "/settings" {
				content.Padding(visualspec.PageTopPadding, visualspec.PageSidePadding, 14)
			}
			content.Children(func() {
				if u.errorText != "" {
					errorBanner(c, p, u.errorText)
				}
				if u.reading {
					ui.Progress(c, -1).Label(u.t("refresh"))
				}
				if u.busy {
					ui.Row(c).Padding(8).Radius(10).Background(p.AccentSoft).Gap(10).Children(func() {
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
			})
			// Dock is anchored to the canvas bottom, 10 DIP above the statusbar.
			u.dock(c, p)
		})
		ui.Row(c).Height(visualspec.StatusbarHeight).Padding(0, visualspec.StatusbarSidePadding).Background(p.SurfaceAlt).Gap(10).BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
			status := u.status
			if status == "" {
				status = "Ready"
			}
			ui.Text(c, status).FontSize(11).TextColor(p.Muted).Grow(1)
			ui.Box(c).Size(7, 7).Radius(4).Background(p.Green)
			ui.Text(c, "Operational · LAM").FontSize(11).TextColor(p.Muted)
		})
	})
	u.dialogs(c)
}

// The center area grows between the left brand and the fixed right toolbar,
// exactly as .titlebarCenter did in the original CSS. Theme controls remain
// in the titlebar; changes are persisted using the normal Settings service.
func (u *UI) titlebar(c *ui.Context, p palette) {
	key := strings.TrimPrefix(u.router.Path(), "/")
	width, _ := c.Size()
	narrow := width <= visualspec.OriginalNarrowBreakpoint
	header := ui.Row(c).Gap(16).Background(p.Surface).BorderWidth(0, 0, 1, 0).BorderColor(p.Border)
	if narrow {
		// Original @media(max-width:1240px): titlebar wraps, loses its
		// fixed height and uses 12px 16px padding; actions take a full row.
		header.Wrap().Padding(visualspec.NarrowTitlebarPadY, visualspec.NarrowTitlebarPadX)
	} else {
		header.Height(visualspec.TitlebarHeight).Padding(0, visualspec.PageSidePadding)
	}
	header.Children(func() {
		ui.Row(c).Gap(12).Children(func() {
			// The source brand is a small LAM mark, not a decorative star.
			ui.Box(c).Size(32, 32).Center().Children(func() {
				originalLAMLogo(c, 32)
			})
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, "LAM").Bold().FontSize(11).TextColor(p.Muted)
				ui.Text(c, u.t(key)).Bold().FontSize(20).TextColor(p.Ink)
			})
		})
		ui.Row(c).Grow(1).Center().Children(func() {
			ui.Row(c).Padding(3).Gap(2).Radius(8).Background(p.SurfaceAlt).Border(1, p.Border).Children(func() {
				if u.settings.AccountMode == "pat" {
					pill(c, p, "PAT Mode", p.Accent, p.Surface)
				} else {
					pill(c, p, "Profile Mode", p.Accent, p.Surface)
				}
			})
		})
		actions := ui.Row(c).Gap(visualspec.TitlebarActionsGap)
		if narrow {
			actions.FillWidth().Justify(ui.SpaceBetween)
		}
		actions.Children(func() {
			if ui.Button(c, "↻ "+u.t("refresh")).Disabled(u.busy).Padding(6, 12).Radius(8).Clicked() {
				u.reload()
			}
			if ui.Button(c, "+ New Account").Disabled(u.busy).Padding(6, 12).Radius(8).Clicked() {
				u.newProfile()
			}
			if ui.Button(c, "+ External API").Disabled(u.busy).Padding(6, 12).Radius(8).Clicked() {
				u.openProvider(model.Provider{Protocol: "responses", AuthKind: "bearer", Credential: model.CredentialRef{Kind: "keychain"}, Capabilities: model.UnknownCapabilities()})
			}
			ui.Row(c).Padding(3).Gap(2).Radius(9).Background(p.SurfaceAlt).Border(1, p.Border).Children(func() {
				for _, option := range []struct{ icon, theme string }{{"device", "system"}, {"sun", "light"}, {"moon", "dark"}} {
					selected := u.settings.Theme == option.theme
					fg, bg := p.Muted, p.SurfaceAlt
					if selected {
						fg, bg = p.Ink, p.Surface
					}
					button := ui.Button(c, "").Tooltip("Theme: "+option.theme).TextColor(fg).Background(bg).Height(28).Radius(8).Padding(0, 10)
					button.Children(func() { originalIcon(c, option.icon, 14, fg) })
					if button.Clicked() && !selected && !u.busy {
						s := u.settings
						s.Theme = option.theme
						u.settings = s
						u.applyAppearance()
						u.saveSettings(s)
					}
				}
			})
		})
	})
}
func (u *UI) dock(c *ui.Context, p palette) {
	// The floating navigation is always present, regardless of the OS Dock icon preference.
	// Original .bottomDock is position:fixed; island bottom is 36px from
	// window bottom (26px statusbar + 10px gap). It must never shift page content.
	width, _ := c.Size()
	narrow := width <= visualspec.OriginalNarrowBreakpoint
	bottomGap := visualspec.FloatingDockBottomGap
	islandRadius := visualspec.DockRadius
	islandPadX := visualspec.FloatingDockPaddingX
	navGap := visualspec.FloatingDockGap
	iconSize := visualspec.FloatingDockIcon
	iconRadius := visualspec.FloatingDockIconRadius
	labelSize := visualspec.FloatingDockLabelSize
	maxWidth := float32(visualspec.DockMaxWidth)
	if width-48 < maxWidth {
		maxWidth = width - 48
	}
	if narrow {
		bottomGap = visualspec.NarrowDockBottomGap
		islandRadius = visualspec.NarrowDockRadius
		islandPadX = visualspec.NarrowDockPaddingX
		navGap = visualspec.NarrowDockGap
		iconSize = visualspec.NarrowDockIcon
		iconRadius = visualspec.NarrowDockIconRadius
		labelSize = visualspec.NarrowDockLabelSize
		maxWidth = width - 24
	}
	ui.Row(c).Absolute().Left(0).Right(0).Bottom(float32(bottomGap)).Center().Children(func() {
		// LAM's CSS uses backdrop-filter:blur(28px) and a translucent
		// overlay, not macOS Liquid Glass. Use MyGo's native plain Blur
		// material rather than the curved Liquid Glass surface.
		ui.Row(c).MaxWidth(maxWidth).Radius(float32(islandRadius)).Material(glass.Blur{Radius: visualspec.DockBlur}).Children(func() {
			alpha := visualspec.DockAlphaLight
			if p.Dark {
				alpha = visualspec.DockAlphaDark
			}
			dockBorder := ui.RGBA(255, 255, 255, .45)
			if p.Dark {
				dockBorder = ui.RGBA(255, 255, 255, .14)
			}
			island := ui.Row(c).Padding(visualspec.FloatingDockPaddingY, float32(islandPadX)).
				Gap(float32(navGap)).Radius(float32(islandRadius)).
				Background(p.Surface.Alpha(float32(alpha))).Border(1, dockBorder)
			if p.Dark {
				island.Shadow(0, 10, 28, 0, ui.RGBA(0, 0, 0, .35)).
					Shadow(0, 18, 44, 0, ui.RGBA(0, 0, 0, .30))
			} else {
				island.Shadow(0, 8, 24, 0, ui.RGBA(15, 23, 42, .08)).
					Shadow(0, 16, 36, 0, ui.RGBA(15, 23, 42, .06))
			}
			island.Children(func() {
				for _, nav := range []struct {
					key string
					bg  ui.Color
				}{
					{"overview", ui.Hex("#3b82f6")},
					{"usage", ui.Hex("#8b5cf6")},
					{"sessions", ui.Hex("#10b981")},
					{"providers", ui.Hex("#f97316")},
					{"settings", ui.Hex("#64748b")},
				} {
					selected := u.router.Path() == "/"+nav.key
					labelColor := p.Muted
					if selected {
						labelColor = p.Ink
					}
					navItem := ui.Column(c).Width(visualspec.FloatingDockItemWidth).Gap(4).Center()
					if narrow {
						navItem.Padding(visualspec.NarrowDockItemPadY, visualspec.NarrowDockItemPadX)
					}
					navItem.Children(func() {
						button := ui.Button(c, "").Background(nav.bg).TextColor(ui.Hex("#ffffff")).
							Size(float32(iconSize), float32(iconSize)).
							Padding(0).Radius(float32(iconRadius)).
							Tooltip(u.t(nav.key))
						button.Children(func() { originalIcon(c, nav.key, 16, ui.Hex("#ffffff")) })
						label := ui.Text(c, u.t(nav.key)).FontSize(float32(labelSize)).TextColor(labelColor).SingleLine()
						if (button.Clicked() || label.Clicked() || navItem.Clicked()) && !selected {
							u.router.Push("/" + nav.key)
							u.reload()
						}
					})
				}
			})
		})
	})
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
func (u *UI) quotaView(c *ui.Context, q model.QuotaSnapshot) {
	p := paletteFor(c.Theme().Dark)
	stale := q.Stale || (!q.ExpiresAt.IsZero() && time.Now().After(q.ExpiresAt))
	ui.Row(c).Gap(6).Children(func() {
		label := q.PlanType
		if label == "" {
			label = q.Source
		}
		if label == "" {
			label = "Quota"
		}
		pill(c, p, label, p.Accent, p.AccentSoft)
		if stale {
			pill(c, p, "STALE", p.Amber, p.AmberSoft)
		}
		if q.Status == "unavailable" || q.ErrorCode != "" {
			pill(c, p, "UNAVAILABLE", p.Danger, p.DangerSoft)
		}
	})
	for _, window := range q.Windows {
		reset := ""
		if window.ResetsAt != nil {
			reset = "Resets " + window.ResetsAt.Local().Format("Jan 2, 15:04")
		}
		progressLabel(c, p, window.ID, window.RemainingPercent, reset)
	}
	for _, group := range q.Groups {
		ui.Text(c, group.Name).Bold().FontSize(12).TextColor(p.Ink)
		for _, bucket := range group.Buckets {
			var remaining *float64
			if bucket.RemainingFraction != nil {
				v := 100 * (*bucket.RemainingFraction)
				remaining = &v
			}
			progressLabel(c, p, bucket.Name, remaining, bucket.ResetTime)
		}
	}
	if q.Credits != nil {
		count := "Unknown reset credits"
		if q.Credits.AvailableCount != nil {
			count = fmt.Sprintf("%d reset credits", *q.Credits.AvailableCount)
		}
		infoLine(c, p, count)
		if q.Credits.ExpiresAt != nil {
			infoLine(c, p, "Expires "+q.Credits.ExpiresAt.Local().Format("Jan 2, 15:04"))
		}
	}
	if q.ErrorCode != "" {
		ui.Text(c, q.ErrorCode).FontSize(11).TextColor(p.Danger)
	}
	if !q.FetchedAt.IsZero() {
		ui.Text(c, "Updated "+q.FetchedAt.Local().Format("Jan 2 15:04")).FontSize(11).TextColor(p.Muted)
	}
	if len(q.Windows) == 0 && len(q.Groups) == 0 && q.ErrorCode == "" {
		infoLine(c, p, "Quota windows not reported")
	}
}
func (u *UI) sessionsPage(c *ui.Context) {
	p := paletteFor(c.Theme().Dark)
	sectionHeading(c, p, "Sessions", "Search, resume, hand off and safely archive Codex conversations")
	card(c, p).Children(func() {
		ui.Row(c).Gap(8).Wrap().Children(func() {
			ui.TextInput(c, &u.query.Search).Label(u.t("search")).Grow(1)
			ui.Select(c, &u.queryProfile, u.profileIDs(true)).Label(u.t("allProfiles")).Width(200)
			ui.Select(c, &u.query.Sort, []string{"modified", "size", "title"}).Label("Sort").Width(110)
			if ui.Button(c, u.t("search")).Disabled(u.busy).Clicked() {
				u.query.Offset = 0
				u.reload()
			}
		})
		s, has := u.selectedSession()
		ui.Row(c).Gap(6).Wrap().Children(func() {
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
	})
	card(c, p).Grow(1).Children(func() {
		cardHeader(c, p, "Session library", fmt.Sprintf("%d matching sessions · %d selected", u.sessions.Total, len(u.selected)))
		u.sessionTable.Selected = &u.sessionRow
		u.sessionTable.Key = func(i int) any { return u.sessions.Items[i].ProfileID + "/" + u.sessions.Items[i].ID }
		// Legacy .sessionsTable has six columns with a 1280px minimum and
		// 22/7/13/16/12/30 percent widths. Native Table supplies the actual
		// virtualization and horizontal scrolling instead of custom widgets.
		panelWidth, _ := c.Size()
		widths := visualspec.SessionColumnWidths(int(panelWidth) - 2*visualspec.PageSidePadding)
		cols := []ui.TableColumn{
			{Title: "Session", Width: float32(widths[0]), MinWidth: float32(widths[0])},
			{Title: "Size", Width: float32(widths[1]), MinWidth: float32(widths[1])},
			{Title: "Last active", Width: float32(widths[2]), MinWidth: float32(widths[2])},
			{Title: "cwd", Width: float32(widths[3]), MinWidth: float32(widths[3])},
			{Title: "Provider", Width: float32(widths[4]), MinWidth: float32(widths[4])},
			{Title: "Actions", Width: float32(widths[5]), MinWidth: float32(widths[5])},
		}
		ui.Table(c, &u.sessionTable, cols, len(u.sessions.Items), func(row, col int) {
			s := u.sessions.Items[row]
			key := s.ProfileID + "/" + s.ID
			switch col {
			case 0:
				ui.Column(c).Gap(2).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						_, checked := u.selected[key]
						if ui.Checkbox(c, &checked, "").Disabled(s.Protected || u.busy).Changed() {
							if checked {
								u.selected[key] = session.Ref{ProfileID: s.ProfileID, SessionID: s.ID}
							} else {
								delete(u.selected, key)
							}
						}
						title := s.Title
						if title == "" {
							title = s.ID
						}
						ui.Text(c, title).Bold().FontSize(13).SingleLine().Grow(1)
						if ui.Button(c, "Copy").Disabled(u.busy).Clicked() {
							u.copyResume(s.ProfileID, s.ID, s.CWD)
						}
					})
					if s.Title != "" && s.Title != s.ID {
						ui.Text(c, s.ID).FontSize(11).TextColor(p.Muted).SingleLine()
					}
				})
			case 1:
				ui.Text(c, visualspec.FormatStorageBytes(s.Size)).FontSize(13).SingleLine()
			case 2:
				ui.Text(c, s.ModifiedAt.Local().Format("01-02 15:04")).FontSize(13).TextColor(p.Muted).SingleLine()
			case 3:
				cwd := s.CWD
				if cwd == "" {
					cwd = "unknown"
				}
				ui.Text(c, cwd).FontSize(12).SingleLine()
			case 4:
				providerID := s.Provider
				if providerID == "" {
					providerID = "unknown"
				}
				modelID := s.Model
				if modelID == "" {
					modelID = "unknown"
				}
				ui.Column(c).Gap(2).Children(func() {
					ui.Text(c, providerID).Bold().FontSize(12).SingleLine()
					ui.Text(c, modelID).FontSize(11).TextColor(p.Muted).SingleLine()
				})
			case 5:
				ui.Row(c).Gap(6).Children(func() {
					if ui.Button(c, "Copy").Disabled(u.busy).Clicked() {
						u.copyResume(s.ProfileID, s.ID, s.CWD)
					}
					if ui.Button(c, ">_ Terminal").Disabled(u.busy).Clicked() {
						u.launch(s.ProfileID, s.ID, s.CWD)
					}
					if ui.Button(c, "Relay To...").Disabled(u.busy).Clicked() {
						u.handoffSource = s.ProfileID
						u.handoffSession = s.ID
						u.handoffTarget = ""
						u.handoffOpen = true
					}
					if ui.Button(c, "Details").Disabled(u.busy).Clicked() {
						u.task(u.t("details"), func(ctx context.Context) (any, error) { return u.core.Sessions.Read(ctx, s.ProfileID, s.ID) }, func(v any) { u.info(u.t("details"), v) })
					}
				})
			}
		}).Grow(1).MaxHeight(560).Radius(12).Background(p.SurfaceAlt).Border(1, p.Border)
		u.pagination(c, u.sessions.Total, u.query.Offset, u.query.Limit, func(n int) { u.query.Offset = n; u.reload() })
	})
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
	p := paletteFor(c.Theme().Dark)
	sectionHeading(c, p, "Providers", "Model endpoints, credentials, capability checks and Codex bindings")
	card(c, p).Children(func() {
		p, has := u.provider()
		ui.Row(c).Gap(8).Wrap().Children(func() {
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
		ui.Row(c).Gap(8).Wrap().Children(func() {
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
					ui.Row(c).Gap(8).Wrap().Children(func() {
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
	})
	card(c, p).Grow(1).Children(func() {
		cardHeader(c, p, "Provider catalog", fmt.Sprintf("%d configured providers", len(u.basic.Providers)))
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
		}).Grow(1).Radius(10).Background(p.SurfaceAlt).Border(1, p.Border)
		ui.Text(c, "Unknown capabilities are not treated as supported. Edit capabilities only after verifying the provider.").FontSize(12)
	})
}
func (u *UI) usagePage(c *ui.Context) {
	p := paletteFor(c.Theme().Dark)
	ui.Scroll(c).Grow(1).Padding(visualspec.UsagePagePadding).Gap(visualspec.UsagePageGap).Children(func() {
		// .usageDashboardPage: 18px padding / 14px gap.
		// .usageDashboardTitle: 34x34 logo / 10px gap / 22px title.
		ui.Row(c).Gap(visualspec.UsagePageGap).AlignItems(ui.Start).Children(func() {
			ui.Row(c).Grow(1).Gap(10).Children(func() {
				ui.Box(c).Size(visualspec.UsageHeaderLogoSize, visualspec.UsageHeaderLogoSize).
					Radius(visualspec.UsageHeaderLogoRadius).Background(p.Surface).
					Border(1, p.Border).Center().Children(func() {
					originalIcon(c, "usage", visualspec.UsageHeaderIconSize, p.Ink)
				})
				ui.Column(c).Gap(3).Children(func() {
					ui.Text(c, "Usage").Bold().FontSize(visualspec.UsageHeaderTitleSize).TextColor(p.Ink)
					latest := "Local JSONL activity"
					if n := len(u.usage.Daily); n > 0 {
						latest = "Updated " + u.usage.Daily[n-1].Date
					}
					ui.Text(c, latest).FontSize(visualspec.UsageHeaderSubtitleSize).TextColor(p.Muted)
				})
			})
			// .usageDashboardControls: 12px gap. The label/select pair
			// is an 8px-gap .usageControlLabelInline, with a 130px select.
			ui.Row(c).Gap(visualspec.UsageHeaderControlsGap).Children(func() {
				ui.Row(c).Gap(8).Center().Children(func() {
					ui.Text(c, "LOAD LIMIT").Bold().FontSize(11).LetterSpacing(.55).TextColor(p.Muted)
					if ui.Select(c, &u.usageLoadLimit, []string{"5000", "10000", "20000", "all"}).Width(130).Changed() {
						u.filter.Offset, u.threadOffset = 0, 0
						u.reload()
					}
				})
				if ui.Button(c, "↻ "+u.t("refresh")).Disabled(u.busy).Clicked() {
					u.reload()
				}
			})
		})
		// Legacy horizontal Total/profile strip. Different sources remain
		// available under Filters instead of altering the public layout.
		ui.Row(c).Gap(3).Padding(3).Radius(9).Border(1, p.Border).Background(p.SurfaceAlt).Children(func() {
			ids := u.profileIDs(true)
			for i, id := range ids {
				label := id
				if i == 0 {
					label = "Total"
				}
				for _, profile := range u.basic.Profiles {
					if profile.ID == id {
						label = profile.Name
						break
					}
				}
				active := u.usageProfile == id
				fg, bg := p.Muted, p.SurfaceAlt
				if active {
					fg, bg = p.Accent, p.Surface
				}
				if ui.Button(c, label).Background(bg).TextColor(fg).Radius(7).Padding(7, 13).Clicked() && !active {
					u.usageProfile = id
					u.filter.Offset, u.threadOffset = 0, 0
					u.reload()
				}
			}
		})
		ui.Row(c).Gap(8).Wrap().Children(func() {
			pill(c, p, "Pricing source: local rate card", p.Muted, p.SurfaceAlt)
			pill(c, p, "Privacy mode: aggregate only", p.Muted, p.SurfaceAlt)
			pill(c, p, "Parser diagnostics: "+fmt.Sprint(len(u.usage.Diagnostics.ParserDiagnostics)), p.Muted, p.SurfaceAlt)
			pill(c, p, "Scope: "+u.filter.ArchiveScope, p.Muted, p.SurfaceAlt)
			ui.Spacer(c).Grow(1)
			label := "Filters"
			if u.usageFiltersOpen {
				label = "Hide filters"
			}
			if ui.Button(c, label).Clicked() {
				u.usageFiltersOpen = !u.usageFiltersOpen
			}
		})
		if u.usageFiltersOpen {
			card(c, p).Children(func() {
				cardHeader(c, p, "Data & filters", "Choose the scope and refresh individual results without changing source records")
				ui.Row(c).Gap(8).Wrap().Children(func() {
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
				ui.Row(c).Gap(8).Wrap().Children(func() {
					if ui.Select(c, &u.usageSource, []string{"local", "account"}).Label("Usage source").Width(155).Changed() {
						u.reload()
					}
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
				ui.Row(c).Gap(8).Wrap().Children(func() {
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
				ui.Row(c).Gap(8).Wrap().Children(func() {
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
			})
		}
		if u.usageSource == "account" {
			card(c, p).Children(func() { u.accountUsageView(c) })
			return
		}
		x := u.usage.Summary
		a := u.usage.Activity
		tokenTotal := x.Tokens.Input + x.Tokens.Output
		ui.Row(c).Gap(16).Children(func() {
			usageKPI(c, p, "TOKEN VOLUMES", prettyNumber(tokenTotal)+" tokens", 0, []usageStat{
				{"LIFETIME", prettyNumber(tokenTotal)},
				{"CACHED INPUT", prettyNumber(x.Tokens.CachedInput)},
				{"UNCACHED", prettyNumber(x.Tokens.Uncached())},
				{"REASONING", prettyNumber(x.Tokens.ReasoningOutput)},
			})
			usageKPI(c, p, "PRODUCTIVITY & STREAKS", prettyNumber(a.Peak.Tokens)+" tokens", 1, []usageStat{
				{"LONGEST STREAK", fmt.Sprintf("%d days", a.LongestStreak)},
				{"CURRENT STREAK", fmt.Sprintf("%d days", a.CurrentStreak)},
				{"ACTIVE DAYS", fmt.Sprint(a.ActiveDays)},
				{"TOTAL CALLS", prettyNumber(x.Records)},
			})
			cost := "Unknown"
			if x.KnownPrices > 0 {
				cost = fmt.Sprintf("$%.2f", x.EstimatedUSD)
			}
			usageKPI(c, p, "COST & CREDITS", cost, 2, []usageStat{
				{"KNOWN PRICE CALLS", fmt.Sprint(x.KnownPrices)},
				{"UNPRICED CALLS", fmt.Sprint(x.UnknownPrices)},
				{"ESTIMATE ONLY", "API-equivalent"},
				{"CURRENCY", "USD"},
			})
		})
		card(c, p).Children(func() { u.activityView(c) })
		card(c, p).Children(func() {
			cardHeader(c, p, "Insights & records", "Explore all calls and threads; pricing is an estimate, not an account charge")
			previousTab := u.usageTabIndex
			ui.Tabs(c, &u.usageTabIndex, "Insights", u.t("calls"), u.t("threads"), "Diagnostics")
			if previousTab != u.usageTabIndex {
				u.reload()
			}
			switch u.usageTabIndex {
			case 1:
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
				}).Height(355).Radius(10).Border(1, p.Border).Background(p.SurfaceAlt)
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
			case 2:
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
				}).Height(355).Radius(10).Border(1, p.Border).Background(p.SurfaceAlt)
				u.pagination(c, u.usage.ThreadTotal, u.threadOffset, 100, func(n int) { u.threadOffset = n; u.reload() })
				if ui.Button(c, "Thread details / calls").Disabled(u.threadRow < 0 || u.threadRow >= len(u.usage.Threads)).Clicked() {
					row := u.usage.Threads[u.threadRow]
					u.filter.ThreadKey = row.Key
					u.filter.Offset = 0
					u.usageTabIndex = 1
					u.reload()
				}
			case 3:
				if ui.Button(c, "Refresh diagnostics").Disabled(u.busy).Clicked() {
					u.reload()
				}
				u.infoAvailableDiagnostics(c, p)
				u.usageChart(c)
			case 0:
				ui.Scroll(c).Height(310).Gap(10).Children(func() {
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
		})
	})
}
func prettyNumber(v int64) string {
	switch {
	case v >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(v)/1_000_000_000)
	case v >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(v)/1_000_000)
	case v >= 1000:
		return fmt.Sprintf("%.1fK", float64(v)/1000)
	default:
		return fmt.Sprint(v)
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

// The original Diagnostics tab is independent from the main Insights tab.
func (u *UI) infoAvailableDiagnostics(c *ui.Context, p palette) {
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, "Parser diagnostics").Bold().FontSize(15).TextColor(p.Ink)
		for key, value := range u.usage.Errors {
			ui.Text(c, key+": "+value).FontSize(12).TextColor(p.Muted)
		}
		if ui.Button(c, "View parser diagnostic details").Clicked() {
			u.info("Usage diagnostics", u.usage.Diagnostics)
		}
	})
}
