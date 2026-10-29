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
	"lam.local/lam2/internal/visualspec"
)

func (u *UI) settingsPage(c *ui.Context) {
	p := paletteFor(c.Theme().Dark)
	ui.Row(c).Fill().Gap(0).Background(p.Surface).Children(func() {
		ui.Column(c).Width(visualspec.SettingsSidebarWidth).
			Padding(visualspec.SettingsSidebarPaddingY, visualspec.SettingsSidebarPaddingX).Gap(visualspec.SettingsSidebarGap).Background(p.SurfaceAlt).
			BorderWidth(0, 1, 0, 0).BorderColor(p.Border).Children(func() {
			ui.Text(c, "APP SETTINGS").FontSize(10).Bold().TextColor(p.Muted)
			for _, item := range []struct {
				idx         int
				title, icon string
			}{
				{0, "General", "⚙"}, {1, "Advanced", "◇"},
			} {
				u.settingsNavButton(c, p, item.idx, item.icon+"  "+item.title)
			}
			ui.Text(c, "INTEGRATIONS").FontSize(10).Bold().TextColor(p.Muted)
			u.settingsNavButton(c, p, 3, "▣  System & Desktop")
			ui.Text(c, "RESOURCES").FontSize(10).Bold().TextColor(p.Muted)
			u.settingsNavButton(c, p, 2, "▥  Rate Card")
			ui.Spacer(c).Grow(1)
			ui.Divider(c)
			ui.Row(c).Children(func() {
				infoLine(c, p, "Status")
				ui.Spacer(c).Grow(1)
				ui.Text(c, "● Connected").FontSize(11).Bold().TextColor(p.Green)
			})
			ui.Row(c).Children(func() {
				infoLine(c, p, "Version")
				ui.Spacer(c).Grow(1)
				ui.Text(c, "0.3.0").FontSize(11).Bold().TextColor(p.Ink)
			})
			ui.Row(c).Children(func() {
				infoLine(c, p, "Home Root")
				ui.Spacer(c).Grow(1)
				ui.Text(c, u.settingsHomeRoot()).FontSize(10).TextColor(p.Muted).SingleLine()
			})
		})
		ui.Scroll(c).Grow(1).Padding(visualspec.SettingsContentPadTop, visualspec.SettingsContentPadX, visualspec.SettingsContentPadBottom, visualspec.SettingsContentPadX).Gap(visualspec.SettingsRowGap).Children(func() {
			titles := []string{"General Settings", "Advanced Settings", "Rate Card & Usage", "System & Desktop"}
			subtitles := []string{
				"Configure core application preferences and Codex launch behavior.",
				"Account mode, proxy-independent runtime and gateway limits.",
				"Price tables and rebuildable Usage index maintenance.",
				"Data migration, recovery and local gateway administration.",
			}
			sectionHeading(c, p, titles[u.settingsSection], subtitles[u.settingsSection])
			if u.settingsDirty {
				ui.Row(c).Gap(8).Children(func() {
					pill(c, p, "UNSAVED CHANGES", p.Amber, p.AmberSoft)
					if ui.PrimaryButton(c, u.t("save")).Disabled(u.busy).Clicked() {
						if v, e := u.readSettingsFields(); e != nil {
							u.fail(e)
						} else {
							u.saveSettings(v)
						}
					}
				})
			}
			card(c, p).Children(func() {
				switch u.settingsSection {
				case 0:
					ui.Text(c, "APP PREFERENCES").Bold().FontSize(11).TextColor(p.Muted)
					u.settingsPreferenceRow(c, p, "Mode availability", "Controls which account mode information and titlebar controls are shown.", func() {
						choice := "Profile Only"
						switch u.settings.ModeAvailability {
						case "pat":
							choice = "PAT Only"
						case "both":
							choice = "Profile & PAT"
						}
						if ui.Select(c, &choice, []string{"Profile Only", "PAT Only", "Profile & PAT"}).Width(200).Changed() {
							switch choice {
							case "PAT Only":
								u.settings.ModeAvailability = "pat"
								u.settings.AccountMode = "pat"
							case "Profile & PAT":
								u.settings.ModeAvailability = "both"
							default:
								u.settings.ModeAvailability = "profile"
								u.settings.AccountMode = "profile"
							}
							u.settingsDirty = true
						}
					})
					ui.Divider(c)
					u.settingsPreferenceRow(c, p, "Codex launch permissions", "Applied to Codex sessions started, resumed, or relayed by LAM.", func() {
						choice := "Ask for approval"
						if u.settings.ApprovalPolicy == "never" {
							choice = "Approve for me"
							if u.settings.SandboxMode == "danger-full-access" {
								choice = "Full Access"
							}
						}
						if ui.Select(c, &choice, []string{"Ask for approval", "Approve for me", "Full Access"}).Width(200).Changed() {
							switch choice {
							case "Ask for approval":
								u.settings.ApprovalPolicy = "on-request"
								u.settings.SandboxMode = "workspace-write"
							case "Approve for me":
								u.settings.ApprovalPolicy = "never"
								u.settings.SandboxMode = "workspace-write"
							case "Full Access":
								u.settings.ApprovalPolicy = "never"
								u.settings.SandboxMode = "danger-full-access"
							}
							u.settingsDirty = true
						}
					})
					ui.Divider(c)
					u.settingsPreferenceRow(c, p, "Handoff terminal", "Terminal app used by profile relay, resume and login commands.", func() {
						if ui.Select(c, &u.settings.Terminal, []string{"terminal", "ghostty", "cmux", "embedded"}).Width(200).Changed() {
							u.settingsDirty = true
						}
					})
					ui.Divider(c)
					u.settingsPreferenceRow(c, p, "Compact card actions", "Use a compact 2-button layout on account cards. Disable to display more actions directly.", func() {
						choice := "Enabled (Compact)"
						if u.settings.ExpandedCardActions {
							choice = "Disabled (Show all actions)"
						}
						if ui.Select(c, &choice, []string{"Enabled (Compact)", "Disabled (Show all actions)"}).Width(200).Changed() {
							u.settings.ExpandedCardActions = choice == "Disabled (Show all actions)"
							u.settingsDirty = true
						}
					})
				case 1:

					if ui.Select(c, &u.settings.AccountMode, []string{"profile", "pat"}).Label("Default account mode").Changed() {
						if u.settings.AccountMode == "pat" && u.settings.ModeAvailability == "profile" {
							u.settings.ModeAvailability = "both"
						}
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
				case 2:

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
				case 3:
					ui.Row(c).Gap(10).Children(func() {
						if ui.Select(c, &u.settings.Theme, []string{"system", "light", "dark"}).Label(u.t("theme")).Grow(1).Changed() {
							u.settingsDirty = true
						}
						if ui.Select(c, &u.settings.Language, []string{"system", "en", "zh-Hans"}).Label(u.t("language")).Grow(1).Changed() {
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
					if ui.Button(c, "Restart ChatGPT").Disabled(u.busy).Clicked() {
						u.confirm("Restart ChatGPT", "Close and reopen the installed ChatGPT app. Unsaved work may be interrupted.", func() {
							u.task("Restart ChatGPT", func(ctx context.Context) (any, error) { return nil, run.RestartChatGPT(ctx) }, nil)
						})
					}
					if ui.Button(c, "Detect installed terminals").Clicked() {
						u.info("Terminal targets", run.TerminalTargets())
					}

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
				}
			})
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

// Desktop tests deliberately construct a UI without a running Core.
func (u *UI) settingsHomeRoot() string {
	if u.core == nil || u.core.Store == nil {
		return "~/.lam-next"
	}
	return u.core.Store.Root
}

// These native widgets reproduce the original left-rail and full-width
// label/description/control rows, without an HTML/CSS layer.
func (u *UI) settingsNavButton(c *ui.Context, p palette, idx int, title string) {
	active := u.settingsSection == idx
	fg, bg := p.Muted, p.SurfaceAlt
	if active {
		fg, bg = p.Surface, p.Accent
	}
	if ui.Button(c, title).Background(bg).TextColor(fg).FontSize(13).Radius(8).Padding(10, 14).Clicked() {
		u.settingsSection = idx
	}
}
func (u *UI) settingsPreferenceRow(c *ui.Context, p palette, title, description string, control func()) {
	ui.Row(c).Padding(visualspec.SettingsRowPadY, visualspec.SettingsRowPadX).
		Gap(visualspec.SettingsRowGap).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Column(c).Grow(1).Gap(4).Children(func() {
			ui.Text(c, title).FontSize(visualspec.SettingsRowTitleSize).Bold().TextColor(p.Ink)
			ui.Text(c, description).FontSize(visualspec.SettingsRowDescSize).LineHeight(1.4).TextColor(p.Muted)
		})
		control()
	})
}
