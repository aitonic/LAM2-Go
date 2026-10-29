package desktop

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
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
		u.win = mygo.NewWindow(mygo.WindowOptions{Title: "LAM 2", Width: 1280, Height: 880, MinWidth: 1050, MinHeight: 720, StateKey: "main", Content: ui.View(u.view)})
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
func trayPNG() []byte {
	// Small monochrome L glyph, not a font asset or copied legacy binary.
	im := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 5; y < 27; y++ {
		for x := 8; x < 24; x++ {
			if x < 13 || y > 21 {
				im.SetNRGBA(x, y, color.NRGBA{A: 255})
			}
		}
	}
	var out bytes.Buffer
	_ = png.Encode(&out, im)
	return out.Bytes()
}
func (u *UI) createTray() {
	t, e := mygo.NewTray(mygo.TrayOptions{Icon: trayPNG(), IconIsTemplate: true, ToolTip: "LAM quota"})
	if e != nil {
		u.fail(fault.New("TRAY_UNAVAILABLE", "System tray could not be created; the main window remains usable"))
		return
	}
	u.tray = t
	u.trayWin = mygo.NewWindow(mygo.WindowOptions{Title: "LAM quota", Opacity: u.settings.TrayOpacity, Width: 420, Height: 540, Hidden: true, Frameless: true, Panel: true, SkipTaskbar: true, DisableResize: true, AlwaysOnTop: true, Content: ui.View(u.trayView)})
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
		u.trayWin.SetPosition(b.X+b.Width-420, b.Y+b.Height+4)
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
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Text(c, "LAM 2 / "+u.t("quota")).Bold().FontSize(18)
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, u.t("openMain")).Clicked() {
				u.trayWin.Hide()
				u.win.Show()
				u.win.Focus()
			}
			if ui.Button(c, u.t("close")).Clicked() {
				u.trayWin.Hide()
			}
		})
		ui.Tabs(c, &u.trayTab, "Codex", "Antigravity")
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
			if u.quotaExtra.Source != "" {
				u.quotaView(c, u.quotaExtra)
			} else {
				ui.Text(c, "Antigravity: not refreshed / unavailable")
			}
			return
		}
		if ui.Button(c, u.t("refreshQuota")).Disabled(u.busy).Clicked() {
			u.refreshQuotas()
		}
		ui.Scroll(c).Grow(1).Gap(10).Children(func() {
			for _, p := range u.basic.Profiles {
				_, registered := u.basic.State.Profiles[p.ID]
				ui.Text(c, p.Name).Bold()
				if q, ok := u.basic.Quotas[p.ID]; ok {
					u.quotaView(c, q)
				} else {
					ui.Text(c, u.t("unknown"))
				}
				if !registered {
					ui.Text(c, "Discovered account - register in Overview").FontSize(11)
					continue
				}
				ui.Row(c).Gap(7).Children(func() {
					if ui.Button(c, "Refresh "+p.Name).Disabled(u.busy).Clicked() {
						id := p.ID
						u.task("Refresh "+p.Name, func(ctx context.Context) (any, error) { return u.core.Quota.Refresh(ctx, id) }, func(v any) { u.basic.Quotas[id] = v.(model.QuotaSnapshot) })
					}
					if u.basic.State.Settings.AccountMode == "pat" {
						if ui.Button(c, "Switch PAT").Disabled(u.busy || u.basic.State.ActivePAT == p.ID).Clicked() {
							id := p.ID
							u.confirm("Switch PAT", "Replace the shared Codex authentication; other running clients may be affected.", func() {
								u.task("Switch PAT", func(ctx context.Context) (any, error) { return nil, u.core.Accounts.ActivatePAT(ctx, id, true) }, u.changed)
							})
							u.trayWin.Hide()
							u.win.Show()
							u.win.Focus()
						}
					} else if ui.Button(c, "Relay latest").Disabled(u.busy).Clicked() {
						id := p.ID
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
			if len(u.basic.Profiles) == 0 {
				ui.Text(c, u.t("empty"))
			}
		})
		if u.errorText != "" {
			ui.Text(c, u.errorText).FontSize(11)
		}
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
