package desktop

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"lam.local/lam2/internal/account"
	"lam.local/lam2/internal/app"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/handoff"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/provider"
)

func (u *UI) newProfile() {
	u.configTemplate = ""
	mode := u.settings.AccountMode
	if mode == "" {
		mode = "profile"
	}
	u.profileDraft = model.Profile{AuthMode: mode, Proxy: model.Proxy{Mode: "inherit"}}
	u.profileHome = ""
	u.profileImportJSON = ""
	u.patDraft = ""
	u.expiryDraft = ""
	if mode == "pat" {
		u.profileCreateModeTab = 1
		u.profileCreateMethodTab = 0
	} else {
		u.profileCreateModeTab = 0
		u.profileCreateMethodTab = 1
	}
	u.profileOverwriteWrapper = false
	u.profileEdit = false
	u.profileOpen = true
}
func (u *UI) setModelDraft(models []model.ProviderModel) {
	u.providerDraft.Models = models
	ids := make([]string, 0, len(models))
	for _, m := range models {
		ids = append(ids, m.ID)
	}
	u.modelDraft = strings.Join(ids, "\n")
}
func (u *UI) openProvider(p model.Provider) {
	u.providerDraft = p
	u.setModelDraft(p.Models)
	u.secretDraft = ""
	u.credentialKind = p.Credential.Kind
	if u.credentialKind == "auth_command" {
		u.credentialKind = "command"
	}
	u.credentialEnv = p.Credential.EnvKey
	u.providerOpen = true
	if u.credentialKind == "" {
		u.credentialKind = "keychain"
	}
}
func (u *UI) clearCredentialDraft() {
	u.credentialOpen = false
	u.authJSONDraft = ""
	u.patDraft = ""
	u.expiryDraft = ""
	u.authProfile = ""
}
func (u *UI) dialogs(c *ui.Context) {
	p := paletteFor(c.Theme().Dark)
	ui.Modal(c, &u.credentialOpen, func() {
		ui.Column(c).Width(640).Gap(8).Padding(20).Radius(16).Background(p.Surface).Border(1, p.Border).Shadow(0, 4, 14, 0, rootShadowMd(p)).Children(func() {
			ui.Text(c, "Import Session / PAT").Bold().FontSize(19).TextColor(p.Ink)
			ui.Row(c).Gap(8).Padding(10).Radius(10).Background(p.AmberSoft).Children(func() {
				ui.Text(c, "!").Bold().TextColor(p.Amber)
				ui.Text(c, "Sensitive credentials · only paste trusted session JSON. Secrets are never copied during Handoff.").FontSize(12).TextColor(p.Amber)
			})
			ui.Select(c, &u.authPurpose, []string{"runtime", "quota-session"}).Label("Credential purpose")
			ui.Text(c, "Session / auth JSON")
			ui.TextArea(c, &u.authJSONDraft).Height(180)
			if u.authPurpose == "runtime" {
				ui.TextInput(c, &u.patDraft).Password().Label("Optional personal access token")
				ui.TextInput(c, &u.expiryDraft).Label("Optional expiry (RFC3339)")
			}
			u.formError(c)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, u.t("cancel")).Clicked() {
					u.clearCredentialDraft()
				}
				if ui.PrimaryButton(c, u.t("import")).Disabled(u.busy).Clicked() {
					id, purpose := u.authProfile, u.authPurpose
					raw := []byte(u.authJSONDraft)
					if u.patDraft != "" && purpose == "runtime" {
						var e error
						raw, e = account.NormalizePAT(raw, u.patDraft, u.expiryDraft)
						if e != nil {
							u.fail(e)
							return
						}
					}
					u.clearCredentialDraft()
					u.task("Import credential", func(ctx context.Context) (any, error) {
						defer clear(raw)
						if purpose == "quota-session" {
							return nil, u.core.Accounts.UpdateQuotaAuth(ctx, id, raw, true)
						}
						return nil, u.core.Accounts.ImportAuth(ctx, id, raw, true)
					}, u.changed)
				}
			})
		})
	})
	// Escape/outside dismissal must discard secret drafts as well.
	if !u.credentialOpen {
		u.authJSONDraft = ""
		u.patDraft = ""
		u.expiryDraft = ""
	}

	ui.Modal(c, &u.profileOpen, func() {
		ui.Column(c).Width(500).Padding(18).Gap(14).Radius(16).Background(p.Surface).Border(1, p.Border).Shadow(0, 4, 14, 0, rootShadowMd(p)).Children(func() {
			ui.Row(c).Height(46).Children(func() {
				label := "Add Account"
				if u.profileEdit {
					label = "Edit Account"
				}
				ui.Text(c, label).Bold().FontSize(19).TextColor(p.Ink).Grow(1)
				if ui.Button(c, "Close").Clicked() {
					u.profileOpen = false
					u.profileImportJSON = ""
				}
			})
			ui.Divider(c)
			if !u.profileEdit {
				// Preserve the original Profile Account / PAT Account rail.
				availability := u.settings.ModeAvailability
				showTabs := availability == "both" || u.settings.AccountMode == "pat"
				if showTabs {
					ui.Tabs(c, &u.profileCreateModeTab, "Profile Account", "PAT Account")
					if u.profileCreateModeTab == 1 {
						u.profileDraft.AuthMode = "pat"
					} else {
						u.profileDraft.AuthMode = "profile"
					}
				} else {
					if availability == "pat" || u.settings.AccountMode == "pat" {
						u.profileDraft.AuthMode = "pat"
					} else {
						u.profileDraft.AuthMode = "profile"
					}
					ui.Row(c).Padding(10).Radius(9).Border(1, p.Border).Background(p.SurfaceAlt).Center().Children(func() {
						ui.Text(c, map[string]string{"profile": "Profile Account", "pat": "PAT Account"}[u.profileDraft.AuthMode]).Bold().FontSize(14).TextColor(p.Accent)
					})
				}
				if u.profileDraft.AuthMode == "profile" {
					ui.Tabs(c, &u.profileCreateMethodTab, "CLI Auth", "Import Session")
				} else {
					ui.Tabs(c, &u.profileCreateMethodTab, "Token & Session", "Upload auth.json")
				}
				ui.Field(c, "Account name *", func() { ui.TextInput(c, &u.profileDraft.Name).AutoFocus() })
				if u.profileDraft.AuthMode == "pat" {
					if u.profileCreateMethodTab == 0 {
						// Token & Session: complete PAT pairing with secondary session quota
						ui.Field(c, "Personal Access Token *", func() { ui.TextInput(c, &u.patDraft).Password() })
						ui.Row(c).Padding(10).Radius(10).Background(p.AmberSoft).Border(1, p.Amber.Alpha(.2)).Children(func() {
							ui.Text(c, "ⓘ Sensitive credentials — Do not share or expose session JSON files. Only paste trusted session data.").FontSize(12).TextColor(p.Amber)
						})
						ui.Field(c, "Session JSON *", func() { ui.TextArea(c, &u.profileImportJSON).Height(130) })
						ui.Field(c, "Token Expiration (optional, e.g. RFC3339)", func() { ui.TextInput(c, &u.expiryDraft) })
					} else {
						// Direct auth.json upload / paste
						ui.Field(c, "auth.json content *", func() { ui.TextArea(c, &u.profileImportJSON).Height(180) })
					}
				} else if u.profileCreateMethodTab == 1 {
					ui.Row(c).Padding(12).Radius(10).Background(p.AmberSoft).Border(1, p.Amber.Alpha(.2)).Children(func() {
						ui.Text(c, "ⓘ Sensitive credentials — Do not share or expose session JSON files. Only paste trusted session data.").FontSize(12).TextColor(p.Amber)
					})
					ui.Field(c, "Session JSON *", func() { ui.TextArea(c, &u.profileImportJSON).Height(210) })
				} else {
					ui.Field(c, "Copy config from", func() { ui.TextInput(c, &u.configTemplate) })
					ui.Field(c, "Existing CODEX_HOME (optional)", func() { ui.TextInput(c, &u.profileHome) })
				}
				ui.Column(c).Padding(12).Gap(10).Radius(10).Background(p.SurfaceAlt).Border(1, p.Border).Children(func() {
					root := "~/.codex-" + u.profileDraft.Name
					if u.profileDraft.Name == "" {
						root = "~/.codex-name"
					}
					if u.profileHome != "" {
						root = u.profileHome
					}
					ui.Row(c).Children(func() { infoLine(c, p, "CODEX_HOME"); ui.Spacer(c).Grow(1); ui.Text(c, root).Bold().FontSize(12) })
					ui.Row(c).Children(func() {
						infoLine(c, p, "Auth")
						ui.Spacer(c).Grow(1)
						method := "Codex CLI login"
						if u.profileDraft.AuthMode == "pat" {
							if u.profileCreateMethodTab == 0 {
								method = "PAT Runtime + Quota Session"
							} else {
								method = "Direct auth.json"
							}
						} else if u.profileCreateMethodTab == 1 {
							method = "Converted Codex auth.json"
						}
						ui.Text(c, method).Bold().FontSize(12)
					})
				})
				// The original wrapper override choice is shown, but do not
				// pretend it is honored until the write-policy contract is
				// implemented in Core. Existing wrappers are handled by Core.
				ui.Column(c).Padding(10).Radius(9).Background(p.SurfaceAlt).Border(1, p.Border).Children(func() {
					ui.Checkbox(c, &u.profileOverwriteWrapper, "Overwrite wrapper if it exists").Disabled(true)
					infoLine(c, p, "Wrapper policy is controlled by the current Go runtime. This option is not yet available.")
				})
			} else {
				ui.Field(c, u.t("name"), func() { ui.TextInput(c, &u.profileDraft.Name).AutoFocus() })
				ui.Field(c, u.t("note"), func() { ui.TextInput(c, &u.profileDraft.Note) })
				ui.Field(c, u.t("renewal"), func() { ui.TextInput(c, &u.profileDraft.RenewalDate) })
				ui.Field(c, u.t("proxy"), func() { ui.Select(c, &u.profileDraft.Proxy.Mode, []string{"inherit", "disabled", "manual"}) })
				if u.profileDraft.Proxy.Mode == "manual" {
					ui.Field(c, "HTTP_PROXY", func() { ui.TextInput(c, &u.profileDraft.Proxy.HTTP) })
					ui.Field(c, "HTTPS_PROXY", func() { ui.TextInput(c, &u.profileDraft.Proxy.HTTPS) })
					ui.Field(c, "ALL_PROXY", func() { ui.TextInput(c, &u.profileDraft.Proxy.All) })
					ui.Field(c, "NO_PROXY", func() { ui.TextInput(c, &u.profileDraft.Proxy.NoProxy) })
				}
			}
			u.formError(c)
			ui.Divider(c)
			ui.Row(c).Gap(12).Children(func() {
				if ui.Button(c, u.t("cancel")).Clicked() {
					u.profileOpen = false
					u.profileImportJSON = ""
					u.patDraft = ""
					u.expiryDraft = ""
				}
				ui.Spacer(c).Grow(1)
				label := "Create Profile"
				if u.profileEdit {
					label = "Save Changes"
				} else if u.profileDraft.AuthMode == "pat" || u.profileCreateMethodTab == 1 {
					label = "Import Profile"
				}
				if ui.PrimaryButton(c, label).Disabled(u.busy).Clicked() {
					p, home, edit, rev := u.profileDraft, u.profileHome, u.profileEdit, u.basic.State.Revision
					template := u.configTemplate
					isPAT := !edit && p.AuthMode == "pat"
					isPATTokenSession := isPAT && u.profileCreateMethodTab == 0
					importing := !edit && (isPAT || u.profileCreateMethodTab == 1)
					raw := []byte(u.profileImportJSON)
					patToken := u.patDraft
					expiry := u.expiryDraft

					if isPATTokenSession && strings.TrimSpace(patToken) == "" {
						u.fail(fault.New("INPUT_INVALID", "Personal Access Token is required"))
						return
					}
					if importing && !json.Valid(raw) {
						clear(raw)
						u.fail(fault.New("INPUT_INVALID", "Valid Session or Auth JSON is required"))
						return
					}
					u.profileImportJSON = ""
					u.patDraft = ""
					u.expiryDraft = ""
					u.task(label, func(ctx context.Context) (any, error) {
						defer clear(raw)
						if edit {
							return nil, u.core.Accounts.Edit(ctx, p.ID, p.Name, p.Note, p.RenewalDate, p.Proxy, rev)
						}
						if strings.TrimSpace(home) != "" {
							return u.core.Accounts.Register(ctx, home, p.Name, rev)
						}
						created, e := u.core.CreateProfile(ctx, p.Name, p.AuthMode, rev, template)
						if e != nil {
							return nil, e
						}
						if isPATTokenSession {
							normalized, normErr := account.NormalizePAT(raw, patToken, expiry)
							if normErr != nil {
								return nil, normErr
							}
							if e := u.core.Accounts.ImportAuth(ctx, created.ID, normalized, true); e != nil {
								return nil, fault.New("IMPORT_AUTH_FAILED", "Profile was created but PAT credentials could not be imported: "+e.Error())
							}
						} else if importing {
							if e := u.core.Accounts.ImportAuth(ctx, created.ID, raw, true); e != nil {
								return nil, fault.New("IMPORT_AUTH_FAILED", "Profile was created but credentials could not be imported. Use the account menu to retry; the profile was not deleted")
							}
						}
						return created, nil
					}, func(any) { u.profileOpen = false; u.reload() })
				}
			})
		})
	})
	if !u.profileOpen {
		u.profileImportJSON = ""
		u.patDraft = ""
		u.expiryDraft = ""
	}

	ui.Modal(c, &u.providerOpen, func() {
		ui.Column(c).Width(780).Gap(12).Padding(18).Radius(16).Background(p.Surface).Border(1, p.Border).Shadow(0, 4, 14, 0, rootShadowMd(p)).Children(func() {
			ui.Row(c).Height(48).Children(func() {
				title := "Add External API"
				if u.providerDraft.ID != "" {
					title = "Edit API Account"
				}
				ui.Text(c, title).Bold().FontSize(19).TextColor(p.Ink).Grow(1)
				if ui.Button(c, "Close").Clicked() {
					u.providerOpen = false
					u.secretDraft = ""
				}
			})
			ui.Divider(c)
			ui.Scroll(c).Height(510).Gap(8).Children(func() {
				if ui.Button(c, "Advanced options  +").Clicked() {
					current := u.providerDraft
					u.providerOpen = false
					u.editJSON("Advanced options", current, func(raw string) {
						var v model.Provider
						if json.Unmarshal([]byte(raw), &v) != nil {
							u.fail(fault.New("INPUT_INVALID", "Invalid provider JSON"))
							return
						}
						u.jsonOpen = false
						u.openProvider(v)
					})
				}
				ui.Row(c).Gap(12).Children(func() {
					softCard(c, p).Grow(1).Children(func() {
						cardHeader(c, p, "1 · Connection details", "Endpoint and authentication")
						ui.Field(c, u.t("name"), func() { ui.TextInput(c, &u.providerDraft.Name) })
						ui.Field(c, u.t("protocol"), func() { ui.Select(c, &u.providerDraft.Protocol, []string{"responses", "chat_completions"}) })
						ui.Field(c, u.t("baseURL"), func() { ui.TextInput(c, &u.providerDraft.BaseURL) })
						ui.Field(c, u.t("authKind"), func() { ui.Select(c, &u.providerDraft.AuthKind, []string{"bearer", "api_key", "none"}) })
						if u.providerDraft.AuthKind == "api_key" {
							ui.Field(c, "API key header", func() { ui.TextInput(c, &u.providerDraft.AuthHeader) })
						}
						if u.providerDraft.AuthKind != "none" {
							ui.Field(c, "Credential source", func() { ui.Select(c, &u.credentialKind, []string{"keychain", "env", "command", "codex_profile"}) })
							if u.credentialKind == "env" {
								ui.Field(c, "Environment variable", func() { ui.TextInput(c, &u.credentialEnv) })
							}
							if u.credentialKind == "keychain" {
								ui.Field(c, u.t("secret"), func() { ui.TextInput(c, &u.secretDraft).Password() })
							}
							if u.credentialKind == "codex_profile" {
								ui.Field(c, "API-key profile", func() { ui.Select(c, &u.providerDraft.Credential.ProfileID, u.profileIDs(false)) })
							}
							if u.credentialKind == "command" {
								ui.Field(c, "Explicitly approved command ID", func() { ui.TextInput(c, &u.providerDraft.Credential.ApprovalID) })
							}
						}
					})
					softCard(c, p).Grow(1).Children(func() {
						cardHeader(c, p, "2 · Model configuration", "Supported models and capabilities")
						ui.Field(c, u.t("modelList"), func() { ui.TextArea(c, &u.modelDraft).Height(90) })
						ui.Field(c, u.t("defaultModel"), func() { ui.TextInput(c, &u.providerDraft.DefaultModel) })
						choices := []string{"unknown", "supported", "partial", "unsupported"}
						ui.Row(c).Gap(8).Children(func() {
							ui.Field(c, "Streaming", func() { ui.Select(c, &u.providerDraft.Capabilities.Streaming, choices) }).Grow(1)
							ui.Field(c, "Function tools", func() { ui.Select(c, &u.providerDraft.Capabilities.FunctionTools, choices) }).Grow(1)
							ui.Field(c, "Images", func() { ui.Select(c, &u.providerDraft.Capabilities.Images, choices) }).Grow(1)
						})
						ui.Row(c).Gap(8).Children(func() {
							ui.Field(c, "Structured output", func() { ui.Select(c, &u.providerDraft.Capabilities.StructuredOutputs, choices) }).Grow(1)
							ui.Field(c, "Reasoning", func() { ui.Select(c, &u.providerDraft.Capabilities.Reasoning, choices) }).Grow(1)
							ui.Field(c, "Custom tools", func() { ui.Select(c, &u.providerDraft.Capabilities.CustomTools, choices) }).Grow(1)
							ui.Field(c, "Namespaces", func() { ui.Select(c, &u.providerDraft.Capabilities.Namespaces, choices) }).Grow(1)
						})
					})
				})
				softCard(c, p).Children(func() {
					cardHeader(c, p, "3 · Review API Account", "Review provider endpoints and capabilities before saving")
				})
			})
			u.formError(c)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, u.t("cancel")).Clicked() {
					u.providerOpen = false
					u.secretDraft = ""
				}
				saveLabel := "Create API Account"
				if u.providerDraft.ID != "" {
					saveLabel = "Save Changes"
				}
				if ui.PrimaryButton(c, saveLabel).Disabled(u.busy).Clicked() {
					p := u.providerDraft
					old := map[string]model.ProviderModel{}
					for _, m := range p.Models {
						old[m.ID] = m
					}
					p.Models = nil
					for _, id := range strings.Fields(u.modelDraft) {
						m := old[id]
						m.ID = id
						p.Models = append(p.Models, m)
					}
					p.Credential.Kind = u.credentialKind
					p.Credential.EnvKey = u.credentialEnv
					key, rev := u.secretDraft, u.basic.State.Revision
					u.secretDraft = ""
					u.task(u.t("save"), func(ctx context.Context) (any, error) { return u.core.Providers.Save(ctx, p, key, rev) }, func(v any) {
						u.providerOpen = false
						u.reload()
						if p.ID == "" {
							created := v.(model.Provider)
							u.bindProvider = created.ID
							u.bindModel = created.DefaultModel
							u.apiAccountName = created.Name
							u.apiAccountOpen = true
						}
					})
				}
			})
		})
	})
	ui.Modal(c, &u.loginOpen, func() {
		ui.Column(c).Width(520).Gap(10).Padding(20).Radius(16).Background(p.Surface).Border(1, p.Border).Shadow(0, 4, 14, 0, rootShadowMd(p)).Children(func() {
			ui.Text(c, u.t("login")).Bold().FontSize(19).TextColor(p.Ink)
			ui.Field(c, u.t("mode"), func() { ui.Select(c, &u.loginKind, []string{"chatgpt", "chatgptDeviceCode", "apiKey"}) })
			if u.loginKind == "apiKey" {
				ui.Field(c, "API key", func() { ui.TextInput(c, &u.loginKey).Password() })
			}
			u.formError(c)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, u.t("cancel")).Clicked() {
					u.loginOpen = false
					u.loginKey = ""
				}
				if ui.PrimaryButton(c, u.t("login")).Disabled(u.busy).Clicked() {
					id, kind, key := u.bindProfile, u.loginKind, u.loginKey
					u.loginKey = ""
					u.task(u.t("login"), func(ctx context.Context) (any, error) { return u.core.Login(ctx, id, kind, key) }, func(v any) {
						u.loginOpen = false
						raw := v.(json.RawMessage)
						var msg map[string]json.RawMessage
						_ = json.Unmarshal(raw, &msg)
						visible := map[string]json.RawMessage{}
						for _, k := range []string{"type", "loginId", "authUrl", "userCode", "verificationUrl", "verificationUri", "expiresIn"} {
							if x, ok := msg[k]; ok {
								visible[k] = x
							}
						}
						var url string
						_ = json.Unmarshal(msg["authUrl"], &url)
						if strings.HasPrefix(url, "https://") {
							_ = mygo.Shell.OpenExternal(url)
						}
						u.info(u.t("login"), visible)
						u.reload()
					})
				}
			})
		})
	})
	ui.Modal(c, &u.bindOpen, func() {
		ui.Column(c).Width(600).Gap(10).Padding(20).Radius(16).Background(p.Surface).Border(1, p.Border).Shadow(0, 4, 14, 0, rootShadowMd(p)).Children(func() {
			ui.Text(c, u.t("bind")).Bold()
			ui.Text(c, u.bindProvider)
			ui.Field(c, "Profile ID", func() { ui.Select(c, &u.bindProfile, u.profileIDs(false)) })
			ui.Field(c, u.t("defaultModel"), func() { ui.TextInput(c, &u.bindModel) })
			u.formError(c)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, u.t("cancel")).Clicked() {
					u.bindOpen = false
				}
				if ui.Button(c, u.t("detach")).Disabled(u.bindProfile == "" || u.busy).Clicked() {
					id := u.bindProfile
					u.bindOpen = false
					u.task("Preview detach", func(ctx context.Context) (any, error) { return u.core.Providers.PlanDetach(ctx, id) }, func(v any) {
						plan := v.(provider.DetachPlan)
						u.confirm("Detach provider", app.Pretty(plan), func() {
							u.task("Detach provider", func(ctx context.Context) (any, error) {
								return nil, u.core.Providers.CommitDetach(ctx, plan.ID, plan.Fingerprint)
							}, u.changed)
						})
					})
				}
				if ui.PrimaryButton(c, u.t("plan")).Disabled(u.busy).Clicked() {
					id, pr, m := u.bindProfile, u.bindProvider, u.bindModel
					u.task(u.t("plan"), func(ctx context.Context) (any, error) { return u.core.Providers.PlanBind(ctx, id, pr, m) }, func(v any) {
						p := v.(provider.BindPlan)
						u.bindOpen = false
						u.confirm(u.t("bind"), app.Pretty(p), func() {
							u.task(u.t("bind"), func(ctx context.Context) (any, error) { return u.core.Providers.CommitBind(ctx, p.ID, p.Fingerprint) }, u.changed)
						})
					})
				}
			})
		})
	})
	ui.Modal(c, &u.apiAccountOpen, func() {
		ui.Column(c).Width(600).Gap(10).Padding(20).Radius(16).Background(p.Surface).Border(1, p.Border).Shadow(0, 4, 14, 0, rootShadowMd(p)).Children(func() {
			ui.Text(c, "New API account").Bold()
			ui.Text(c, u.bindProvider)
			ui.TextInput(c, &u.apiAccountName).Label("Display name").AutoFocus()
			models := []string{}
			for _, m := range u.basic.State.Providers[u.bindProvider].Models {
				models = append(models, m.ID)
			}
			ui.Select(c, &u.bindModel, models).Label("Model")
			u.formError(c)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, u.t("cancel")).Clicked() {
					u.apiAccountOpen = false
				}
				if ui.PrimaryButton(c, u.t("plan")).Disabled(u.busy).Clicked() {
					name, pr, m := u.apiAccountName, u.bindProvider, u.bindModel
					u.task(u.t("plan"), func(ctx context.Context) (any, error) { return u.core.APIAccounts.PlanCreate(ctx, name, pr, m) }, func(v any) {
						p := v.(provider.APIAccountPlan)
						u.apiAccountOpen = false
						u.confirm("Create API account", app.Pretty(p), func() {
							u.task("Create API account", func(ctx context.Context) (any, error) {
								return u.core.APIAccounts.ExecuteCreate(ctx, p.ID, p.Fingerprint)
							}, u.changed)
						})
					})
				}
			})
		})
	})
	ui.Modal(c, &u.handoffOpen, func() {
		ui.Column(c).Width(600).Gap(10).Padding(20).Radius(16).Background(p.Surface).Border(1, p.Border).Shadow(0, 4, 14, 0, rootShadowMd(p)).Children(func() {
			ui.Text(c, u.t("handoff")).Bold().FontSize(19).TextColor(p.Ink)
			ui.Text(c, u.handoffSource+" / "+u.handoffSession)
			ui.Field(c, u.t("target"), func() { ui.Select(c, &u.handoffTarget, u.profileIDs(false)) })
			ui.Field(c, u.t("policy"), func() {
				ui.Select(c, &u.handoffPolicy, []string{"stop_and_ask", "prefer_source", "prefer_target", "timeline_merge_to_fork", "summarize_fork_with_target_account", "fork"})
			})
			ui.Checkbox(c, &u.handoffProviderChange, "Allow explicit provider change after reviewing the preview")
			u.formError(c)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, u.t("cancel")).Clicked() {
					u.handoffOpen = false
				}
				if ui.PrimaryButton(c, u.t("plan")).Disabled(u.busy).Clicked() {
					src, sid, dst, policy, allow := u.handoffSource, u.handoffSession, u.handoffTarget, u.handoffPolicy, u.handoffProviderChange
					u.task(u.t("plan"), func(ctx context.Context) (any, error) { return u.core.Handoff.Plan(ctx, src, sid, dst, policy, allow) }, func(v any) {
						p := v.(handoff.Plan)
						u.handoffOpen = false
						u.confirm(u.t("handoff"), app.Pretty(p), func() {
							u.task(u.t("handoff"), func(ctx context.Context) (any, error) { return u.core.Handoff.Execute(ctx, p.ID, p.Fingerprint) }, func(v any) {
								r := v.(handoff.Result)
								u.reload()
								u.confirm("Resume in target account", app.Pretty(r), func() {
									if r.ResumePrompt != "" {
										u.launch(dst, r.SessionID, p.Source.CWD, r.ResumePrompt)
									} else {
										u.launch(dst, r.SessionID, p.Source.CWD)
									}
								})
							})
						})
					})
				}
			})
		})
	})
	ui.Modal(c, &u.infoOpen, func() {
		ui.Column(c).Width(700).Gap(10).Padding(20).Radius(16).Background(p.Surface).Border(1, p.Border).Shadow(0, 4, 14, 0, rootShadowMd(p)).Children(func() {
			ui.Text(c, u.infoTitle).Bold().FontSize(19).TextColor(p.Ink)
			ui.TextArea(c, &u.infoText).ReadOnly(true).Height(420)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, u.t("copy")).Clicked() {
					mygo.Clipboard.WriteText(u.infoText)
				}
				if ui.Button(c, u.t("close")).Clicked() {
					u.infoOpen = false
				}
			})
		})
	})
	ui.Modal(c, &u.jsonOpen, func() {
		ui.Column(c).Width(700).Gap(10).Padding(20).Radius(16).Background(p.Surface).Border(1, p.Border).Shadow(0, 4, 14, 0, rootShadowMd(p)).Children(func() {
			ui.Text(c, u.jsonTitle).Bold().FontSize(19).TextColor(p.Ink)
			ui.TextArea(c, &u.jsonText).Height(420)
			u.formError(c)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, u.t("cancel")).Clicked() {
					u.jsonOpen = false
				}
				if ui.PrimaryButton(c, u.t("save")).Disabled(u.busy).Clicked() && u.jsonSave != nil {
					u.jsonSave(u.jsonText)
				}
			})
		})
	})
	ui.Modal(c, &u.confirmOpen, func() {
		ui.Column(c).Width(680).Gap(10).Padding(20).Radius(16).Background(p.Surface).Border(1, p.Border).Shadow(0, 4, 14, 0, rootShadowMd(p)).Children(func() {
			ui.Text(c, u.confirmTitle).Bold().FontSize(19).TextColor(p.Ink)
			ui.TextArea(c, &u.confirmText).ReadOnly(true).Height(260)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, u.t("cancel")).Clicked() {
					u.confirmOpen = false
					u.confirmAction = nil
				}
				if ui.PrimaryButton(c, u.t("confirm")).Disabled(u.busy).Clicked() {
					fn := u.confirmAction
					u.confirmOpen = false
					u.confirmAction = nil
					if fn != nil {
						fn()
					}
				}
			})
		})
	})
	// Secrets are removed when Escape or an outside click dismisses a dialog too.
	if !u.providerOpen {
		u.secretDraft = ""
	}
	if !u.loginOpen {
		u.loginKey = ""
	}
}
func (u *UI) formError(c *ui.Context) {
	if u.errorText != "" {
		errorBanner(c, paletteFor(c.Theme().Dark), u.errorText)
	}
}
