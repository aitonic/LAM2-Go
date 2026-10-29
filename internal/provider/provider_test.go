package provider

import (
	"context"
	"encoding/json"
	"lam.local/lam2/internal/account"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"lam.local/lam2/internal/testutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T) (*Service, model.Profile) {
	t.Helper()
	st, e := storage.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	a := account.Service{Store: st}
	p, e := a.Create(context.Background(), "profile", "profile", 1)
	if e != nil {
		t.Fatal(e)
	}
	return &Service{Store: st, Secrets: &testutil.Secrets{}, Executable: "/Applications/LAM.app/Contents/MacOS/LAM"}, p
}
func candidate() model.Provider {
	return model.Provider{Name: "test", Protocol: "responses", BaseURL: "https://example.test/v1", AuthKind: "bearer", Models: []model.ProviderModel{{ID: "model-a"}}, DefaultModel: "model-a", Capabilities: model.UnknownCapabilities()}
}
func TestProviderCRUDAndSecretRotation(t *testing.T) {
	ctx := context.Background()
	s, _ := setup(t)
	st, _ := s.Store.Read(ctx)
	p, e := s.Save(ctx, candidate(), "sk-test-only", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(s.Store.Path())
	if strings.Contains(string(raw), "sk-test-only") {
		t.Fatal("secret persisted")
	}
	old := p.Credential
	st, _ = s.Store.Read(ctx)
	p, e = s.Save(ctx, p, "sk-new-test-only", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if p.Credential == old || p.Revision != 2 {
		t.Fatal(p)
	}
	if _, e = s.Secrets.Get(ctx, old); e == nil {
		t.Fatal("old secret retained")
	}
	if _, e = s.Save(ctx, p, "", st.Revision); fault.Code(e) != "REVISION_CONFLICT" {
		t.Fatal(e)
	}
}
func TestProviderValidation(t *testing.T) {
	for _, url := range []string{"http://example.test/v1", "https://user:password@example.test", "https://example.test?api_key=secret", "file:///a"} {
		p := candidate()
		p.BaseURL = url
		if Validate(p) == nil {
			t.Fatal(url)
		}
	}
	p := candidate()
	p.Protocol = "unknown"
	if Validate(p) == nil {
		t.Fatal("unknown protocol accepted")
	}
	p = candidate()
	p.DefaultModel = "missing"
	if Validate(p) == nil {
		t.Fatal("missing model accepted")
	}
}
func TestDiscoverModelsNoRedirectCredentialLeak(t *testing.T) {
	var leaked bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer other.Close()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, 302) }))
	defer up.Close()
	s, _ := setup(t)
	p := candidate()
	p.BaseURL = up.URL
	p.AuthKind = "none"
	if _, e := s.Discover(context.Background(), p); e == nil {
		t.Fatal("redirect accepted")
	}
	if leaked {
		t.Fatal("redirect followed")
	}
}
func TestDiscoverModels(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Error(r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"a"},{"id":"a"},{"id":"b"}]}`))
	}))
	defer up.Close()
	s, _ := setup(t)
	p := candidate()
	p.BaseURL = up.URL + "/v1"
	p.AuthKind = "none"
	m, e := s.Discover(context.Background(), p)
	if e != nil || len(m) != 2 {
		t.Fatal(m, e)
	}
}

type editor struct {
	before   []byte
	fail     bool
	observed []Edit
}

func (e *editor) Apply(ctx context.Context, p model.Profile, hash string, edits []Edit) error {
	e.observed = edits
	if e.fail {
		return fault.New("CONFIG_FAILED", "test failure")
	}
	h, _ := storage.FileHash(filepath.Join(p.Home, "config.toml"))
	if h != hash {
		return fault.New("CONFIG_CONFLICT", "changed")
	}
	return storage.AtomicWrite(ctx, filepath.Join(p.Home, "config.toml"), []byte("# preserved\nmodel = \"model-a\"\n"), 0600)
}
func TestBindAndDetachRestoreConfig(t *testing.T) {
	ctx := context.Background()
	s, p := setup(t)
	ed := &editor{}
	s.Config = ed
	path := filepath.Join(p.Home, "config.toml")
	before := []byte("# user comment\nmodel = \"original\"\n")
	os.WriteFile(path, before, 0600)
	st, _ := s.Store.Read(ctx)
	pr, e := s.Save(ctx, candidate(), "test-key", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	plan, e := s.PlanBind(ctx, p.ID, pr.ID, "model-a")
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.CommitBind(ctx, plan.ID, plan.Fingerprint)
	if e != nil {
		t.Fatal(e)
	}
	if b.RouteKind != "direct" {
		t.Fatal(b)
	}
	for _, v := range ed.observed {
		data, _ := json.Marshal(v)
		if strings.Contains(string(data), "test-key") {
			t.Fatal("secret in config")
		}
	}
	if e = s.Detach(ctx, p.ID, true); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal(string(after))
	}
}
func TestBindFailureAndStalePlanNoWrites(t *testing.T) {
	ctx := context.Background()
	s, p := setup(t)
	s.Config = &editor{fail: true}
	st, _ := s.Store.Read(ctx)
	pr, _ := s.Save(ctx, candidate(), "test-key", st.Revision)
	plan, _ := s.PlanBind(ctx, p.ID, pr.ID, "model-a")
	if _, e := s.CommitBind(ctx, plan.ID, plan.Fingerprint); e == nil {
		t.Fatal("failure hidden")
	}
	st, _ = s.Store.Read(ctx)
	if len(st.Bindings) != 0 {
		t.Fatal("half binding")
	}
	s.Config = &editor{}
	os.WriteFile(filepath.Join(p.Home, "config.toml"), []byte("external edit"), 0600)
	if _, e := s.CommitBind(ctx, plan.ID, plan.Fingerprint); fault.Code(e) != "PLAN_STALE" {
		t.Fatal(e)
	}
}

func TestSafeProviderQueryAndSensitiveRejection(t *testing.T) {
	p := model.Provider{BaseURL: "https://example.test/v1", QueryParams: map[string]string{"api-version": "2026-10-01"}}
	u, e := EndpointFor(p, "models")
	if e != nil || !strings.Contains(u, "api-version=2026-10-01") {
		t.Fatal(u, e)
	}
	p.QueryParams = map[string]string{"api_key": "do-not-leak"}
	if _, e = EndpointFor(p, "models"); e == nil {
		t.Fatal("accepted query credential")
	}
}

func TestUnsafeHeaderNamesRejected(t *testing.T) {
	for _, header := range []string{"", "Host", "Connection", "Content-Length", "Transfer-Encoding", "Proxy-Authorization", "X-Bad\tName", "X-(bad)", "\u4e2d\u6587"} {
		t.Run(header, func(t *testing.T) {
			p := model.Provider{Name: "test", Protocol: "responses", BaseURL: "https://example.test/v1", AuthKind: "none", Capabilities: model.UnknownCapabilities()}
			p.EnvHeaders = map[string]string{header: "TEST_KEY"}
			if err := Validate(p); err == nil {
				t.Fatal("unsafe header accepted")
			}
		})
	}
}

type readyGateway struct{ calls int }

func (g *readyGateway) Ensure(context.Context, model.Binding) (string, error) {
	g.calls++
	return "http://127.0.0.1:42222/v1", nil
}
func TestBindingProjectsCatalogCompactionAndInvalidatesHome(t *testing.T) {
	ctx := context.Background()
	s, p := setup(t)
	ed := &editor{}
	s.Config = ed
	var invalidated string
	s.OnChanged = func(home string) { invalidated = home }
	st, _ := s.Store.Read(ctx)
	v := candidate()
	v.Models[0].ContextWindow = 64000
	pr, e := s.Save(ctx, v, "test-key", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	plan, e := s.PlanBind(ctx, p.ID, pr.ID, "model-a")
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.CommitBind(ctx, plan.ID, plan.Fingerprint)
	if e != nil {
		t.Fatal(e)
	}
	fields := map[string]any{}
	for _, edit := range ed.observed {
		fields[edit.KeyPath] = edit.Value
	}
	if fields["model_context_window"] != int64(64000) || fields["model_auto_compact_token_limit"] != int64(57600) {
		t.Fatal(fields)
	}
	path, ok := fields["model_catalog_json"].(string)
	if !ok || path == "" {
		t.Fatal("catalog path absent")
	}
	data, e := os.ReadFile(path)
	if e != nil || !strings.Contains(string(data), `"model-a"`) {
		t.Fatal(e, string(data))
	}
	if b.CatalogPath != path || invalidated != p.Home {
		t.Fatal(b, invalidated)
	}
	if e = s.Detach(ctx, p.ID, true); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("owned catalog was not cleaned")
	}
}
func TestNativeResponsesCanExplicitlyRouteViaGateway(t *testing.T) {
	ctx := context.Background()
	s, p := setup(t)
	s.Config = &editor{}
	g := &readyGateway{}
	s.Gateway = g
	st, _ := s.Store.Read(ctx)
	v := candidate()
	v.Codex.RouteViaGateway = true
	pr, e := s.Save(ctx, v, "key", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	plan, e := s.PlanBind(ctx, p.ID, pr.ID, "model-a")
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.CommitBind(ctx, plan.ID, plan.Fingerprint)
	if e != nil || b.RouteKind != "gateway" || g.calls != 1 {
		t.Fatal(b, e)
	}
}
func TestFailedBindDoesNotLeaveCatalog(t *testing.T) {
	ctx := context.Background()
	s, p := setup(t)
	s.Config = &editor{fail: true}
	st, _ := s.Store.Read(ctx)
	pr, e := s.Save(ctx, candidate(), "key", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	plan, e := s.PlanBind(ctx, p.ID, pr.ID, "model-a")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CommitBind(ctx, plan.ID, plan.Fingerprint); e == nil {
		t.Fatal("expected config failure")
	}
	files, _ := filepath.Glob(filepath.Join(s.Store.Root, "catalogs", "*.json"))
	if len(files) != 0 {
		t.Fatal(files)
	}
}
func TestBoundProviderConnectionEditReprojectsConfig(t *testing.T) {
	ctx := context.Background()
	s, p := setup(t)
	ed := &editor{}
	s.Config = ed
	st, _ := s.Store.Read(ctx)
	pr, e := s.Save(ctx, candidate(), "key", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	plan, _ := s.PlanBind(ctx, p.ID, pr.ID, "model-a")
	if _, e = s.CommitBind(ctx, plan.ID, plan.Fingerprint); e != nil {
		t.Fatal(e)
	}
	st, _ = s.Store.Read(ctx)
	pr.BaseURL = "https://other.test/v1"
	pr.Codex.DisplayName = "New display"
	pr.Models = append(pr.Models, model.ProviderModel{ID: "second"})
	pr, e = s.Save(ctx, pr, "", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	projected := map[string]any{}
	for _, v := range ed.observed {
		projected[v.KeyPath] = v.Value
	}
	config, ok := projected["model_providers.lam_"+pr.ID].(map[string]any)
	if !ok || config["base_url"] != pr.BaseURL || config["name"] != "New display" {
		t.Fatal(projected)
	}
	st, _ = s.Store.Read(ctx)
	if st.Providers[pr.ID].BaseURL != pr.BaseURL {
		t.Fatal("metadata not committed")
	}
	pr.Models = []model.ProviderModel{{ID: "second"}}
	pr.DefaultModel = "second"
	if _, e = s.Save(ctx, pr, "", st.Revision); e == nil {
		t.Fatal("removed model still in use")
	}
}

func TestAttachedProtocolChangeMigratesRouteAndToken(t *testing.T) {
	ctx := context.Background()
	s, p := setup(t)
	s.Config = &editor{}
	s.Gateway = &readyGateway{}
	st, _ := s.Store.Read(ctx)
	pr, e := s.Save(ctx, candidate(), "key", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	plan, _ := s.PlanBind(ctx, p.ID, pr.ID, "model-a")
	if _, e = s.CommitBind(ctx, plan.ID, plan.Fingerprint); e != nil {
		t.Fatal(e)
	}
	st, _ = s.Store.Read(ctx)
	pr.Protocol = "chat_completions"
	pr, e = s.Save(ctx, pr, "", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	st, _ = s.Store.Read(ctx)
	b := st.Bindings[p.ID]
	if b.RouteKind != "gateway" || b.TokenRef.Kind != "keychain" {
		t.Fatal(b)
	}
	oldRef := b.TokenRef
	pr.Protocol = "responses"
	if _, e = s.Save(ctx, pr, "", st.Revision); e != nil {
		t.Fatal(e)
	}
	st, _ = s.Store.Read(ctx)
	if st.Bindings[p.ID].RouteKind != "direct" {
		t.Fatal("route unchanged")
	}
	if _, e = s.Secrets.Get(ctx, oldRef); e == nil {
		t.Fatal("obsolete gateway token retained")
	}
}

func TestAttachedContextUpdateReprojectsAndPreservesOriginalRestore(t *testing.T) {
	ctx := context.Background()
	s, p := setup(t)
	ed := &editor{}
	s.Config = ed
	original := []byte("# real original\nmodel=\"old\"\n")
	os.WriteFile(filepath.Join(p.Home, "config.toml"), original, 0600)
	st, _ := s.Store.Read(ctx)
	pr, e := s.Save(ctx, candidate(), "key", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	plan, _ := s.PlanBind(ctx, p.ID, pr.ID, "model-a")
	bound, e := s.CommitBind(ctx, plan.ID, plan.Fingerprint)
	if e != nil {
		t.Fatal(e)
	}
	st, _ = s.Store.Read(ctx)
	pr.Models[0].ContextWindow = 128000
	pr.ReasoningEffort = "high"
	pr, e = s.Save(ctx, pr, "", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	edits := map[string]any{}
	for _, v := range ed.observed {
		edits[v.KeyPath] = v.Value
	}
	if edits["model_context_window"] != int64(128000) || edits["model_auto_compact_token_limit"] != int64(115200) || edits["model_reasoning_effort"] != "high" {
		t.Fatal(edits)
	}
	after, _ := s.Store.Read(ctx)
	if after.Bindings[p.ID].ConfigBackup != bound.ConfigBackup {
		t.Fatal("lost original backup")
	}
	if e = s.Detach(ctx, p.ID, true); e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(filepath.Join(p.Home, "config.toml"))
	if string(got) != string(original) {
		t.Fatal(string(got))
	}
}
func TestAttachedSettingsRPCFailureDoesNotCommitProvider(t *testing.T) {
	ctx := context.Background()
	s, p := setup(t)
	ed := &editor{}
	s.Config = ed
	st, _ := s.Store.Read(ctx)
	pr, _ := s.Save(ctx, candidate(), "key", st.Revision)
	plan, _ := s.PlanBind(ctx, p.ID, pr.ID, "model-a")
	if _, e := s.CommitBind(ctx, plan.ID, plan.Fingerprint); e != nil {
		t.Fatal(e)
	}
	st, _ = s.Store.Read(ctx)
	pr.ReasoningEffort = "high"
	ed.fail = true
	if _, e := s.Save(ctx, pr, "", st.Revision); e == nil {
		t.Fatal("expected RPC failure")
	}
	after, _ := s.Store.Read(ctx)
	if after.Revision != st.Revision || after.Providers[pr.ID].ReasoningEffort != "" {
		t.Fatal("partial provider state")
	}
}

func TestCodexProfileCredentialResolver(t *testing.T) {
	s, _ := setup(t)
	called := ""
	s.ProfileToken = func(ctx context.Context, id string) (string, error) { called = id; return "profile-api-key", nil }
	key, e := s.Credential(context.Background(), model.CredentialRef{Kind: "codex_profile", ProfileID: "profile-a"})
	if e != nil || key != "profile-api-key" || called != "profile-a" {
		t.Fatal(key, e, called)
	}
	if _, e = s.Credential(context.Background(), model.CredentialRef{Kind: "auth_command", ApprovalID: "missing"}); fault.Code(e) != "AUTH_COMMAND_UNAPPROVED" {
		t.Fatal(e)
	}
}

// List/delete and auth-command approval are not implied by a provider Save test.
func TestProviderListAndDeleteRequireConfirmation(t *testing.T) {
	s, _ := setup(t)
	ctx := context.Background()
	st, err := s.Store.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first := candidate()
	first.Name = "Zoo"
	created, err := s.Save(ctx, first, "secret-one", st.Revision)
	if err != nil {
		t.Fatal(err)
	}
	st, _ = s.Store.Read(ctx)
	second := candidate()
	second.Name = "Alpha"
	other, err := s.Save(ctx, second, "secret-two", st.Revision)
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.List(ctx)
	if err != nil || len(list) != 2 || list[0].ID != other.ID || list[1].ID != created.ID {
		t.Fatalf("list: %+v %v", list, err)
	}
	st, _ = s.Store.Read(ctx)
	if e := s.Delete(ctx, created.ID, st.Revision, false); fault.Code(e) != "CONFIRMATION_REQUIRED" {
		t.Fatal("delete without confirmation:", e)
	}
	if e := s.Delete(ctx, created.ID, st.Revision, true); e != nil {
		t.Fatal(e)
	}
	list, err = s.List(ctx)
	if err != nil || len(list) != 1 || list[0].ID != other.ID {
		t.Fatalf("deleted wrong provider: %+v %v", list, err)
	}
}

func TestApproveAuthCommandRequiresExecutableAndExplicitConsent(t *testing.T) {
	s, _ := setup(t)
	ctx := context.Background()
	st, err := s.Store.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "auth-helper")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, e := s.ApproveCommand(ctx, script, []string{"--token"}, st.Revision, false); fault.Code(e) != "CONFIRMATION_REQUIRED" {
		t.Fatal(e)
	}
	if _, e := s.ApproveCommand(ctx, "auth-helper", nil, st.Revision, true); fault.Code(e) != "AUTH_COMMAND_INVALID" {
		t.Fatal(e)
	}
	approved, err := s.ApproveCommand(ctx, script, []string{"--token"}, st.Revision, true)
	if err != nil || approved.Executable != script || approved.SHA256 == "" {
		t.Fatalf("approval: %+v %v", approved, err)
	}
	st, err = s.Store.Read(ctx)
	if err != nil || len(st.Approvals) != 1 || st.Approvals[approved.ID].SHA256 != approved.SHA256 {
		t.Fatalf("approval not stored: %+v %v", st.Approvals, err)
	}
}
