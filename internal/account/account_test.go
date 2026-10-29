package account

import (
	"context"
	"encoding/json"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"lam.local/lam2/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Service, *storage.Store) {
	t.Helper()
	st, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v := &storage.Vault{Root: st.Root, Secrets: &testutil.Secrets{}}
	return &Service{Store: st, Vault: v, SharedHome: filepath.Join(t.TempDir(), ".codex")}, st
}
func TestProfilesAreIsolated(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	a, err := s.Create(ctx, "Office", "profile", 1)
	if err != nil {
		t.Fatal(err)
	}
	state, _ := st.Read(ctx)
	b, err := s.Create(ctx, "Home", "profile", state.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if a.Home == b.Home || a.ID == b.ID {
		t.Fatal("profiles share identity")
	}
	for _, p := range []model.Profile{a, b} {
		if !storage.Within(p.Home, []string{st.Root}) {
			t.Fatal("unmanaged destination")
		}
		info, _ := os.Stat(p.Home)
		if info.Mode().Perm() != 0700 {
			t.Fatal(info.Mode())
		}
	}
}
func TestInvalidNameAndRevision(t *testing.T) {
	s, _ := fixture(t)
	if _, err := s.Create(context.Background(), "", "profile", 1); err == nil {
		t.Fatal("empty name accepted")
	}
	if _, err := s.Create(context.Background(), "name", "bad", 1); err == nil {
		t.Fatal("invalid auth mode")
	}
	if _, err := s.Create(context.Background(), "name", "profile", 99); fault.Code(err) != "REVISION_CONFLICT" {
		t.Fatal(err)
	}
}
func TestImportCPAAndExportRequireConsent(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	p, _ := s.Create(ctx, "A", "profile", 1)
	raw := []byte(`{"type":"codex","email":"a@example.test","account_id":"account-a","access_token":"test-token","refresh_token":"test-refresh","id_token":"test-id"}`)
	if err := s.ImportAuth(ctx, p.ID, raw, false); fault.Code(err) != "CONFIRMATION_REQUIRED" {
		t.Fatal(err)
	}
	if err := s.ImportAuth(ctx, p.ID, raw, true); err != nil {
		t.Fatal(err)
	}
	auth, err := os.ReadFile(filepath.Join(p.Home, "auth.json"))
	if err != nil || !strings.Contains(string(auth), "test-token") {
		t.Fatal(err)
	}
	state, _ := st.Read(ctx)
	b, _ := json.Marshal(state)
	if strings.Contains(string(b), "test-token") {
		t.Fatal("credential leaked into state")
	}
	if _, err = s.ExportCPA(ctx, p.ID, false); fault.Code(err) != "CONFIRMATION_REQUIRED" {
		t.Fatal(err)
	}
	b, err = s.ExportCPA(ctx, p.ID, true)
	if err != nil || !strings.Contains(string(b), "account-a") {
		t.Fatal(err)
	}
}
func TestPATSwitchIsExplicitAndRecoverable(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	p, _ := s.Create(ctx, "PAT", "pat", 1)
	if err := s.ImportAuth(ctx, p.ID, []byte(`{"tokens":{"access_token":"pat-value","account_id":"pat-account"}}`), true); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(s.SharedHome, 0700)
	old := []byte(`{"tokens":{"access_token":"previous"}}`)
	os.WriteFile(filepath.Join(s.SharedHome, "auth.json"), old, 0600)
	if err := s.ActivatePAT(ctx, p.ID, false); fault.Code(err) != "CONFIRMATION_REQUIRED" {
		t.Fatal(err)
	}
	if err := s.ActivatePAT(ctx, p.ID, true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(s.SharedHome, "auth.json"))
	if !strings.Contains(string(b), "pat-value") {
		t.Fatal(string(b))
	}
	state, _ := st.Read(ctx)
	if state.ActivePAT != p.ID {
		t.Fatal(state.ActivePAT)
	}
	if err := s.RestorePAT(ctx, true); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(s.SharedHome, "auth.json"))
	if string(b) != string(old) {
		t.Fatal("previous auth not restored")
	}
}
func TestDiscoveryDoesNotWriteAndDeletePreservesHome(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	root := t.TempDir()
	home := filepath.Join(root, ".codex-office")
	os.Mkdir(home, 0700)
	os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{}`), 0600)
	list, err := Discover(root)
	if err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}
	p, err := s.Register(ctx, home, "external", 1)
	if err != nil {
		t.Fatal(err)
	}
	state, _ := st.Read(ctx)
	if err = s.Delete(ctx, p.ID, state.Revision, true); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(home, "auth.json")); err != nil {
		t.Fatal("external data deleted")
	}
}
func TestProxyValidationAndCredentialRejection(t *testing.T) {
	for _, p := range []model.Proxy{{Mode: "manual", HTTPS: "http://user:secret@example.test"}, {Mode: "manual", All: "file:///tmp/socket"}, {Mode: "madeup"}} {
		if ValidateProxy(p) == nil {
			t.Fatal(p)
		}
	}
	if err := ValidateProxy(model.Proxy{Mode: "manual", All: "socks5h://127.0.0.1:1080"}); err != nil {
		t.Fatal(err)
	}
}

func TestPersonalAccessTokenAndSeparateQuotaCredential(t *testing.T) {
	s, store := fixture(t)
	ctx := context.Background()
	p, e := s.Create(ctx, "PAT", "pat", 1)
	if e != nil {
		t.Fatal(e)
	}
	runtime, e := NormalizePAT([]byte(`{"tokens":{"access_token":"quota-session","account_id":"account"}}`), "runtime-pat", "2027-01-01T00:00:00Z")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ImportAuth(ctx, p.ID, runtime, true); e != nil {
		t.Fatal(e)
	}
	st, _ := store.Read(ctx)
	p = st.Profiles[p.ID]
	if p.QuotaCredentialID == "" || p.TokenExpiration == nil {
		t.Fatal("missing separate quota identity", p)
	}
	qp, e := s.QuotaProfile(ctx, p)
	if e != nil || qp.Home == s.SharedHome {
		t.Fatal(qp, e)
	}
	b, _ := os.ReadFile(filepath.Join(qp.Home, "auth.json"))
	if !strings.Contains(string(b), "quota-session") || strings.Contains(string(b), "runtime-pat") {
		t.Fatal("quota/runtime credentials mixed")
	}
	os.MkdirAll(s.SharedHome, 0700)
	os.WriteFile(filepath.Join(s.SharedHome, "auth.json"), []byte(`{"old":true}`), 0600)
	os.WriteFile(filepath.Join(s.SharedHome, "auth-f.json"), []byte(`{"older":"quota"}`), 0600)
	if e = s.ActivatePAT(ctx, p.ID, true); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(filepath.Join(s.SharedHome, "auth.json"))
	if !strings.Contains(string(b), "runtime-pat") || strings.Contains(string(b), "quota-session") {
		t.Fatal("wrong runtime auth")
	}
	b, _ = os.ReadFile(filepath.Join(s.SharedHome, "auth-f.json"))
	if !strings.Contains(string(b), "quota-session") {
		t.Fatal("missing secondary auth")
	}
	export, e := s.ExportCPA(ctx, p.ID, true)
	if e != nil || !strings.Contains(string(export), "Bearer runtime-pat") || !strings.Contains(string(export), "quota-session") {
		t.Fatal(string(export), e)
	}
	if e = s.RestorePAT(ctx, true); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(filepath.Join(s.SharedHome, "auth-f.json"))
	if string(b) != `{"older":"quota"}` {
		t.Fatal("quota baseline not restored", string(b))
	}
}
func TestPersonalAccessTokenImportDoesNotNeedFakeOAuthTokens(t *testing.T) {
	b, _, e := NormalizeAuth([]byte(`{"OPENAI_API_KEY":null,"personal_access_token":"real-pat"}`))
	if e != nil || !strings.Contains(string(b), "real-pat") {
		t.Fatal(string(b), e)
	}
	if strings.Contains(string(b), "id_token") {
		t.Fatal("fabricated OAuth identity")
	}
}

func TestProviderAPIKeyNeverUsesOAuthOrQuotaCredentials(t *testing.T) {
	ctx := context.Background()
	s, _ := fixture(t)
	p, e := s.Create(ctx, "API Source", "profile", 1)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ImportAuth(ctx, p.ID, []byte(`{"OPENAI_API_KEY":"only-this-api-key"}`), true); e != nil {
		t.Fatal(e)
	}
	key, e := s.ProviderAPIKey(ctx, p.ID)
	if e != nil || key != "only-this-api-key" {
		t.Fatal(key, e)
	}
	if e = s.ImportAuth(ctx, p.ID, []byte(`{"tokens":{"access_token":"oauth-access","refresh_token":"oauth-refresh","id_token":"oauth-id"}}`), true); e != nil {
		t.Fatal(e)
	}
	if key, e = s.ProviderAPIKey(ctx, p.ID); key != "" || fault.Code(e) != "CODEX_API_KEY_MISSING" {
		t.Fatal("OAuth is not an API key", key, e)
	}
}

func TestNativeAuthPreservesUnknownFieldsAndRefresh(t *testing.T) {
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"test-access","refresh_token":"test-refresh","id_token":"test-id","account_id":"a","future_scope":"team"},"last_refresh":"2026-07-17T00:00:00Z","workspace_extension":{"id":"w"}}`)
	out, _, e := NormalizeAuth(raw)
	if e != nil {
		t.Fatal(e)
	}
	var got map[string]any
	json.Unmarshal(out, &got)
	if got["last_refresh"] != "2026-07-17T00:00:00Z" || got["workspace_extension"] == nil || got["tokens"].(map[string]any)["future_scope"] != "team" {
		t.Fatal(string(out))
	}
	out, _, e = NormalizeAuth([]byte(`{"OPENAI_API_KEY":"test-api","future_flag":true}`))
	if e != nil || !strings.Contains(string(out), "future_flag") {
		t.Fatal(string(out), e)
	}
	if _, _, e = NormalizeAuth([]byte(`{"OPENAI_API_KEY":"test\napi"}`)); e == nil {
		t.Fatal("invalid token accepted")
	}
}

func TestPATKeepsIsolatedHomeAndGlobalModeControlsRuntime(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	p, e := s.Create(ctx, "PAT isolated", "pat", 1)
	if e != nil {
		t.Fatal(e)
	}
	if p.Home == s.SharedHome {
		t.Fatal("PAT identity incorrectly shares its stored home")
	}
	raw, e := NormalizePAT([]byte(`{"access_token":"quota-session"}`), "runtime-pat", "")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ImportAuth(ctx, p.ID, raw, true); e != nil {
		t.Fatal(e)
	}
	for file, want := range map[string]string{"auth.json": "runtime-pat", "auth-f.json": "quota-session"} {
		b, e := os.ReadFile(filepath.Join(p.Home, file))
		if e != nil || !strings.Contains(string(b), want) {
			t.Fatalf("%s: %s %v", file, b, e)
		}
	}
	r, e := s.RuntimeProfile(ctx, p.ID)
	if e != nil || r.Home != p.Home {
		t.Fatalf("isolated launch: %+v %v", r, e)
	}
	state, _ := st.Read(ctx)
	_, e = st.Update(ctx, state.Revision, func(v *model.State) error { v.Settings.AccountMode = "pat"; return nil })
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.RuntimeProfile(ctx, p.ID); fault.Code(e) != "PAT_INACTIVE" {
		t.Fatal(e)
	}
	if e = s.ActivatePAT(ctx, p.ID, true); e != nil {
		t.Fatal(e)
	}
	r, e = s.RuntimeProfile(ctx, p.ID)
	if e != nil || r.Home != s.SharedHome {
		t.Fatalf("shared launch: %+v %v", r, e)
	}
	b, _ := os.ReadFile(filepath.Join(p.Home, "auth.json"))
	if !strings.Contains(string(b), "runtime-pat") {
		t.Fatal("switch altered identity home")
	}
	if e = s.RestorePAT(ctx, true); e != nil {
		t.Fatal(e)
	}
	state, _ = st.Read(ctx)
	if e = s.Delete(ctx, p.ID, state.Revision, true); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(p.Home); !os.IsNotExist(e) {
		t.Fatal("isolated PAT home not archived", e)
	}
}
func TestImportedProfileCanActivateInPATMode(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()
	p, e := s.Create(ctx, "Imported OAuth", "profile", 1)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ImportAuth(ctx, p.ID, []byte(`{"tokens":{"access_token":"oauth"}}`), true); e != nil {
		t.Fatal(e)
	}
	if e = s.ActivatePAT(ctx, p.ID, true); e != nil {
		t.Fatal("legacy switch accepts all non-main identities", e)
	}
	state, _ := st.Read(ctx)
	if state.ActivePAT != p.ID {
		t.Fatal(state.ActivePAT)
	}
}

func TestCreateCanCopyConfigurationWithoutCopyingCredentials(t *testing.T) {
	ctx := context.Background()
	s, store := fixture(t)
	source, e := s.Create(ctx, "Template", "profile", 1)
	if e != nil {
		t.Fatal(e)
	}
	config := []byte("# keep comments\r\nmodel=\"example\"\r\n[custom]\r\nflag=true\r\n")
	os.WriteFile(filepath.Join(source.Home, "config.toml"), config, 0600)
	os.WriteFile(filepath.Join(source.Home, "auth.json"), []byte("private credential"), 0600)
	state, _ := store.Read(ctx)
	target, e := s.Create(ctx, "Copied", "profile", state.Revision, source.ID)
	if e != nil {
		t.Fatal(e)
	}
	got, e := storage.ReadFile(filepath.Join(target.Home, "config.toml"), 1<<20)
	if e != nil || string(got) != string(config) {
		t.Fatal("config not preserved", e)
	}
	if _, e = os.Stat(filepath.Join(target.Home, "auth.json")); !os.IsNotExist(e) {
		t.Fatal("copied authentication")
	}
	for _, dir := range []string{"sessions", "cache", "log", "tmp", "rules", "skills", "memories"} {
		if f, e := os.Stat(filepath.Join(target.Home, dir)); e != nil || !f.IsDir() {
			t.Fatal("missing legacy home directory", dir, e)
		}
	}
	state, _ = store.Read(ctx)
	if _, e = s.Create(ctx, "Missing template", "profile", state.Revision, "missing"); fault.Code(e) != "PROFILE_NOT_FOUND" {
		t.Fatal(e)
	}
	state, _ = store.Read(ctx)
	store.Update(ctx, state.Revision, func(v *model.State) error {
		v.Bindings[source.ID] = model.Binding{ID: "bound", ProfileID: source.ID}
		return nil
	})
	state, _ = store.Read(ctx)
	if _, e = s.Create(ctx, "No credential reuse", "profile", state.Revision, source.ID); fault.Code(e) != "PROFILE_BOUND" {
		t.Fatal(e)
	}
}

// Account note/proxy/renewal were formerly mapped to an entire test file,
// without a test exercising the Edit operation itself.
func TestEditPersistsNoteRenewalAndProxyWithRevision(t *testing.T) {
	svc, st := fixture(t)
	ctx := context.Background()
	p, err := svc.Create(ctx, "Original", "profile", 1)
	if err != nil {
		t.Fatal(err)
	}
	before, err := st.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	proxy := model.Proxy{Mode: "manual", HTTPS: "http://127.0.0.1:8080"}
	if err = svc.Edit(ctx, p.ID, "Renamed", "account note", "2027-01-31", proxy, before.Revision); err != nil {
		t.Fatal(err)
	}
	after, err := svc.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Name != "Renamed" || after.Note != "account note" || after.RenewalDate != "2027-01-31" || after.Proxy.HTTPS != proxy.HTTPS {
		t.Fatalf("edit not persisted: %+v", after)
	}
	if err = svc.Edit(ctx, p.ID, "stale", "", "", model.Proxy{Mode: "inherit"}, before.Revision); fault.Code(err) != "REVISION_CONFLICT" {
		t.Fatalf("stale write accepted: %v", err)
	}
	if err = svc.Edit(ctx, p.ID, "anything", "", "not-a-date", model.Proxy{Mode: "inherit"}, before.Revision+1); fault.Code(err) != "INVALID_DATE" {
		t.Fatalf("invalid renewal accepted: %v", err)
	}
}

func TestProfileTokenExpirationStatusTransitions(t *testing.T) {
	svc, st := fixture(t)
	ctx := context.Background()
	p, err := svc.Create(ctx, "Session", "profile", 1)
	if err != nil {
		t.Fatal(err)
	}
	v, err := svc.Expiration(ctx, p.ID)
	if err != nil || v.Status != "unknown" || v.Days != nil {
		t.Fatalf("unknown expiration: %+v %v", v, err)
	}
	cases := []struct {
		days int
		want string
	}{
		{-2, "expired"}, {3, "critical"}, {15, "warning"}, {40, "ok"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			s, err := st.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			expires := time.Now().Add(time.Duration(tc.days) * 24 * time.Hour)
			if _, err = st.Update(ctx, s.Revision, func(dst *model.State) error {
				v := dst.Profiles[p.ID]
				v.TokenExpiration = &expires
				dst.Profiles[p.ID] = v
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			got, err := svc.Expiration(ctx, p.ID)
			if err != nil || got.Status != tc.want || got.Days == nil || got.ExpiresAt == nil {
				t.Fatalf("%+v %v", got, err)
			}
		})
	}
	if _, err := svc.Expiration(ctx, "missing"); fault.Code(err) != "PROFILE_NOT_FOUND" {
		t.Fatal("missing account accepted:", err)
	}
}
