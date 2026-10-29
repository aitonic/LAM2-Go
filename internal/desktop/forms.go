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
	u.profileDraft = model.Profile{AuthMode: u.settings.AccountMode, Proxy: model.Proxy{Mode: "inherit"}}
	u.profileHome = ""
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
	ui.Modal(c, &u.credentialOpen, func() {
		ui.Column(c).Width(640).Gap(8).Children(func() {
			ui.Text(c, "Import Session / PAT").Bold()
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
		ui.Column(c).Width(620).Gap(10).Children(func() {
			ui.Text(c, u.t("newProfile")).Bold()
			ui.Field(c, u.t("name"), func() { ui.TextInput(c, &u.profileDraft.Name).AutoFocus() })
			if !u.profileEdit {
				ui.Field(c, u.t("mode"), func() { ui.Select(c, &u.profileDraft.AuthMode, []string{"profile", "pat"}) })
				ui.Field(c, u.t("home"), func() { ui.TextInput(c, &u.profileHome) })
				ui.Field(c, "Copy config from (optional Profile ID)", func() { ui.TextInput(c, &u.configTemplate).Disabled(u.profileHome != "") })
			} else {
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
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, u.t("cancel")).Clicked() {
					u.profileOpen = false
				}
				if ui.PrimaryButton(c, u.t("save")).Disabled(u.busy).Clicked() {
					p, home, edit, rev := u.profileDraft, u.profileHome, u.profileEdit, u.basic.State.Revision
					template := u.configTemplate
					u.task(u.t("save"), func(ctx context.Context) (any, error) {
						if edit {
							return nil, u.core.Accounts.Edit(ctx, p.ID, p.Name, p.Note, p.RenewalDate, p.Proxy, rev)
						}
						if strings.TrimSpace(home) != "" {
							return u.core.Accounts.Register(ctx, home, p.Name, rev)
						}
						return u.core.CreateProfile(ctx, p.Name, p.AuthMode, rev, template)
					}, func(v any) { u.profileOpen = false; u.reload() })
				}
			})
		})
	})
	ui.Modal(c, &u.providerOpen, func() {
		ui.Column(c).Width(680).Gap(8).Children(func() {
			ui.Text(c, u.t("providers")).Bold()
			ui.Scroll(c).Height(510).Gap(8).Children(func() {
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
				if ui.Button(c, u.t("advanced")).Clicked() {
					p := u.providerDraft
					u.providerOpen = false
					u.editJSON(u.t("advanced"), p, func(raw string) {
						var v model.Provider
						if json.Unmarshal([]byte(raw), &v) != nil {
							u.fail(fault.New("INPUT_INVALID", "Invalid provider JSON"))
							return
						}
						u.jsonOpen = false
						u.openProvider(v)
					})
				}
			})
			u.formError(c)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, u.t("cancel")).Clicked() {
					u.providerOpen = false
					u.secretDraft = ""
				}
				if ui.PrimaryButton(c, u.t("save")).Disabled(u.busy).Clicked() {
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
					u.task(u.t("save"), func(ctx context.Context) (any, error) { return u.core.Providers.Save(ctx, p, key, rev) }, func(v any) { u.providerOpen = false; u.reload() })
				}
			})
		})
	})
	ui.Modal(c, &u.loginOpen, func() {
		ui.Column(c).Width(520).Gap(10).Children(func() {
			ui.Text(c, u.t("login")).Bold()
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
		ui.Column(c).Width(600).Gap(10).Children(func() {
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
		ui.Column(c).Width(600).Gap(10).Children(func() {
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
		ui.Column(c).Width(600).Gap(10).Children(func() {
			ui.Text(c, u.t("handoff")).Bold()
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
		ui.Column(c).Width(700).Gap(10).Children(func() {
			ui.Text(c, u.infoTitle).Bold()
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
		ui.Column(c).Width(700).Gap(10).Children(func() {
			ui.Text(c, u.jsonTitle).Bold()
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
		ui.Column(c).Width(680).Gap(10).Children(func() {
			ui.Text(c, u.confirmTitle).Bold()
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
		ui.Text(c, u.errorText).FontSize(12)
	}
}
