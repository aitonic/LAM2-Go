package desktop

import (
	"github.com/egoist/mygo/ui"
	"lam.local/lam2/internal/model"
	"testing"
)

func TestFiveNativePages(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	tt := ui.NewTester(u.view, 1200, 800)
	for _, label := range []string{"Overview", "Usage", "Sessions", "Providers", "Settings"} {
		if !tt.HasText(label) {
			t.Fatalf("missing %s: %v", label, tt.Texts())
		}
	}
	if e := tt.Click("Sessions"); e != nil {
		t.Fatal(e)
	}
	if u.router.Path() != "/sessions" {
		t.Fatal(u.router.Path())
	}
	if e := tt.Click("Providers"); e != nil {
		t.Fatal(e)
	}
	if u.router.Path() != "/providers" {
		t.Fatal(u.router.Path())
	}
}
func TestCreateProfileFormCancelsWithoutMutation(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	tt := ui.NewTester(u.view, 1200, 900)
	if e := tt.Click("+ New Account"); e != nil {
		t.Fatal(e)
	}
	if !u.profileOpen || !tt.HasText("Account name *") {
		t.Fatal(tt.Texts())
	}
	if e := tt.Click("Cancel"); e != nil {
		t.Fatal(e)
	}
	if u.profileOpen || len(u.basic.Profiles) != 0 {
		t.Fatal("cancel mutated accounts")
	}
}
func TestUnknownQuotaIsNotZero(t *testing.T) {
	if QuotaLabel(nil) != "Unknown" {
		t.Fatal("unknown quota must not be exhausted")
	}
	z := 0.0
	if QuotaLabel(&z) != "0.0%" {
		t.Fatal("zero is a known quota")
	}
}

func TestAccountUsageAndAPIAccountEntries(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	u.router.Push("/usage")
	tt := ui.NewTester(u.view, 1280, 900)
	if err := tt.Click("Filters"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Usage source") {
		t.Fatal(tt.Texts())
	}
	u.usageSource = "account"
	account := ui.NewTester(u.view, 1280, 900)
	if !account.HasText("Select a profile to view account-reported usage.") {
		t.Fatal(account.Texts())
	}
	u.router.Push("/providers")
	providers := ui.NewTester(u.view, 1280, 900)
	if !providers.HasText("New API account") {
		t.Fatal(providers.Texts())
	}
}

func TestCredentialPasteDialogClearsSensitiveDraftOnCancel(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	u.credentialOpen = true
	u.authJSONDraft = `{"accessToken":"test-only"}`
	u.patDraft = "test-only-pat"
	u.authPurpose = "runtime"
	tt := ui.NewTester(u.view, 1200, 900)
	if !tt.HasText("Session / auth JSON") {
		t.Fatal(tt.Texts())
	}
	if e := tt.Click("Cancel"); e != nil {
		t.Fatal(e)
	}
	if u.credentialOpen || u.authJSONDraft != "" || u.patDraft != "" {
		t.Fatal("sensitive draft retained")
	}
}

func TestDiscoveredAccountRegistrationIsReachableWithoutExistingRegistration(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	u.profileRow = 0
	u.basic.Profiles = []model.Profile{{ID: "main", Name: "main", Home: "/tmp/nonexistent-codex", Managed: false}}
	tt := ui.NewTester(u.view, 1280, 850)
	if err := tt.Click("⋮"); err != nil {
		t.Fatal("discovered account must have an operations menu:", err)
	}
	if err := tt.Click("Register discovered account"); err != nil {
		t.Fatal("discovery registration UI is disabled:", err)
	}
	if !u.confirmOpen {
		t.Fatal("registration must display a confirmation before writing a discovered home")
	}
}

func TestTrayExposesCodexAndAntigravityWithPerAccountAction(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	u.basic.Profiles = []model.Profile{{ID: "one", Name: "Office", Home: "/tmp/codex-office"}}
	u.basic.State.Profiles["one"] = u.basic.Profiles[0]
	tt := ui.NewTester(u.trayView, 520, 750)
	for _, want := range []string{"Codex", "Antigravity", "Refresh Office", "Relay latest"} {
		if !tt.HasText(want) {
			t.Fatalf("tray missing %q: %v", want, tt.Texts())
		}
	}
}
