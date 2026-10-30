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

func TestAuthModeTabsSwitchBetweenProfileAndPAT(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	u.settings.AccountMode = "profile"
	tt := ui.NewTester(u.view, 1280, 850)

	if !tt.HasText("Profile Mode") || !tt.HasText("PAT Mode") {
		t.Fatalf("titlebar missing auth mode tabs: %v", tt.Texts())
	}
	if !tt.HasText("Codex profile mode") {
		t.Fatalf("expected profile active source: %v", tt.Texts())
	}

	// Click PAT Mode tab
	if err := tt.Click("PAT Mode"); err != nil {
		t.Fatal("failed to click PAT Mode tab:", err)
	}
	if u.settings.AccountMode != "pat" {
		t.Fatalf("expected AccountMode to be 'pat', got %q", u.settings.AccountMode)
	}
	if u.settings.ModeAvailability != "both" {
		t.Fatalf("expected ModeAvailability to upgrade to 'both', got %q", u.settings.ModeAvailability)
	}

	// Verify Overview Active source now indicates PAT mode
	ttPat := ui.NewTester(u.view, 1280, 850)
	if !ttPat.HasText("PAT mode · No active account") {
		t.Fatalf("expected PAT mode active source indicator: %v", ttPat.Texts())
	}

	// Click back to Profile Mode tab
	if err := ttPat.Click("Profile Mode"); err != nil {
		t.Fatal("failed to click Profile Mode tab:", err)
	}
	if u.settings.AccountMode != "profile" {
		t.Fatalf("expected AccountMode to be 'profile', got %q", u.settings.AccountMode)
	}
}

func TestPATModeAccountCardsExposeActiveBadgeAndSwitchPAT(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	u.settings.AccountMode = "pat"
	u.basic.State.Settings.AccountMode = "pat"
	u.basic.Profiles = []model.Profile{
		{ID: "main", Name: "main", Home: "/tmp/codex-main", Managed: true},
		{ID: "pat-1", Name: "Team PAT", Home: "/tmp/codex-pat1", Managed: true},
	}
	u.basic.State.Profiles["main"] = u.basic.Profiles[0]
	u.basic.State.Profiles["pat-1"] = u.basic.Profiles[1]

	// Before activation: Team PAT should show "Switch PAT" button
	tt := ui.NewTester(u.view, 1280, 850)
	if !tt.HasText("Switch PAT") {
		t.Fatalf("expected 'Switch PAT' button on unactivated PAT card: %v", tt.Texts())
	}

	// After activation: Active PAT should show "ACTIVE" badge
	u.basic.State.ActivePAT = "pat-1"
	ttActive := ui.NewTester(u.view, 1280, 850)
	if !ttActive.HasText("ACTIVE") {
		t.Fatalf("expected 'ACTIVE' badge on active PAT card: %v", ttActive.Texts())
	}
	if !ttActive.HasText("PAT · pat-1") {
		t.Fatalf("expected 'PAT · pat-1' in active source bar: %v", ttActive.Texts())
	}
}

func TestPATAccountCreationModalLayoutAndTokenImport(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	u.settings.AccountMode = "pat"
	u.basic.State.Settings.AccountMode = "pat"

	u.newProfile()
	if !u.profileOpen {
		t.Fatal("expected profileOpen to be true")
	}
	if u.profileDraft.AuthMode != "pat" {
		t.Fatalf("expected AuthMode to be 'pat', got %s", u.profileDraft.AuthMode)
	}

	tt := ui.NewTester(u.view, 1280, 850)
	for _, expectedText := range []string{
		"PAT Account",
		"Token & Session",
		"Upload auth.json",
		"Personal Access Token *",
		"Session JSON *",
		"Token Expiration",
	} {
		if !tt.HasText(expectedText) {
			t.Errorf("expected modal to contain %q, but texts were: %v", expectedText, tt.Texts())
		}
	}
}
