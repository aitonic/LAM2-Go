// Package app wires production adapters to LAM use cases. It owns no GUI,
// protocol transport, database engine, or generic event framework.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"lam.local/lam2/internal/account"
	"lam.local/lam2/internal/codex"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/gateway"
	"lam.local/lam2/internal/handoff"
	"lam.local/lam2/internal/integrations/agentparser"
	"lam.local/lam2/internal/integrations/codexsdk"
	"lam.local/lam2/internal/integrations/httpclient"
	"lam.local/lam2/internal/integrations/mygosecrets"
	"lam.local/lam2/internal/integrations/mygosqlite"
	"lam.local/lam2/internal/integrations/translator"
	"lam.local/lam2/internal/migration"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/provider"
	"lam.local/lam2/internal/quota"
	run "lam.local/lam2/internal/runtime"
	"lam.local/lam2/internal/session"
	"lam.local/lam2/internal/settings"
	"lam.local/lam2/internal/storage"
	"lam.local/lam2/internal/usage"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Core struct {
	Store            *storage.Store
	Secrets          storage.Secrets
	Vault            *storage.Vault
	Accounts         *account.Service
	Codex            *codex.Pool
	Sessions         *session.Service
	Handoff          *handoff.Service
	Providers        *provider.Service
	APIAccounts      *provider.APIAccountService
	AntigravityCache *quota.AntigravityCache
	Quota            *quota.Service
	Settings         settings.Service
	Migration        migration.Service
	Gateway          *gateway.Supervisor
	Executable       string
	dbMu             sync.Mutex
	dataMu           sync.RWMutex
	database         *mygosqlite.DB
}

// Open deliberately does not open a database, start an App Server, create a
// window, or acquire a desktop-instance lock. Auth and gateway remain headless.
func Open(home string, secrets storage.Secrets) (*Core, error) {
	root, e := storage.ResolveHome(home)
	if e != nil {
		return nil, e
	}
	store, e := storage.Open(root)
	if e != nil {
		return nil, e
	}
	exe, e := os.Executable()
	if e != nil {
		return nil, e
	}
	exe, e = filepath.EvalSymlinks(exe)
	if e != nil {
		return nil, e
	}
	if secrets == nil {
		secrets = mygosecrets.Secrets{}
	}
	c := &Core{Store: store, Secrets: secrets, Executable: exe}
	c.Vault = &storage.Vault{Root: filepath.Join(root, "vault"), Secrets: secrets}
	userHome, e := os.UserHomeDir()
	if e != nil {
		return nil, e
	}
	c.Codex = codex.NewPool(8, 10*time.Minute, codexsdk.Factory(func() string {
		st, e := store.Read(context.Background())
		if e != nil {
			return "codex"
		}
		return st.Settings.CodexPath
	}, c.ProfileEnvironment))
	c.Accounts = &account.Service{Store: store, Vault: c.Vault, SharedHome: filepath.Join(userHome, ".codex"), OnAuthChanged: c.Codex.InvalidateHome}
	c.Sessions = &session.Service{Store: store, SharedHome: c.Accounts.SharedHome}
	c.Handoff = &handoff.Service{Store: store, Sessions: c.Sessions}
	c.Gateway = &gateway.Supervisor{Root: root, Executable: exe}
	c.Providers = &provider.Service{Store: store, Secrets: secrets, Config: &codex.ConfigEditor{Caller: c.Codex}, Gateway: c.Gateway, Executable: exe, OnChanged: c.Codex.InvalidateHome}
	c.Providers.BeforePortChange = c.Gateway.StopAndWait
	c.Providers.ProfileToken = c.Accounts.ProviderAPIKey
	c.APIAccounts = &provider.APIAccountService{Providers: c.Providers, Accounts: c.Accounts}
	c.Quota = quota.New(store, c.Codex)
	c.Quota.LookupProfile = c.Accounts.RuntimeProfile
	c.Quota.PrepareProfile = c.Accounts.QuotaProfile
	c.AntigravityCache = quota.NewAntigravityCache()
	c.Settings = settings.Service{Store: store}
	c.Migration = migration.Service{Store: store, Usage: &usageImport{core: c}}
	return c, nil
}
func (c *Core) Close() error {
	c.dbMu.Lock()
	defer c.dbMu.Unlock()
	var errs []error
	if c.database != nil {
		errs = append(errs, c.database.Close())
		c.database = nil
	}
	if c.Codex != nil {
		errs = append(errs, c.Codex.Close())
	}
	return errors.Join(errs...)
}
func (c *Core) DB(ctx context.Context) (*mygosqlite.DB, error) {
	c.dbMu.Lock()
	defer c.dbMu.Unlock()
	if c.database != nil {
		return c.database, nil
	}
	d, e := mygosqlite.Open(ctx, filepath.Join(c.Store.Root, "usage", "usage.sqlite3"))
	if e != nil {
		return nil, e
	}
	c.database = d
	return d, nil
}
func (c *Core) Recovery(ctx context.Context) ([]string, error) {
	first, e := c.Store.Recover(ctx)
	if e != nil {
		return first, e
	}
	rest, e := c.Providers.Recover(ctx)
	first = append(first, rest...)
	if e != nil {
		return first, e
	}
	archives, e := c.Sessions.RecoverArchives(ctx)
	return append(first, archives...), e
}

// ProfileEnvironment only restores explicitly selected provider variables.
// Generic SDK spawn and terminal Env append to the process environment, so an
// empty value must be emitted for credentials owned by every other provider.
func (c *Core) ProfileEnvironment(ctx context.Context, p model.Profile) (map[string]string, error) {
	st, e := c.Store.Read(ctx)
	if e != nil {
		return nil, e
	}
	if (p.AuthMode == "pat_runtime" || (p.AuthMode == "pat" && p.Home == c.Accounts.SharedHome)) && st.ActivePAT != p.ID {
		return nil, fault.New("PAT_INACTIVE", "Activate this PAT before using its shared Codex home")
	}
	out := map[string]string{"OPENAI_API_KEY": "", "CODEX_API_KEY": "", "ANTHROPIC_API_KEY": "", "LAM_PROVIDER_HEADER_TOKEN": ""}
	for _, v := range st.Providers {
		if v.Credential.Kind == "env" {
			out[v.Credential.EnvKey] = ""
		}
		for _, key := range v.EnvHeaders {
			out[key] = ""
		}
	}
	if p.AuthMode == "quota" {
		return out, nil
	}
	binding, ok := st.Bindings[p.ID]
	if !ok {
		return out, nil
	}
	v, ok := st.Providers[binding.ProviderID]
	if !ok {
		return nil, fault.New("PROVIDER_NOT_FOUND", "Bound provider is missing")
	}
	if v.Credential.Kind == "env" {
		value, e := c.Providers.Credential(ctx, v.Credential)
		if e != nil {
			return nil, e
		}
		out[v.Credential.EnvKey] = value
	}
	for _, key := range v.EnvHeaders {
		value, ok := os.LookupEnv(key)
		if !ok {
			return nil, fault.New("AUTH_MISSING", "A configured header environment variable is missing")
		}
		out[key] = value
	}
	if binding.RouteKind == "direct" && v.AuthKind == "api_key" {
		key, e := c.Providers.Credential(ctx, v.Credential)
		if e != nil {
			return nil, e
		}
		out["LAM_PROVIDER_HEADER_TOKEN"] = key
	}
	return out, nil
}
func (c *Core) Profile(ctx context.Context, id string) (model.Profile, error) {
	return c.Accounts.RuntimeProfile(ctx, id)
}
func (c *Core) CreateProfile(ctx context.Context, name, mode string, revision uint64, copyConfigFrom ...string) (model.Profile, error) {
	p, e := c.Accounts.Create(ctx, name, mode, revision, copyConfigFrom...)
	if e != nil {
		return p, e
	}
	// A wrapper failure is reported without deleting the successfully created
	// profile. RepairWrapper is idempotent and available from CLI/UI.
	if e = c.RepairWrapper(ctx, p.ID); e != nil {
		return p, e
	}
	return p, nil
}
func (c *Core) CreateRelay(ctx context.Context, id, fingerprint string) (model.Profile, error) {
	p, e := c.Accounts.CreateRelay(ctx, id, fingerprint)
	if e != nil {
		return p, e
	}
	return p, c.RepairWrapper(ctx, p.ID)
}
func (c *Core) RepairWrapper(ctx context.Context, id string) error {
	if _, e := c.Accounts.Get(ctx, id); e != nil {
		return e
	}
	dir := filepath.Join(c.Store.Root, "bin")
	if e := storage.EnsurePrivateDir(dir); e != nil {
		return e
	}
	return run.WriteWrapper(ctx, filepath.Join(dir, "lam-"+id), c.Executable, c.Store.Root, id)
}
func (c *Core) Login(ctx context.Context, id, kind, key string) (json.RawMessage, error) {
	p, e := c.Profile(ctx, id)
	if e != nil {
		return nil, e
	}
	if kind != "chatgpt" && kind != "apiKey" && kind != "chatgptDeviceCode" {
		return nil, fault.New("LOGIN_KIND_INVALID", "Choose ChatGPT, device code or API key login")
	}
	params := map[string]any{"type": kind}
	if kind == "apiKey" {
		if key == "" {
			return nil, fault.New("AUTH_MISSING", "API key is required")
		}
		params["apiKey"] = key
	}
	var out json.RawMessage
	e = c.Codex.Call(ctx, p, "account/login/start", params, &out)
	return out, e
}
func (c *Core) AccountRead(ctx context.Context, id string) (json.RawMessage, error) {
	p, e := c.Profile(ctx, id)
	if e != nil {
		return nil, e
	}
	var out json.RawMessage
	e = c.Codex.Call(ctx, p, "account/read", map[string]any{"refreshToken": false}, &out)
	return out, e
}
func (c *Core) Threads(ctx context.Context, id, cursor string) (json.RawMessage, error) {
	p, e := c.Profile(ctx, id)
	if e != nil {
		return nil, e
	}
	var out json.RawMessage
	params := map[string]any{"limit": 50}
	if cursor != "" {
		params["cursor"] = cursor
	}
	e = c.Codex.Call(ctx, p, "thread/list", params, &out)
	return out, e
}
func (c *Core) LaunchPlan(ctx context.Context, id, resume, cwd string, extra []string) (run.LaunchPlan, error) {
	p, e := c.Profile(ctx, id)
	if e != nil {
		return run.LaunchPlan{}, e
	}
	st, e := c.Store.Read(ctx)
	if e != nil {
		return run.LaunchPlan{}, e
	}
	if resume != "" {
		if _, e = c.Sessions.Resolve(ctx, id, resume); e != nil {
			return run.LaunchPlan{}, e
		}
	}
	if b, ok := st.Bindings[id]; ok && b.RouteKind == "gateway" {
		if _, e = c.Gateway.Ensure(ctx, b); e != nil {
			return run.LaunchPlan{}, e
		}
	}
	env, e := c.ProfileEnvironment(ctx, p)
	if e != nil {
		return run.LaunchPlan{}, e
	}
	return run.Plan(p, st.Settings, run.LaunchOptions{Resume: resume, CWD: cwd, Extra: extra, Environment: env})
}
func (c *Core) ExternalTerminal(ctx context.Context, id, resume, cwd string, extra ...string) error {
	if _, e := c.LaunchPlan(ctx, id, resume, cwd, extra); e != nil {
		return e
	}
	st, e := c.Store.Read(ctx)
	if e != nil {
		return e
	}
	return run.OpenTerminal(ctx, st.Settings.Terminal, c.Executable, c.Store.Root, id, resume, cwd, extra...)
}
func (c *Core) RunGateway(ctx context.Context, address string) error {
	st, e := c.Store.Read(ctx)
	if e != nil {
		return e
	}
	tr, e := translator.New()
	if e != nil {
		return e
	}
	srv := gateway.NewServer(st.Settings.Gateway, &gateway.StateResolver{Store: c.Store, Secrets: c.Secrets, Providers: c.Providers}, tr)
	pool := httpclient.New()
	defer pool.Close()
	srv.ClientFor = func(s gateway.Snapshot) (*http.Client, error) { return pool.Client(s.Proxy) }
	defer srv.HTTP.CloseIdleConnections()
	return gateway.RunDaemon(ctx, c.Store.Root, c.Executable, address, srv)
}

type Basic struct {
	State     model.State
	Profiles  []model.Profile
	Providers []model.Provider
	Quotas    map[string]model.QuotaSnapshot
	Recovery  []string
}

func (c *Core) BasicSnapshot(ctx context.Context) (Basic, error) {
	var v Basic
	st, e := c.Store.Read(ctx)
	if e != nil {
		return v, e
	}
	v.State = st
	if v.State.ActivePAT == "" && c.Accounts != nil {
		if detected, _ := c.Accounts.DetectActivePAT(ctx); detected != "" {
			v.State.ActivePAT = detected
		}
	}
	v.Quotas = map[string]model.QuotaSnapshot{}
	// Match original list_accounts: existing ~/.codex* accounts with real
	// Codex signals are visible immediately. Discovery remains read-only;
	// write operations require registration and cannot claim external homes.
	v.Profiles, e = c.Accounts.ListVisible(ctx, filepath.Dir(c.Accounts.SharedHome))
	if e != nil {
		return v, e
	}
	for _, p := range v.Profiles {
		q, e := c.Quota.Read(ctx, p.ID)
		if e == nil {
			v.Quotas[p.ID] = q
		}
	}
	sort.Slice(v.Profiles, func(i, j int) bool {
		if v.Profiles[i].Name == v.Profiles[j].Name {
			return v.Profiles[i].ID < v.Profiles[j].ID
		}
		return v.Profiles[i].Name < v.Profiles[j].Name
	})
	for _, p := range st.Providers {
		v.Providers = append(v.Providers, p)
	}
	sort.Slice(v.Providers, func(i, j int) bool { return v.Providers[i].Name < v.Providers[j].Name })
	return v, nil
}
func (c *Core) RefreshQuotas(ctx context.Context, ids []string) (map[string]model.QuotaSnapshot, error) {
	if len(ids) == 0 {
		ps, e := c.Accounts.ListVisible(ctx, filepath.Dir(c.Accounts.SharedHome))
		if e != nil {
			return nil, e
		}
		for _, p := range ps {
			ids = append(ids, p.ID)
		}
	}
	out := map[string]model.QuotaSnapshot{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, id := range ids {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return out, ctx.Err()
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()
			q, _ := c.Quota.Refresh(ctx, id)
			mu.Lock()
			out[id] = q
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	return out, ctx.Err()
}
func (c *Core) Antigravity(ctx context.Context) (model.QuotaSnapshot, error) {
	st, e := c.Store.Read(ctx)
	if e != nil {
		return model.QuotaSnapshot{}, e
	}
	return (quota.Antigravity{URL: st.Settings.AntigravityURL, Port: st.Settings.AntigravityPort, CSRF: st.Settings.AntigravityCSRF, Secrets: c.Secrets, Cache: c.AntigravityCache}).Read(ctx)
}

type IndexProgress struct {
	Files, Total, Events, Diagnostics int
	Path                              string
	Errors                            []string
}

func (c *Core) RefreshUsage(ctx context.Context, profileID string, progress func(IndexProgress)) (IndexProgress, error) {
	c.dataMu.Lock()
	defer c.dataMu.Unlock()
	d, e := c.DB(ctx)
	if e != nil {
		return IndexProgress{}, e
	}
	st, e := c.Store.Read(ctx)
	if e != nil {
		return IndexProgress{}, e
	}
	lineage, e := c.Handoff.Provenance(ctx)
	if e != nil {
		return IndexProgress{}, e
	}
	lockRoot := filepath.Join(c.Store.Root, "usage", "locks")
	if e = storage.EnsurePrivateDir(lockRoot); e != nil {
		return IndexProgress{}, e
	}
	ix := usage.Indexer{Repo: d, Parser: agentparser.New, LockRoot: lockRoot, PATTimeline: st.PATTimeline}
	type indexJob struct {
		Profile model.Profile
		Session model.Session
	}
	var all []indexJob
	seenHomes := map[string]bool{}
	profiles := make([]model.Profile, 0, len(st.Profiles)+1)
	for _, p := range st.Profiles {
		if p.Home != c.Accounts.SharedHome {
			profiles = append(profiles, p)
		}
	}
	// Shared sessions remain a distinct source; event-time attribution handles
	// every PAT switch instead of charging all history to the current identity.
	shared := model.Profile{ID: "shared-codex", Home: c.Accounts.SharedHome, AuthMode: "profile"}
	for _, p := range st.Profiles {
		if p.Home == c.Accounts.SharedHome && p.AuthMode != "pat" {
			shared = p
			break
		}
	}
	if len(st.PATTimeline) > 0 {
		shared.AuthMode = "pat_shared"
	}
	if _, err := os.Stat(shared.Home); err == nil {
		profiles = append(profiles, shared)
	}
	for _, p := range profiles {
		if profileID != "" && p.ID != profileID && p.Home != c.Accounts.SharedHome {
			continue
		}
		if seenHomes[p.Home] {
			continue
		}
		seenHomes[p.Home] = true
		ss, err := c.Sessions.ListUsageSources(ctx, p)
		if err != nil {
			return IndexProgress{}, err
		}
		for _, item := range ss {
			all = append(all, indexJob{Profile: p, Session: item})
		}
	}
	state := IndexProgress{Total: len(all)}
	for _, job := range all {
		s := job.Session
		if e = ctx.Err(); e != nil {
			return state, e
		}
		r, err := ix.Index(ctx, job.Profile, s, st.Prices, lineage)
		state.Files++
		state.Events += r.Events
		state.Diagnostics += r.Diagnostics
		state.Path = s.Path
		if err != nil {
			state.Errors = append(state.Errors, fault.Code(err)+":"+s.ProfileID+":"+s.ID)
		}
		if progress != nil {
			progress(state)
		}
	}
	if len(state.Errors) > 0 {
		_ = d.RecordRefresh(context.Background(), "USAGE_PARTIAL")
		return state, fault.New("USAGE_PARTIAL", "Some session files could not be indexed; see diagnostic error codes")
	}
	if e = d.RecordRefresh(ctx, ""); e != nil {
		return state, e
	}
	return state, nil
}

type UsageView struct {
	Diagnostics      usage.Diagnostics
	LegacyAccounts   []mygosqlite.LegacyAccountSnapshot
	Insights         usage.Insights
	Errors           map[string]string
	Account          *usage.AccountUsage
	AccountFetchedAt time.Time
	AccountDelta     usage.AccountDelta
	ThreadTotal      int
	Activity         usage.Activity
	Summary          usage.Summary
	Calls            model.Page[model.UsageEvent]
	Threads          []mygosqlite.Thread
	Daily            []usage.Daily
}

func (c *Core) ReadUsage(ctx context.Context, f usage.Filter, loc *time.Location) (UsageView, error) {
	if loc == nil {
		loc = time.Local
	}
	c.dataMu.RLock()
	defer c.dataMu.RUnlock()
	var v UsageView
	d, e := c.DB(ctx)
	if e != nil {
		return v, e
	}
	v.Errors = map[string]string{}
	sectionError := func(section string, err error) {
		if err != nil {
			v.Errors[section] = fault.Code(err)
		}
	}
	v.Summary, e = d.Summary(ctx, f)
	sectionError("summary", e)
	v.Calls, e = d.Calls(ctx, f)
	sectionError("calls", e)
	threadPage, e := d.ThreadPage(ctx, f)
	sectionError("threads", e)
	if e == nil {
		v.Threads, v.ThreadTotal = threadPage.Items, threadPage.Total
	}
	if f.After.IsZero() {
		f.After = usage.DefaultActivityStart(time.Now())
	}
	if f.Before.IsZero() {
		f.Before = time.Now().AddDate(0, 0, 1)
	}
	v.Daily, e = d.Daily(ctx, f, loc)
	sectionError("activity", e)
	v.Activity = usage.AnalyzeActivity(v.Daily, time.Now().In(loc))
	v.Insights, e = d.Insights(ctx, f)
	sectionError("insights", e)
	v.Diagnostics, e = d.Diagnostics(ctx, f)
	sectionError("diagnostics", e)
	v.LegacyAccounts, e = d.LegacyAccountSnapshots(ctx)
	sectionError("legacyAccountUsage", e)
	if f.ProfileID != "" {
		raw, at, found, err := d.AccountUsageSnapshot(ctx, f.ProfileID)
		sectionError("accountUsage", err)
		if err == nil && found {
			parsed, err := usage.ParseAccountUsage(raw)
			sectionError("accountUsage", err)
			if err == nil {
				v.Account = &parsed
				v.AccountFetchedAt = at
				if f.Workspace == "" && f.Model == "" && f.ThreadKey == "" && f.ArchiveScope == "" {
					v.AccountDelta = usage.CompareAccountDays(parsed, v.Daily, f.After, f.Before, loc)
				} else {
					v.AccountDelta.Reason = "Account-wide usage cannot be compared to a model/workspace/thread/archive subset"
				}
			}
		}
	}
	// A bad section must not discard the good sections or turn the page blank.
	return v, ctx.Err()
}

func (c *Core) AccountUsage(ctx context.Context, id string) (json.RawMessage, error) {
	raw, e := c.Quota.AccountUsage(ctx, id)
	if e != nil {
		return nil, e
	}
	if _, e = usage.ParseAccountUsage(raw); e != nil {
		return nil, e
	}
	d, e := c.DB(ctx)
	if e != nil {
		return nil, e
	}
	if e = d.SaveAccountUsage(ctx, id, raw); e != nil {
		return nil, e
	}
	return raw, nil
}
func (c *Core) MaintainUsage(ctx context.Context, kind string, confirmed bool) error {
	if kind == "reset" && !confirmed {
		return fault.New("CONFIRMATION_REQUIRED", "Reset affects only the rebuildable usage index")
	}
	c.dataMu.Lock()
	defer c.dataMu.Unlock()
	d, e := c.DB(ctx)
	if e != nil {
		return e
	}
	switch kind {
	case "reset":
		return d.Reset(ctx)
	case "compact":
		return d.Compact(ctx)
	case "reprice":
		st, e := c.Store.Read(ctx)
		if e != nil {
			return e
		}
		return d.Reprice(ctx, st.Prices)
	default:
		return fault.New("ACTION_INVALID", "Unknown index maintenance operation")
	}
}
func (c *Core) Doctor(ctx context.Context) map[string]any {
	st, e := c.Store.Read(ctx)
	out := map[string]any{"home": c.Store.Root, "executable": c.Executable, "mygo": "v0.7.1", "codexSDK": "v0.163.0", "requiredGo": "1.27.3", "nativeVerification": "requires macOS acceptance"}
	if e != nil {
		out["storeError"] = fault.Public(e)
		return out
	}
	out["revision"] = st.Revision
	out["profiles"] = len(st.Profiles)
	out["providers"] = len(st.Providers)
	out["codexPath"] = st.Settings.CodexPath
	_, h, e := c.Gateway.Probe(ctx)
	if e == nil {
		out["gateway"] = h
	} else {
		out["gatewayError"] = fault.Code(e)
	}
	return out
}
func Pretty(v any) string {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return fmt.Sprintf("%T", v)
	}
	return string(b)
}
