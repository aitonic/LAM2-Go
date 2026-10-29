package desktop

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/plugins/updater"
	updaterNative "github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"
	"lam.local/lam2/internal/app"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/session"
	"lam.local/lam2/internal/storage"
	"lam.local/lam2/internal/visualspec"
)

// Run owns only the desktop lifetime. It deliberately does not stop the
// separate Go Gateway when the user quits or hides this process.
func Run(core *app.Core) error {
	u := New(core)
	st, e := core.Store.Read(u.ctx)
	if e != nil {
		return e
	}
	u.settings = st.Settings
	u.syncSettingsFields()
	desktopDir := filepath.Join(core.Store.Root, "desktop")
	if e := storage.EnsurePrivateDir(desktopDir); e != nil {
		return e
	}
	mygo.App.SetPath(mygo.PathUserData, desktopDir)
	if !mygo.App.RequestSingleInstanceLock() {
		u.cancel()
		return nil
	}
	mygo.Use(updaterNative.New(updater.Options{DisableAutomaticChecks: true}))
	mygo.App.OnSecondInstance(func([]string, string) {
		if u.win != nil {
			u.win.Show()
			u.win.Restore()
			u.win.Focus()
		}
	})
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if u.win != nil && !hasVisibleWindows {
			u.win.Show()
			u.win.Focus()
		}
	})
	mygo.App.OnLocaleChanged(func(locale string) {
		if locale != "" {
			u.locale = locale
		} else {
			u.locale = mygo.App.Locale()
		}
		u.setMenu()
		if u.win != nil {
			u.win.Invalidate()
		}
		if u.trayWin != nil {
			u.trayWin.Invalidate()
		}
	})
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if u.quitting {
			return
		}
		e.PreventDefault()
		if u.closed.Swap(true) {
			return
		}
		u.cancel()
		// Keep the event loop alive while canceled workers drain Window.Update.
		go func() { u.wg.Wait(); mygo.RunOnMain(func() { u.quitting = true; mygo.App.Quit() }) }()
	})
	mygo.App.WhenReady(func() {
		u.locale = mygo.App.Locale()
		u.win = mygo.NewWindow(mygo.WindowOptions{Title: "LAM", Width: visualspec.MainWindowWidth, Height: visualspec.MainWindowHeight, MinWidth: visualspec.MainWindowMinWidth, MinHeight: visualspec.MainWindowMinHeight, StateKey: "main", Content: ui.View(u.view)})
		u.win.OnClose(func(e *mygo.CloseEvent) {
			if u.quitting {
				return
			}
			e.PreventDefault()
			if u.settings.CloseToTray && u.tray != nil {
				u.win.Hide()
			} else {
				mygo.App.Quit()
			}
		})
		u.applyAppearance()
		u.setMenu()
		u.createTray()
		u.startTicker()
		u.task("Recovering interrupted operations", func(ctx context.Context) (any, error) { return core.Recovery(ctx) }, func(any) { u.reload() })
	})
	err := mygo.App.Run()
	u.closed.Store(true)
	u.cancel()
	u.wg.Wait()
	return err
}
func (u *UI) applyAppearance() {
	switch u.settings.Theme {
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
	lang := u.settings.Language
	if lang == "system" {
		lang = ""
	}
	mygo.App.SetLocale(lang)
	u.locale = mygo.App.Locale()
	if runtime.GOOS == "darwin" {
		if u.settings.ShowDock {
			mygo.App.Dock.Show()
		} else {
			mygo.App.Dock.Hide()
		}
	}
	if u.trayWin != nil {
		u.trayWin.SetOpacity(u.settings.TrayOpacity)
	}
	if u.win != nil {
		u.win.Invalidate()
	}
}
func (u *UI) setMenu() {
	nav := []*mygo.MenuItem{}
	for i, key := range []string{"overview", "usage", "sessions", "providers", "settings"} {
		path := "/" + key
		nav = append(nav, &mygo.MenuItem{Label: u.t(key), Accelerator: "CmdOrCtrl+" + string(rune('1'+i)), Click: func(*mygo.MenuItem, *mygo.Window) {
			u.router.Push(path)
			u.reload()
			if u.win != nil {
				u.win.Show()
			}
		}})
	}
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Label: "LAM", Submenu: []*mygo.MenuItem{{Role: mygo.RoleAbout}, updater.MenuItem(), mygo.Separator(), {Role: mygo.RoleQuit}}},
		{Role: mygo.RoleEditMenu},
		{Label: "Navigate", Submenu: nav},
		{Label: "Actions", Submenu: []*mygo.MenuItem{
			{Label: u.t("refresh"), Accelerator: "CmdOrCtrl+R", Click: func(*mygo.MenuItem, *mygo.Window) { u.reload() }},
			{Label: u.t("newProfile"), Accelerator: "CmdOrCtrl+N", Click: func(*mygo.MenuItem, *mygo.Window) {
				u.newProfile()
				if u.win != nil {
					u.win.Show()
					u.win.Invalidate()
				}
			}},
		}},
		{Role: mygo.RoleWindowMenu},
	}))
}
func (u *UI) createTray() {
	t, e := mygo.NewTray(mygo.TrayOptions{Icon: originalTrayTemplate, IconIsTemplate: true, ToolTip: "LAM quota"})
	if e != nil {
		u.fail(fault.New("TRAY_UNAVAILABLE", "System tray could not be created; the main window remains usable"))
		return
	}
	u.tray = t
	u.trayWin = mygo.NewWindow(mygo.WindowOptions{Title: "LAM Quota", Opacity: u.settings.TrayOpacity, Width: visualspec.TrayWidth, Height: visualspec.TrayHeight, Hidden: true, Frameless: true, Panel: true, SkipTaskbar: true, AlwaysOnTop: true, Content: ui.View(u.trayView)})
	u.trayWin.OnBlur(func() { u.trayWin.Hide() })
	u.trayWin.OnClose(func(e *mygo.CloseEvent) {
		if !u.quitting {
			e.PreventDefault()
			u.trayWin.Hide()
		}
	})
	show := func() {
		if u.trayWin.IsVisible() {
			u.trayWin.Hide()
			return
		}
		b := t.Bounds()
		u.trayWin.SetPosition(b.X+b.Width-visualspec.TrayWidth, b.Y+b.Height+6)
		u.trayWin.Show()
		u.trayWin.Focus()
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: u.t("quota"), Click: func(*mygo.MenuItem, *mygo.Window) { show() }},
		{Label: u.t("openMain"), Click: func(*mygo.MenuItem, *mygo.Window) { u.win.Show(); u.win.Focus() }},
		mygo.Separator(), {Role: mygo.RoleQuit},
	})
	if runtime.GOOS == "linux" {
		t.SetMenu(menu)
	} else {
		t.OnClick(show)
		t.OnRightClick(func() { t.SetMenu(menu); t.PopUpMenu(); t.SetMenu(nil) })
	}
}
func (u *UI) trayView(c *ui.Context) {
	p := applyLAMTheme(c)
	tray := originalTrayStyle(p.Dark)
	trayGradient := ui.LinearGradient{Angle: 180, From: ui.Hex("#fbfbfb"), To: ui.Hex("#f1f3f6")}
	if p.Dark {
		trayGradient = ui.LinearGradient{Angle: 180, From: ui.Hex("#15181f"), To: ui.Hex("#0d0f12")}
	}
	ui.Column(c).Fill().LinearGradient(trayGradient).Gap(0).Radius(14).Clip().
		Border(1, tray.PanelBorder).FontFeatures("tnum").Children(func() {
		ui.Row(c).Padding(10, 12, 9, 12).Gap(8).Background(tray.HeadBG).
			BorderWidth(0, 0, 1, 0).BorderColor(tray.HeadBorder).Children(func() {
			ui.Box(c).Size(30, 30).Radius(8).Background(p.AccentSoft).Center().Children(func() {
				originalLAMLogo(c, 30)
			})
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, "LAM").FontWeight(800).FontSize(15).TextColor(p.Ink)
				state := "Up to date"
				if u.reading || u.busy {
					state = "Refreshing..."
				}
				ui.Text(c, state).FontSize(11).TextColor(p.Muted)
			})
			if ui.Button(c, "☾").Tooltip("Toggle theme").Clicked() {
				s := u.settings
				if s.Theme == "dark" {
					s.Theme = "light"
				} else {
					s.Theme = "dark"
				}
				u.settings = s
				u.applyAppearance()
				u.saveSettings(s)
			}
			if ui.Button(c, "↻").Tooltip("Refresh all quotas").Disabled(u.busy).Clicked() {
				u.refreshQuotas()
			}
			if ui.Button(c, "×").Tooltip("Close quota panel").Clicked() {
				u.trayWin.Hide()
			}
		})
		// Original tray: three stat columns in a single strip with separators.
		ui.Row(c).Height(46).Gap(0).Background(tray.StatsBG).
			BorderWidth(0, 0, 1, 0).BorderColor(tray.StatsBorder).Children(func() {
			registered := len(u.basic.State.Profiles)
			usable := 0
			var quotaSum float64
			var quotaCount int
			for _, q := range u.basic.Quotas {
				if q.Stale || q.Status != "ready" {
					continue
				}
				usable++
				if len(q.Windows) > 0 && q.Windows[0].RemainingPercent != nil {
					quotaSum += *q.Windows[0].RemainingPercent
					quotaCount++
				}
			}
			avg := "--"
			if quotaCount > 0 {
				avg = fmt.Sprintf("%.0f%%", quotaSum/float64(quotaCount))
			}
			for i, stat := range []struct {
				label, value string
				fg           ui.Color
			}{
				{"Accounts", fmt.Sprintf("%d/%d", registered, len(u.basic.Profiles)), p.Accent},
				{"5h avg", avg, p.Purple},
				{"Usable", fmt.Sprint(usable), p.Green},
			} {
				if i > 0 {
					ui.Box(c).Size(1, 20).Background(tray.StatsDivider)
				}
				statColor := []ui.Color{tray.StatsValue, tray.StatsAverage, tray.StatsUsable}[i]
				ui.Column(c).Grow(1).Center().Gap(1).Padding(7, 6, 6, 6).Children(func() {
					ui.Text(c, stat.label).FontSize(10).FontWeight(750).TextColor(tray.StatsLabel)
					ui.Text(c, stat.value).FontWeight(900).FontSize(16).LineHeight(1).TextColor(statColor)
				})
			}
		})
		// Keep the original active-source strip even when the current Codex
		// mode has not published a concrete active identity.
		ui.Row(c).Padding(8, 12).Background(tray.ActiveBG).
			BorderWidth(0, 0, 1, 0).BorderColor(tray.ActiveBorder).Children(func() {
			// The old .trayActiveSourceBar is a second inner chip, with its
			// own 5x10 padding, 6px radius, 8px gap and green tinted border.
			ui.Row(c).Grow(1).Gap(8).Padding(5, 10).Radius(6).
				Background(ui.RGBA(16, 185, 129, .06)).Border(1, ui.RGBA(16, 185, 129, .12)).Children(func() {
				ui.Text(c, "● ACTIVE").FontWeight(850).FontSize(10).TextColor(ui.Hex("#10b981"))
				source := u.basic.State.ActivePAT
				if source == "" {
					source = "Codex profile mode"
				}
				ui.Text(c, source).FontWeight(800).FontSize(11).TextColor(tray.ActiveValue).SingleLine()
			})
		})
		ui.Row(c).Padding(10, 14).Children(func() {
			agCount := 0
			for _, group := range u.quotaExtra.Groups {
				agCount += len(group.Buckets)
			}
			ui.Tabs(c, &u.trayTab, fmt.Sprintf("Codex  %d", len(u.basic.Profiles)), fmt.Sprintf("Antigravity  %d", agCount))
		})
		if u.trayTab == 1 {
			if ui.Button(c, "Refresh Antigravity").Disabled(u.busy).Clicked() {
				u.task("Antigravity", func(ctx context.Context) (any, error) {
					q, e := u.core.Antigravity(ctx)
					if e != nil && q.Source == "" {
						return nil, e
					}
					return q, nil
				}, func(v any) { u.quotaExtra = v.(model.QuotaSnapshot) })
			}
			card(c, p).Grow(1).Children(func() {
				if u.quotaExtra.Source != "" {
					u.quotaView(c, u.quotaExtra)
				} else {
					infoLine(c, p, "Antigravity: not refreshed / unavailable")
				}
			})
		} else {
			ui.Row(c).Gap(10).Children(func() {
				ui.Text(c, "Codex accounts").Bold().FontSize(15).TextColor(p.Ink).Grow(1)
				if ui.Button(c, u.t("refreshQuota")).Disabled(u.busy).Clicked() {
					u.refreshQuotas()
				}
			})
			ui.Scroll(c).Grow(1).Padding(12, 12).Gap(10).Children(func() {
				for _, acct := range u.basic.Profiles {
					id := acct.ID
					_, registered := u.basic.State.Profiles[id]
					card(c, p).Children(func() {
						ui.Row(c).Gap(7).Children(func() {
							ui.Box(c).Size(8, 8).Radius(5).Background(p.Green)
							ui.Text(c, acct.Name).Bold().FontSize(14).TextColor(p.Ink).Grow(1)
							if !registered {
								pill(c, p, "DISCOVERED", p.Amber, p.AmberSoft)
							}
							if acct.AuthMode != "" {
								pill(c, p, acct.AuthMode, p.Accent, p.AccentSoft)
							}
						})
						quota, ok := u.basic.Quotas[id]
						if ok {
							ui.Row(c).Gap(10).Children(func() {
								ui.Column(c).Grow(1).Gap(5).Children(func() {
									infoLine(c, p, quota.Source+" · "+quota.PlanType)
									if len(quota.Windows) > 0 {
										infoLine(c, p, quota.Windows[0].ID)
										if quota.Windows[0].ResetsAt != nil {
											infoLine(c, p, quota.Windows[0].ResetsAt.Local().Format("Jan 2 · 15:04"))
										}
									}
									if quota.Stale {
										pill(c, p, "STALE", p.Amber, p.AmberSoft)
									}
									if len(quota.Windows) == 0 {
										infoLine(c, p, "No reported quota windows")
									}
								})
								if len(quota.Windows) > 0 {
									quotaRing(c, p, quota.Windows[0].RemainingPercent)
								}
							})
						} else {
							infoLine(c, p, "No quota data — refresh to retry")
						}
						if registered {
							ui.Row(c).Gap(7).Children(func() {
								if ui.Button(c, "Refresh "+acct.Name).Grow(1).Disabled(u.busy).Clicked() {
									u.task("Refresh "+acct.Name, func(ctx context.Context) (any, error) { return u.core.Quota.Refresh(ctx, id) }, func(v any) { u.basic.Quotas[id] = v.(model.QuotaSnapshot) })
								}
								if u.basic.State.Settings.AccountMode == "pat" {
									if ui.Button(c, "Switch PAT").Disabled(u.busy || u.basic.State.ActivePAT == id).Clicked() {
										u.confirm("Switch PAT", "Replace the shared Codex authentication; other running clients may be affected.", func() {
											u.task("Switch PAT", func(ctx context.Context) (any, error) { return nil, u.core.Accounts.ActivatePAT(ctx, id, true) }, u.changed)
										})
										u.trayWin.Hide()
										u.win.Show()
										u.win.Focus()
									}
								} else if ui.Button(c, "Relay latest").Disabled(u.busy).Clicked() {
									u.task("Find last active session", func(ctx context.Context) (any, error) {
										return u.core.Sessions.Query(ctx, session.Query{Sort: "modified", Descending: true, Limit: 1})
									}, func(v any) {
										page := v.(model.Page[model.Session])
										if len(page.Items) == 0 {
											u.fail(fault.New("SESSION_NOT_FOUND", "No active Codex session found"))
											return
										}
										last := page.Items[0]
										if last.ProfileID == id {
											u.launch(id, last.ID, last.CWD)
										} else {
											u.handoffSource = last.ProfileID
											u.handoffSession = last.ID
											u.handoffTarget = id
											u.handoffOpen = true
										}
										u.trayWin.Hide()
										u.win.Show()
										u.win.Focus()
									})
								}
							})
						}
					})
				}
				if len(u.basic.Profiles) == 0 {
					infoLine(c, p, u.t("empty"))
				}
			})
		}
		if u.errorText != "" {
			errorBanner(c, p, u.errorText)
		}
		ui.Row(c).Gap(8).Padding(6, 10, 8, 10).Background(tray.FooterBG).
			BorderWidth(1, 0, 0, 0).BorderColor(tray.FooterBorder).Children(func() {
			if ui.Button(c, "× Quit").Grow(1).Clicked() {
				mygo.App.Quit()
			}
			if ui.Button(c, "⌁ Stats").Grow(1).Clicked() {
				u.router.Push("/usage")
				u.reload()
				u.trayWin.Hide()
				u.win.Show()
				u.win.Focus()
			}
			if ui.Button(c, "↗ "+u.t("openMain")).Grow(1).Clicked() {
				u.trayWin.Hide()
				u.win.Show()
				u.win.Focus()
			}
		})
	})
}
func (u *UI) launch(id, resume, cwd string, extra ...string) {
	if u.settings.Terminal != "embedded" && !u.settings.EmbeddedTerminal {
		u.task(u.t("launch"), func(ctx context.Context) (any, error) {
			return nil, u.core.ExternalTerminal(ctx, id, resume, cwd, extra...)
		}, nil)
		return
	}
	exe, home := u.core.Executable, u.core.Store.Root
	u.task(u.t("launch"), func(ctx context.Context) (any, error) {
		_, e := u.core.LaunchPlan(ctx, id, resume, cwd, extra)
		return nil, e
	}, func(any) {
		args := []string{exe, "--home", home, "launch", "--profile", id}
		if resume != "" {
			args = append(args, "--resume", resume)
		}
		if cwd != "" {
			args = append(args, "--cwd", cwd)
		}
		if len(extra) > 0 {
			args = append(args, "--")
			args = append(args, extra...)
		}
		// Launch LAM itself: Terminal.Options.Env appends inherited variables, so
		// the child launcher must apply the same tested credential sanitation.
		var mu sync.Mutex
		var win *mygo.Window
		ended := false
		term, e := terminal.New(terminal.Options{Command: args, Dir: cwd, OnTitle: func(title string) {
			mu.Lock()
			w := win
			mu.Unlock()
			if w != nil {
				w.SetTitle(title)
			}
		}, OnExit: func(int) {
			mu.Lock()
			ended = true
			w := win
			mu.Unlock()
			if w != nil {
				w.Close()
			}
		}})
		if e != nil {
			u.fail(fault.New("TERMINAL_UNAVAILABLE", "MyGo terminal runtime could not start"))
			return
		}
		w := mygo.NewWindow(mygo.WindowOptions{Title: "Codex / " + id, Width: 1050, Height: 720, Content: ui.View(func(c *ui.Context) { terminal.View(c, term).Fill().AutoFocus() })})
		w.OnClosed(func() { term.Close() })
		mu.Lock()
		win = w
		alreadyEnded := ended
		mu.Unlock()
		if alreadyEnded {
			w.Close()
		}
	})
}
func (u *UI) syncWatchers() {
	if u.core == nil || u.closed.Load() {
		return
	}
	if u.watchers == nil {
		u.watchers = map[string]context.CancelFunc{}
	}
	desired := map[string]bool{}
	for _, p := range u.basic.Profiles {
		desired[p.Home] = true
	}
	for home, cancel := range u.watchers {
		if !desired[home] {
			cancel()
			delete(u.watchers, home)
		}
	}
	for home := range desired {
		if _, ok := u.watchers[home]; ok {
			continue
		}
		if len(u.watchers) >= 32 {
			break
		}
		ctx, cancel := context.WithCancel(u.ctx)
		u.watchers[home] = cancel
		u.wg.Add(1)
		go func(root string) {
			defer u.wg.Done()
			watcher, e := mygo.WatchFiles(ctx, root, mygo.FileWatchOptions{Recursive: true, Ignore: func(path string, isDir bool) bool {
				rel, e := filepath.Rel(root, path)
				if e != nil {
					return true
				}
				return rel != "." && rel != "sessions" && !strings.HasPrefix(rel, "sessions"+string(filepath.Separator))
			}})
			if e != nil {
				u.watchDirty.Store(true)
				return
			}
			defer watcher.Close()
			for {
				_, e = watcher.Next(ctx)
				if e != nil {
					if ctx.Err() == nil {
						u.watchDirty.Store(true)
					}
					return
				}
				u.core.Sessions.Invalidate()
				u.watchDirty.Store(true)
			}
		}(home)
	}
}
func (u *UI) startTicker() {
	u.wg.Add(1)
	win := u.win
	go func() {
		defer u.wg.Done()
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		lastScan := time.Now()
		for {
			select {
			case <-u.ctx.Done():
				return
			case now := <-tick.C:
				if now.Sub(lastScan) > 60*time.Second {
					u.core.Sessions.Invalidate()
					u.watchDirty.Store(true)
					lastScan = now
				}
				if u.closed.Load() {
					return
				}
				win.Update(func() {
					if u.closed.Load() || u.busy {
						return
					}
					if u.watchDirty.Swap(false) {
						u.reload()
						return
					}
					// Legacy tray refreshes both quota sources. A missing local
					// Antigravity server is represented as unavailable, not as an
					// error that blocks Codex refresh or invents zero quota.
					if !now.Before(u.nextAntigravity) {
						u.nextAntigravity = now.Add(2 * time.Minute)
						u.task("Refresh Antigravity", func(ctx context.Context) (any, error) {
							q, _ := u.core.Antigravity(ctx)
							return q, nil
						}, func(v any) { u.quotaExtra = v.(model.QuotaSnapshot) })
						return
					}
					if !now.Before(u.nextQuota) && len(u.basic.Profiles) > 0 {
						u.nextQuota = now.Add(time.Duration(u.settings.RefreshSeconds) * time.Second)
						u.refreshQuotas()
					}
				})
			}
		}
	}()
}
