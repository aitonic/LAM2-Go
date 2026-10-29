// Package desktop composes MyGo native widgets with Go business services.
// UI state is mutated only on the main thread through Window.Update.
package desktop

import (
	"context"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"lam.local/lam2/internal/app"
	"lam.local/lam2/internal/desktop/messages"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/session"
	"lam.local/lam2/internal/usage"
	"sync"
	"sync/atomic"
	"time"
)

type UI struct {
	configTemplate                                                 string
	credentialOpen                                                 bool
	authJSONDraft, patDraft, expiryDraft, authPurpose, authProfile string

	readGeneration          uint64
	readCancel              context.CancelFunc
	reading                 bool
	threadOffset, threadRow int
	usageSource             string
	usagePreset             string
	usageLoadLimit          string
	apiAccountName          string
	apiAccountCreate        bool
	apiAccountOpen          bool
	activityMode            string
	activityMetric          string
	antiPort                string
	trayOpacity             string
	trayTab                 int
	usageTabIndex           int
	threadTable             ui.ListState
	watchers                map[string]context.CancelFunc
	watchDirty              atomic.Bool

	runtimeFieldsDirty                                                                           bool
	firstTimeout, requestTimeout, refreshInterval                                                string
	core                                                                                         *app.Core
	ctx                                                                                          context.Context
	cancel                                                                                       context.CancelFunc
	wg                                                                                           sync.WaitGroup
	closed                                                                                       atomic.Bool
	win, trayWin                                                                                 *mygo.Window
	tray                                                                                         *mygo.Tray
	router                                                                                       *ui.Router
	locale                                                                                       string
	basic                                                                                        app.Basic
	quotaExtra                                                                                   model.QuotaSnapshot
	sessions                                                                                     model.Page[model.Session]
	usage                                                                                        app.UsageView
	query                                                                                        session.Query
	filter                                                                                       usage.Filter
	sessionTable, providerTable, profileTable, callTable                                         ui.ListState
	profileRow, providerRow, sessionRow, callRow                                                 int
	selected                                                                                     map[string]session.Ref
	queryProfile, usageProfile, after, before, usageTab                                          string
	busy                                                                                         bool
	status, errorText                                                                            string
	jobCancel                                                                                    context.CancelFunc
	quitting                                                                                     bool
	nextQuota                                                                                    time.Time
	nextAntigravity                                                                              time.Time
	profileOpen, providerOpen, loginOpen, bindOpen, handoffOpen, infoOpen, confirmOpen, jsonOpen bool
	profileEdit                                                                                  bool
	profileDraft                                                                                 model.Profile
	profileHome                                                                                  string
	providerDraft                                                                                model.Provider
	modelDraft, secretDraft, credentialEnv, credentialKind, advancedDraft                        string
	loginKind, loginKey                                                                          string
	bindProfile, bindProvider, bindModel                                                         string
	handoffSource, handoffSession, handoffTarget, handoffPolicy                                  string
	handoffProviderChange                                                                        bool
	infoTitle, infoText, confirmTitle, confirmText                                               string
	confirmAction                                                                                func()
	jsonTitle, jsonText                                                                          string
	jsonSave                                                                                     func(string)
	settings                                                                                     model.Settings
	settingsDirty                                                                                bool
	updatesConfigured                                                                            bool
}

func New(core *app.Core) *UI {
	ctx, cancel := context.WithCancel(context.Background())
	u := &UI{core: core, ctx: ctx, cancel: cancel, router: ui.NewRouter("/overview"), basic: app.Basic{State: model.NewState(), Quotas: map[string]model.QuotaSnapshot{}}, settings: model.DefaultSettings(), query: session.Query{Sort: "modified", Descending: true, Limit: 50}, filter: usage.Filter{Limit: 50, ArchiveScope: "active"}, selected: map[string]session.Ref{}, profileRow: -1, providerRow: -1, sessionRow: -1, callRow: -1, usageTab: "calls", usageSource: "local", usagePreset: "all", usageLoadLimit: "5000", activityMode: "daily", activityMetric: "calls", threadRow: -1, handoffPolicy: "stop", loginKind: "chatgpt"}
	u.after = ""
	u.before = ""
	u.syncSettingsFields()
	return u
}
func (u *UI) t(key string) string { return messages.Text(u.locale, key) }
func QuotaLabel(v *float64) string {
	if v == nil {
		return "Unknown"
	}
	return fmt.Sprintf("%.1f%%", *v)
}
func (u *UI) fail(e error) {
	if e == nil {
		return
	}
	p := fault.Public(e)
	u.errorText = p.Code + ": " + p.Message
}
func (u *UI) task(label string, work func(context.Context) (any, error), done func(any)) {
	if u.busy || u.core == nil || u.win == nil || u.closed.Load() {
		return
	}
	u.busy = true
	u.errorText = ""
	u.status = label
	ctx, cancel := context.WithCancel(u.ctx)
	u.jobCancel = cancel
	win := u.win
	u.wg.Add(1)
	go func() {
		defer u.wg.Done()
		defer cancel()
		value, e := work(ctx)
		if u.ctx.Err() != nil || u.closed.Load() {
			return
		}
		win.Update(func() {
			if u.closed.Load() {
				return
			}
			u.busy = false
			u.jobCancel = nil
			if e != nil {
				u.fail(e)
				u.status = ""
			} else {
				u.status = u.t("ready")
				if done != nil {
					done(value)
				}
			}
			if u.trayWin != nil {
				u.trayWin.Invalidate()
			}
		})
	}()
}

type loaded struct {
	basic        app.Basic
	sessions     model.Page[model.Session]
	usage        app.UsageView
	page         string
	sectionError error
}

func (u *UI) reload() {
	if u.core == nil {
		return
	}
	page := u.router.Path()
	query := u.query
	query.ProfileID = u.queryProfile
	filter := u.filter
	if u.usageTabIndex == 1 {
		filter.Offset = u.threadOffset
	}
	filter.ProfileID = u.usageProfile
	switch u.usageLoadLimit {
	case "5000":
		filter.DetailLimit = 5000
	case "10000":
		filter.DetailLimit = 10000
	case "20000":
		filter.DetailLimit = 20000
	default:
		filter.DetailLimit = 0 // All calls
	}
	if u.usageTabIndex == 1 {
		filter.Limit = 100
	}
	filter.After = time.Time{}
	filter.Before = time.Time{}
	if page == "/usage" && u.after != "" {
		t, e := time.ParseInLocation("2006-01-02", u.after, time.UTC)
		if e != nil {
			u.fail(fault.New("DATE_INVALID", "Enter YYYY-MM-DD"))
			return
		}
		filter.After = t
	}
	if page == "/usage" && u.before != "" {
		t, e := time.ParseInLocation("2006-01-02", u.before, time.UTC)
		if e != nil {
			u.fail(fault.New("DATE_INVALID", "Enter YYYY-MM-DD"))
			return
		}
		filter.Before = t
	}
	u.readTask(func(ctx context.Context) (any, error) {
		x := loaded{page: page}
		var e error
		x.basic, e = u.core.BasicSnapshot(ctx)
		if e != nil {
			return x, e
		}
		switch page {
		case "/sessions":
			x.sessions, x.sectionError = u.core.Sessions.Query(ctx, query)
		case "/usage":
			x.usage, x.sectionError = u.core.ReadUsage(ctx, filter, time.UTC)
		}
		return x, nil
	}, func(v any) {
		x := v.(loaded)
		if x.basic.State.Revision >= u.basic.State.Revision {
			u.basic = x.basic
		}
		if !u.settingsDirty {
			u.settings = x.basic.State.Settings
			u.syncSettingsFields()
		}
		if x.sectionError != nil {
			u.fail(x.sectionError)
		} else {
			switch x.page {
			case "/sessions":
				u.sessions = x.sessions
				u.sessionRow = -1
			case "/usage":
				u.usage = x.usage
				u.callRow = -1
				u.threadRow = -1
			}
		}
		u.syncWatchers()
	})
}
func (u *UI) changed(_ any) { u.reload() }
func (u *UI) info(title string, value any) {
	u.infoTitle = title
	u.infoText = app.Pretty(value)
	u.infoOpen = true
}
func (u *UI) confirm(title, text string, action func()) {
	u.confirmTitle = title
	u.confirmText = text
	u.confirmAction = action
	u.confirmOpen = true
}
func (u *UI) editJSON(title string, value any, save func(string)) {
	u.jsonTitle = title
	u.jsonText = app.Pretty(value)
	u.jsonSave = save
	u.jsonOpen = true
}
func (u *UI) profile() (model.Profile, bool) {
	if u.profileRow < 0 || u.profileRow >= len(u.basic.Profiles) {
		return model.Profile{}, false
	}
	return u.basic.Profiles[u.profileRow], true
}
func (u *UI) provider() (model.Provider, bool) {
	if u.providerRow < 0 || u.providerRow >= len(u.basic.Providers) {
		return model.Provider{}, false
	}
	return u.basic.Providers[u.providerRow], true
}
func (u *UI) selectedSession() (model.Session, bool) {
	if u.sessionRow < 0 || u.sessionRow >= len(u.sessions.Items) {
		return model.Session{}, false
	}
	return u.sessions.Items[u.sessionRow], true
}
