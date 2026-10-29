package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"lam.local/lam2/internal/app"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/migration"
	"lam.local/lam2/internal/model"
	run "lam.local/lam2/internal/runtime"
	"lam.local/lam2/internal/session"
	"lam.local/lam2/internal/storage"
)

func (u *UI) refreshQuotas() {
	ids := u.profileIDs(false)
	u.task(u.t("refreshQuota"), func(ctx context.Context) (any, error) { return u.core.RefreshQuotas(ctx, ids) }, func(v any) {
		u.basic.Quotas = v.(map[string]model.QuotaSnapshot)
		u.nextQuota = time.Now().Add(time.Duration(u.settings.RefreshSeconds) * time.Second)
	})
}
func (u *UI) indexUsage() {
	id, win := u.usageProfile, u.win
	u.task(u.t("index"), func(ctx context.Context) (any, error) {
		return u.core.RefreshUsage(ctx, id, func(p app.IndexProgress) {
			if ctx.Err() != nil || u.closed.Load() {
				return
			}
			win.Update(func() {
				if !u.closed.Load() {
					u.status = fmt.Sprintf("%d/%d files | %d events | %d diagnostics", p.Files, p.Total, p.Events, p.Diagnostics)
				}
			})
		})
	}, func(v any) { u.info(u.t("index"), v); u.reload() })
}
func (u *UI) archiveSelected() {
	refs := make([]session.Ref, 0, len(u.selected))
	for _, r := range u.selected {
		refs = append(refs, r)
	}
	u.task(u.t("plan"), func(ctx context.Context) (any, error) { return u.core.Sessions.PlanDelete(ctx, refs) }, func(v any) {
		p := v.(session.DeletePlan)
		if len(p.Items) == 0 {
			u.info(u.t("archive"), p)
			return
		}
		u.confirm(u.t("archive"), app.Pretty(p), func() {
			u.task(u.t("archive"), func(ctx context.Context) (any, error) { return p.ID, u.core.Sessions.Delete(ctx, p.ID, true) }, func(v any) {
				u.selected = map[string]session.Ref{}
				u.info(u.t("archive"), map[string]string{"archiveId": p.ID})
				u.reload()
			})
		})
	})
}
func (u *UI) importAuth(id string) {
	win := u.win
	u.confirm(u.t("import"), "Import authentication into only the selected profile. The credential snapshot is encrypted using a system-keystore key.", func() {
		u.task(u.t("import"), func(ctx context.Context) (any, error) {
			paths, e := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "Import authentication", Filters: []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json"}}}})
			if e != nil || len(paths) == 0 {
				return nil, e
			}
			b, e := storage.ReadFile(paths[0], 8<<20)
			if e != nil {
				return nil, e
			}
			defer clear(b)
			return nil, u.core.Accounts.ImportAuth(ctx, id, b, true)
		}, u.changed)
	})
}
func (u *UI) exportAuth(id string) {
	win := u.win
	u.confirm(u.t("export"), "This explicitly exports secret credentials into a private JSON file. Keep the exported file private and delete it when no longer needed.", func() {
		u.task(u.t("export"), func(ctx context.Context) (any, error) {
			path, e := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: win, DefaultPath: "lam-credentials.json"})
			if e != nil || path == "" {
				return nil, e
			}
			b, e := u.core.Accounts.ExportCPA(ctx, id, true)
			if e != nil {
				return nil, e
			}
			defer clear(b)
			return nil, storage.AtomicWrite(ctx, path, b, 0600)
		}, nil)
	})
}
func (u *UI) resetCredit(id string) {
	u.task(u.t("plan"), func(ctx context.Context) (any, error) { return u.core.Quota.PrepareReset(ctx, id, "") }, func(v any) {
		op := v.(string)
		u.confirm(u.t("resetCredit"), "Consume one server-reported reset credit. This operation cannot be rolled back. Retries use the same operation ID: "+op, func() {
			u.task(u.t("resetCredit"), func(ctx context.Context) (any, error) { return u.core.Quota.ConsumeReset(ctx, op, true) }, func(v any) {
				u.info(u.t("resetCredit"), map[string]any{"operationId": op, "result": v})
				u.refreshQuotas()
			})
		})
	})
}
func (u *UI) importLegacy() {
	win := u.win
	u.task(u.t("migrate"), func(ctx context.Context) (any, error) {
		paths, e := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "Choose legacy .lam directory", Directory: true})
		if e != nil || len(paths) == 0 {
			return nil, e
		}
		return u.core.Migration.Plan(ctx, paths[0])
	}, func(v any) {
		if v == nil {
			return
		}
		p := v.(migration.Plan)
		u.confirm(u.t("migrate"), app.Pretty(p), func() {
			u.task(u.t("migrate"), func(ctx context.Context) (any, error) { return p.ID, u.core.Migration.Apply(ctx, p.ID, p.Fingerprint) }, func(v any) {
				u.info(u.t("migrate"), map[string]any{"migrationId": p.ID, "warnings": p.Warnings})
				u.reload()
			})
		})
	})
}
func (u *UI) restoreArchive() {
	u.editJSON(u.t("restore"), map[string]string{"archiveId": ""}, func(raw string) {
		var v struct {
			ArchiveID string `json:"archiveId"`
		}
		if json.Unmarshal([]byte(raw), &v) != nil {
			u.fail(fault.New("INPUT_INVALID", "Invalid JSON"))
			return
		}
		u.jsonOpen = false
		u.confirm(u.t("restore"), "Restore only unchanged archived session files: "+v.ArchiveID, func() {
			u.task(u.t("restore"), func(ctx context.Context) (any, error) { return nil, u.core.Sessions.Restore(ctx, v.ArchiveID, true) }, u.changed)
		})
	})
}
func (u *UI) rollbackImport() {
	u.editJSON(u.t("rollback"), map[string]string{"migrationId": ""}, func(raw string) {
		var v struct {
			ID string `json:"migrationId"`
		}
		if json.Unmarshal([]byte(raw), &v) != nil {
			u.fail(fault.New("INPUT_INVALID", "Invalid JSON"))
			return
		}
		u.jsonOpen = false
		u.confirm(u.t("rollback"), "Rollback imported metadata only if the new store is unchanged since import.", func() {
			u.task(u.t("rollback"), func(ctx context.Context) (any, error) { return nil, u.core.Migration.Rollback(ctx, v.ID, true) }, u.changed)
		})
	})
}
func (u *UI) revealHome() {
	if u.core != nil {
		_ = mygo.Shell.OpenPath(filepath.Clean(u.core.Store.Root))
	}
}

func (u *UI) copyResume(p, s, cwd string) {
	line, e := run.LauncherCommand(u.core.Executable, u.core.Store.Root, p, s, cwd)
	if e != nil {
		u.fail(e)
		return
	}
	mygo.Clipboard.WriteText(line)
	u.status = "Resume command copied (no credential embedded)"
}
