package migration

import (
	"context"
	"encoding/json"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, n string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join("..", "..", "tests", "legacy-fixtures", "legacy-provider-contract", "provider-store", n))
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestLegacyFixtures(t *testing.T) {
	for _, n := range []string{"legacy-wire-openai.json", "legacy-wire-responses.json", "empty.json"} {
		p, _, e := DecodeProviders(fixture(t, n))
		if e != nil {
			t.Fatal(n, e)
		}
		if n != "empty.json" && (len(p) != 1 || p[0].Credential.Kind != "env" || p[0].DefaultModel == "") {
			t.Fatal(p)
		}
	}
	for _, n := range []string{"future-version.json", "malformed.json"} {
		if _, _, e := DecodeProviders(fixture(t, n)); e == nil {
			t.Fatal("accepted", n)
		}
	}
}
func TestPreviewDoesNotWriteLegacyAndCommitIsIdempotent(t *testing.T) {
	ctx := context.Background()
	old := t.TempDir()
	os.MkdirAll(filepath.Join(old, "provider-hub"), 0700)
	src := filepath.Join(old, "provider-hub", "providers.json")
	os.WriteFile(src, fixture(t, "legacy-wire-responses.json"), 0600)
	before, _ := storage.FileHash(src)
	store, e := storage.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	m := Service{Store: store}
	plan, e := m.Plan(ctx, old)
	if e != nil {
		t.Fatal(e)
	}
	if len(plan.Providers) != 1 {
		t.Fatal(plan)
	}
	if e = m.Apply(ctx, plan.ID, plan.Fingerprint); e != nil {
		t.Fatal(e)
	}
	if e = m.Apply(ctx, plan.ID, plan.Fingerprint); e != nil {
		t.Fatal("repeat should be idempotent", e)
	}
	after, _ := storage.FileHash(src)
	if before != after {
		t.Fatal("modified source")
	}
	st, _ := store.Read(ctx)
	if len(st.Providers) != 1 {
		t.Fatal(st)
	}
	if e = m.Rollback(ctx, plan.ID, true); e != nil {
		t.Fatal(e)
	}
	st, _ = store.Read(ctx)
	if len(st.Providers) != 0 {
		t.Fatal("rollback failed")
	}
}
func TestSourceChangeAndFutureSchema(t *testing.T) {
	ctx := context.Background()
	old := t.TempDir()
	os.MkdirAll(filepath.Join(old, "provider-hub"), 0700)
	src := filepath.Join(old, "provider-hub", "providers.json")
	os.WriteFile(src, fixture(t, "legacy-wire-openai.json"), 0600)
	store, _ := storage.Open(t.TempDir())
	m := Service{Store: store}
	plan, e := m.Plan(ctx, old)
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(src, []byte("[]"), 0600)
	if e = m.Apply(ctx, plan.ID, plan.Fingerprint); e == nil {
		t.Fatal("changed source accepted")
	}
}
func TestV2PreservesCredentialReferences(t *testing.T) {
	raw := []byte(`{"schemaVersion":1,"revision":7,"providers":[{"id":"p1","name":"P","protocol":"responses","baseUrl":"https://example.test/v1","defaultModel":"m","models":[{"id":"m","label":"Model"}],"upstreamAuth":{"kind":"bearer","source":{"kind":"keychain","service":"lam.remote-provider","account":"p1/version-2","version":2}},"capabilities":{"streaming":"supported","functionTools":"supported","structuredOutputs":"unknown","reasoning":"partial"},"codex":{"envHttpHeaders":{"X-Team":"TEAM_ID"},"streamIdleTimeoutMs":300000}}]}`)
	p, _, e := DecodeProviders(raw)
	if e != nil {
		t.Fatal(e)
	}
	if p[0].Credential.Account != "p1/version-2" || p[0].Credential.Version != 2 || p[0].EnvHeaders["X-Team"] != "TEAM_ID" {
		b, _ := json.Marshal(p)
		t.Fatal(string(b))
	}
}

type fakeHistoryImporter struct {
	staged, published, rolled bool
	publishErr                error
}

func (f *fakeHistoryImporter) Stage(context.Context, string, string) error {
	f.staged = true
	return nil
}
func (f *fakeHistoryImporter) Publish(context.Context, string) error {
	if f.publishErr != nil {
		return f.publishErr
	}
	f.published = true
	return nil
}
func (f *fakeHistoryImporter) RollbackImport(context.Context, string) error {
	f.rolled = true
	return nil
}
func TestLegacyDatabasePublicationResumesAfterInterruptedPublish(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "usage"), 0700)
	os.WriteFile(filepath.Join(root, "usage", "usage.sqlite3"), []byte("test importer boundary"), 0600)
	st, _ := storage.Open(t.TempDir())
	history := &fakeHistoryImporter{publishErr: os.ErrPermission}
	m := Service{Store: st, Usage: history}
	p, e := m.Plan(ctx, root)
	if e != nil {
		t.Fatal(e)
	}
	if e = m.Apply(ctx, p.ID, p.Fingerprint); e == nil {
		t.Fatal("publication failure masked")
	}
	if !history.staged || history.published {
		t.Fatal(history)
	}
	history.publishErr = nil
	if e = m.Apply(ctx, p.ID, p.Fingerprint); e != nil || !history.published {
		t.Fatal("cannot resume prepared import", e)
	}
	if e = m.Rollback(ctx, p.ID, true); e != nil || !history.rolled {
		t.Fatal("usage rollback omitted", e)
	}
}

func TestSettingsNotesProxyTimelineAndExpiryAreNotLost(t *testing.T) {
	ctx := context.Background()
	old := t.TempDir()
	home := t.TempDir()
	write := func(rel string, v any) {
		t.Helper()
		p := filepath.Join(old, rel)
		os.MkdirAll(filepath.Dir(p), 0700)
		b, _ := json.Marshal(v)
		if e := os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	write("config/accounts-cache.json", map[string]any{"accounts": []any{map[string]any{"id": "office", "codexHome": home, "displayName": "Office"}}})
	write("config/settings.json", map[string]any{"hideDockIcon": true, "terminalTargetId": "cmux", "antigravityPort": 10001, "gatewayFirstResponseTimeoutSeconds": 80, "gatewayRequestTimeoutSeconds": 1500, "codexLaunchPermissionPreset": "fullAccess", "authMode": "pat"})
	write("config/account-notes.json", map[string]any{"accounts": map[string]any{"office": map[string]any{"note": "retain note", "renewalDate": "2026-12-30"}}})
	write("config/account-proxies/office.json", map[string]any{"profileId": "office", "enabled": true, "url": "socks5h://127.0.0.1:1080"})
	write("config/auth-metadata/office.json", map[string]any{"tokenExpiration": "2027-01-01T00:00:00Z"})
	write("config/pat-usage-timeline.json", map[string]any{"events": []any{map[string]any{"accountId": "office", "accountLabel": "Office", "workspaceId": "workspace:main", "startedAt": "2026-01-01T00:00:00Z", "endedAt": "2026-02-01T00:00:00Z"}}})
	store, _ := storage.Open(t.TempDir())
	m := Service{Store: store}
	p, e := m.Plan(ctx, old)
	if e != nil {
		t.Fatal(e)
	}
	if e = m.Apply(ctx, p.ID, p.Fingerprint); e != nil {
		t.Fatal(e)
	}
	st, _ := store.Read(ctx)
	pr := st.Profiles["office"]
	if pr.Note != "retain note" || pr.Proxy.All == "" || pr.TokenExpiration == nil || st.Settings.ShowDock || st.Settings.Terminal != "cmux" || st.Settings.Gateway.FirstResponseSeconds != 80 || st.Settings.AntigravityPort != 10001 || len(st.PATTimeline) != 1 {
		t.Fatalf("metadata omitted: %+v %+v", pr, st.Settings)
	}
	if st.Settings.ApprovalPolicy != "never" || st.Settings.SandboxMode != "danger-full-access" {
		t.Fatal(st.Settings)
	}
	if e = m.Rollback(ctx, p.ID, true); e != nil {
		t.Fatal(e)
	}
	st, _ = store.Read(ctx)
	if len(st.PATTimeline) != 0 || !st.Settings.ShowDock {
		t.Fatal("metadata rollback lost", st)
	}
}

func TestBindingMappingsSurviveImportWithoutRunningLegacyHelpers(t *testing.T) {
	ctx := context.Background()
	old := t.TempDir()
	home := t.TempDir()
	os.MkdirAll(filepath.Join(old, "provider-hub"), 0700)
	os.MkdirAll(filepath.Join(old, "config"), 0700)
	os.WriteFile(filepath.Join(old, "provider-hub", "providers.json"), fixture(t, "legacy-wire-responses.json"), 0600)
	cache, _ := json.Marshal(map[string]any{"accounts": []any{map[string]any{"id": "a", "displayName": "A", "codexHome": home}}})
	os.WriteFile(filepath.Join(old, "config", "accounts-cache.json"), cache, 0600)
	os.WriteFile(filepath.Join(old, "provider-hub", "bindings.json"), []byte(`{"schemaVersion":1,"bindings":[{"profileId":"a","providerId":"legacy-responses-service","selectedModel":"example-response-model","routeKind":"direct","configProjection":{"appliedHash":"old-hash"}}]}`), 0600)
	st, _ := storage.Open(t.TempDir())
	m := Service{Store: st}
	p, e := m.Plan(ctx, old)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.PendingBindings) != 1 || p.PendingBindings[0].Model != "example-response-model" {
		t.Fatal(p)
	}
	if e = m.Apply(ctx, p.ID, p.Fingerprint); e != nil {
		t.Fatal(e)
	}
	state, _ := st.Read(ctx)
	if len(state.PendingBindings) != 1 || len(state.Bindings) != 0 {
		t.Fatal(state)
	}
	if _, e = os.Stat(filepath.Join(home, "config.toml")); !os.IsNotExist(e) {
		t.Fatal("migration changed external config")
	}
}

func TestLegacyRelayProfileProvenanceIsPreserved(t *testing.T) {
	ctx := context.Background()
	old := t.TempDir()
	home := t.TempDir()
	os.MkdirAll(filepath.Join(old, "config"), 0700)
	raw, _ := json.Marshal(map[string]any{"accounts": []any{map[string]any{"id": "b-relay-a", "displayName": "Relay", "codexHome": home, "isRelay": true, "relayIdentity": "b", "relaySource": "a"}}})
	os.WriteFile(filepath.Join(old, "config", "accounts-cache.json"), raw, 0600)
	store, _ := storage.Open(t.TempDir())
	svc := Service{Store: store}
	plan, e := svc.Plan(ctx, old)
	if e != nil {
		t.Fatal(e)
	}
	if e = svc.Apply(ctx, plan.ID, plan.Fingerprint); e != nil {
		t.Fatal(e)
	}
	st, _ := store.Read(ctx)
	p := st.Profiles["b-relay-a"]
	if p.Relay == nil || p.Relay.RuntimeProfileID != "b" || p.Relay.SourceProfileID != "a" {
		t.Fatal("relay provenance lost", p)
	}
	if p.Managed || p.Home != home {
		t.Fatal("legacy directory adopted or relocated", p)
	}
}
