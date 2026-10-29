package desktop

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/provider"
	run "lam.local/lam2/internal/runtime"
	"lam.local/lam2/internal/settings"
)

func (u *UI) settingsPage(c *ui.Context) {
	ui.Scroll(c).Grow(1).Gap(12).Children(func() {
		ui.Text(c, u.t("general")).Bold()
		ui.Row(c).Gap(10).Children(func() {
			if ui.Select(c, &u.settings.Theme, []string{"system", "light", "dark"}).Label(u.t("theme")).Grow(1).Changed() {
				u.settingsDirty = true
			}
			if ui.Select(c, &u.settings.Language, []string{"system", "en", "zh-Hans"}).Label(u.t("language")).Grow(1).Changed() {
				u.settingsDirty = true
			}
			if ui.Select(c, &u.settings.Terminal, []string{"terminal", "ghostty", "cmux", "embedded"}).Label(u.t("terminal")).Grow(1).Changed() {
				u.settingsDirty = true
			}
		})
		if ui.TextInput(c, &u.settings.CodexPath).Label(u.t("codexPath")).Changed() {
			u.settingsDirty = true
		}
		if ui.Checkbox(c, &u.settings.CloseToTray, u.t("closeToTray")).Changed() {
			u.settingsDirty = true
		}
		if ui.Checkbox(c, &u.settings.ShowDock, u.t("dock")).Changed() {
			u.settingsDirty = true
		}
		if ui.Checkbox(c, &u.settings.OpenAtLogin, u.t("loginItem")).Changed() {
			u.settingsDirty = true
		}
		ui.Row(c).Gap(10).Children(func() {
			if ui.Select(c, &u.settings.ApprovalPolicy, []string{"on-request", "untrusted", "never", "on-failure"}).Label("Codex approval policy").Grow(1).Changed() {
				u.settingsDirty = true
			}
			if ui.Select(c, &u.settings.SandboxMode, []string{"read-only", "workspace-write", "danger-full-access"}).Label("Codex sandbox").Grow(1).Changed() {
				u.settingsDirty = true
			}
		})
		if ui.Button(c, "Restart ChatGPT").Disabled(u.busy).Clicked() {
			u.confirm("Restart ChatGPT", "Close and reopen the installed ChatGPT app. Unsaved work may be interrupted.", func() {
				u.task("Restart ChatGPT", func(ctx context.Context) (any, error) { return nil, run.RestartChatGPT(ctx) }, nil)
			})
		}
		if ui.Button(c, "Detect installed terminals").Clicked() {
			u.info("Terminal targets", run.TerminalTargets())
		}
		ui.Text(c, "Account mode and runtime").Bold()
		if ui.Select(c, &u.settings.AccountMode, []string{"profile", "pat"}).Label("Default account mode").Changed() {
			u.settingsDirty = true
		}
		ui.Row(c).Gap(8).Children(func() {
			if ui.TextInput(c, &u.antiPort).Label("Antigravity port (0 = disabled)").Grow(1).Changed() {
				u.settingsDirty = true
				u.runtimeFieldsDirty = true
			}
			if ui.TextInput(c, &u.trayOpacity).Label("Tray opacity (0.2 - 1)").Grow(1).Changed() {
				u.settingsDirty = true
				u.runtimeFieldsDirty = true
			}
			if ui.TextInput(c, &u.refreshInterval).Label("Quota refresh (seconds)").Grow(1).Changed() {
				u.settingsDirty = true
				u.runtimeFieldsDirty = true
			}
		})
		ui.Row(c).Gap(8).Children(func() {
			if ui.TextInput(c, &u.firstTimeout).Label("Gateway first response (seconds)").Grow(1).Changed() {
				u.settingsDirty = true
				u.runtimeFieldsDirty = true
			}
			if ui.TextInput(c, &u.requestTimeout).Label("Gateway request timeout (seconds)").Grow(1).Changed() {
				u.settingsDirty = true
				u.runtimeFieldsDirty = true
			}
		})
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, u.t("save")).Disabled(u.busy).Clicked() {
				if s, e := u.readSettingsFields(); e != nil {
					u.fail(e)
				} else {
					u.saveSettings(s)
				}
			}
			if ui.Button(c, u.t("advanced")).Clicked() {
				u.editJSON(u.t("advanced"), u.settings, func(raw string) {
					var s model.Settings
					if json.Unmarshal([]byte(raw), &s) != nil {
						u.fail(fault.New("INPUT_INVALID", "Invalid settings JSON"))
						return
					}
					u.saveSettings(s)
				})
			}
			if ui.Button(c, u.t("updates")).Clicked() {
				updater.CheckForUpdates()
			}
			if ui.Button(c, "Data directory").Clicked() {
				u.revealHome()
			}
		})
		ui.Text(c, "Signed update feeds are defined by the MyGo build configuration; development builds cannot update themselves.").FontSize(12)
		ui.Text(c, u.t("usage")).Bold()
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, u.t("prices")).Clicked() {
				rev := u.basic.State.Revision
				u.editJSON(u.t("prices"), u.basic.State.Prices, func(raw string) {
					var rates []model.RateCard
					if json.Unmarshal([]byte(raw), &rates) != nil {
						u.fail(fault.New("INPUT_INVALID", "Invalid rate-card JSON"))
						return
					}
					u.task(u.t("prices"), func(ctx context.Context) (any, error) { return nil, u.core.Settings.SavePrices(ctx, rates, rev) }, func(any) { u.jsonOpen = false; u.reload() })
				})
			}
			for _, item := range []struct{ key, action string }{{"reprice", "reprice"}, {"compact", "compact"}, {"resetIndex", "reset"}} {
				if ui.Button(c, u.t(item.key)).Disabled(u.busy).Clicked() {
					kind, label := item.action, u.t(item.key)
					u.confirm(label, "This modifies only the rebuildable LAM usage index, never the original session JSONL.", func() {
						u.task(label, func(ctx context.Context) (any, error) { return nil, u.core.MaintainUsage(ctx, kind, true) }, u.changed)
					})
				}
			}
		})
		ui.Text(c, "System").Bold()
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, u.t("migrate")).Disabled(u.busy).Clicked() {
				u.importLegacy()
			}
			if ui.Button(c, u.t("rollback")).Disabled(u.busy).Clicked() {
				u.rollbackImport()
			}
			if ui.Button(c, u.t("restore")).Disabled(u.busy).Clicked() {
				u.restoreArchive()
			}
			if ui.Button(c, u.t("doctor")).Disabled(u.busy).Clicked() {
				u.task(u.t("doctor"), func(ctx context.Context) (any, error) { return u.core.Doctor(ctx), nil }, func(v any) { u.info(u.t("doctor"), v) })
			}
		})
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, u.t("stopGateway")).Disabled(u.busy).Clicked() {
				u.confirm(u.t("stopGateway"), "Stop only the authenticated LAM gateway when no requests are active. External Codex clients will need it again for their next request.", func() {
					u.task(u.t("stopGateway"), func(ctx context.Context) (any, error) { return nil, u.core.Gateway.Stop(ctx) }, nil)
				})
			}
			if ui.Button(c, "Move Gateway port").Disabled(u.busy).Clicked() {
				u.editJSON("Gateway port migration", map[string]int{"port": u.settings.GatewayPort}, func(raw string) {
					var v struct {
						Port int `json:"port"`
					}
					if json.Unmarshal([]byte(raw), &v) != nil {
						u.fail(fault.New("INPUT_INVALID", "Invalid port JSON"))
						return
					}
					u.task("Preview Gateway port", func(ctx context.Context) (any, error) { return u.core.Providers.PlanPortChange(ctx, v.Port) }, func(value any) {
						p := value.(provider.PortPlan)
						u.jsonOpen = false
						u.confirm("Move Gateway port", "Updates every bound config. Active requests prevent this operation; the gateway restarts lazily on the next launch.", func() {
							u.task("Move Gateway port", func(ctx context.Context) (any, error) {
								return nil, u.core.Providers.CommitPortChange(ctx, p.ID, p.Fingerprint)
							}, u.changed)
						})
					})
				})
			}
			if ui.Button(c, "Approve credential command").Disabled(u.busy).Clicked() {
				u.approveCommand()
			}
		})
	})
}
func (u *UI) saveSettings(s model.Settings) {
	rev := u.basic.State.Revision
	u.task(u.t("save"), func(ctx context.Context) (any, error) {
		if e := settings.Validate(s); e != nil {
			return nil, e
		}
		previous := mygo.App.OpenAtLogin()
		if s.OpenAtLogin != previous {
			if e := mygo.App.SetOpenAtLogin(s.OpenAtLogin); e != nil {
				return nil, fault.New("LOGIN_ITEM_FAILED", "The operating system rejected the login-item change")
			}
		}
		if e := u.core.Settings.Save(ctx, s, rev); e != nil {
			if s.OpenAtLogin != previous {
				_ = mygo.App.SetOpenAtLogin(previous)
			}
			return nil, e
		}
		return s, nil
	}, func(v any) {
		u.settings = v.(model.Settings)
		u.settingsDirty = false
		u.syncSettingsFields()
		u.jsonOpen = false
		u.applyAppearance()
		u.reload()
	})
}
func (u *UI) approveCommand() {
	rev := u.basic.State.Revision
	u.editJSON("Approve credential command", struct {
		Executable string
		Args       []string
	}{"", []string{}}, func(raw string) {
		var v struct {
			Executable string
			Args       []string
		}
		if json.Unmarshal([]byte(raw), &v) != nil {
			u.fail(fault.New("INPUT_INVALID", "Invalid command JSON"))
			return
		}
		u.jsonOpen = false
		u.confirm("Approve credential command", "Explicitly trust this executable and arguments. LAM verifies its file hash before every execution.\n"+raw, func() {
			u.task("Approve command", func(ctx context.Context) (any, error) {
				return u.core.Providers.ApproveCommand(ctx, v.Executable, v.Args, rev, true)
			}, func(v any) { u.info("Approved command", v); u.reload() })
		})
	})
}

func (u *UI) syncSettingsFields() {
	u.antiPort = strconv.Itoa(u.settings.AntigravityPort)
	u.trayOpacity = strconv.FormatFloat(u.settings.TrayOpacity, 'f', 2, 64)
	u.firstTimeout = strconv.Itoa(u.settings.Gateway.FirstResponseSeconds)
	u.requestTimeout = strconv.Itoa(u.settings.Gateway.RequestSeconds)
	u.refreshInterval = strconv.Itoa(u.settings.RefreshSeconds)
	u.runtimeFieldsDirty = false
}
func (u *UI) readSettingsFields() (model.Settings, error) {
	s := u.settings
	if !u.runtimeFieldsDirty {
		return s, nil
	}
	fields := []struct {
		text   string
		target *int
	}{{u.antiPort, &s.AntigravityPort}, {u.firstTimeout, &s.Gateway.FirstResponseSeconds}, {u.requestTimeout, &s.Gateway.RequestSeconds}, {u.refreshInterval, &s.RefreshSeconds}}
	for _, field := range fields {
		n, e := strconv.Atoi(field.text)
		if e != nil {
			return s, fault.New("SETTING_INVALID", "Enter integer ports and timeouts")
		}
		*field.target = n
	}
	opacity, e := strconv.ParseFloat(u.trayOpacity, 64)
	if e != nil {
		return s, fault.New("SETTING_INVALID", "Enter an opacity between 0.2 and 1")
	}
	s.TrayOpacity = opacity
	return s, settings.Validate(s)
}
