package desktop

import (
	"github.com/egoist/mygo/ui"
	"lam.local/lam2/internal/model"
	"testing"
)

// These are actual MyGo tester cases, NOT copies of style strings. They
// must execute in the native CI job before any visual parity claim.
func TestOriginalAccountModalLayoutAndTabs(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	u.newProfile()
	screen := ui.NewTester(u.view, 1280, 900)
	for _, s := range []string{"Add Account", "Profile Account", "CLI Auth", "Import Session", "Account name *", "Session JSON *", "CODEX_HOME", "Overwrite wrapper if it exists", "Import Profile"} {
		if !screen.HasText(s) {
			t.Fatalf("original account modal lost %q", s)
		}
	}
	if e := screen.Click("CLI Auth"); e != nil {
		t.Fatal(e)
	}
	if u.profileCreateMethodTab != 0 {
		t.Fatal("auth-method tab did not switch")
	}
}
func TestOriginalProviderEditorLayout(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	u.openProvider(model.Provider{Protocol: "responses"})
	screen := ui.NewTester(u.view, 1280, 900)
	for _, s := range []string{"Add External API", "Close", "Advanced options  +", "Connection details", "Model configuration", "Review API Account", "Create API Account"} {
		if !screen.HasText(s) {
			t.Fatalf("original API editor lost %q", s)
		}
	}
}
func TestSettingsOriginalInformationArchitecture(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	u.router.Push("/settings")
	screen := ui.NewTester(u.view, 1280, 900)
	for _, s := range []string{"APP SETTINGS", "INTEGRATIONS", "RESOURCES", "General Settings", "APP PREFERENCES", "Mode availability", "Codex launch permissions", "Handoff terminal", "Compact card actions"} {
		if !screen.HasText(s) {
			t.Fatalf("settings layout lost %q", s)
		}
	}
}
func TestUsageTabsOriginalOrderAndSelection(t *testing.T) {
	u := New(nil)
	u.locale = "en"
	u.router.Push("/usage")
	screen := ui.NewTester(u.view, 1280, 900)
	for _, s := range []string{"TOKEN VOLUMES", "PRODUCTIVITY & STREAKS", "COST & CREDITS", "Insights", "Calls", "Threads", "Diagnostics"} {
		if !screen.HasText(s) {
			t.Fatalf("usage overview lost %q", s)
		}
	}
	if u.usageTabIndex != 0 {
		t.Fatal("Insights must be the initial tab")
	}
	if e := screen.Click("Calls"); e != nil {
		t.Fatal(e)
	}
	if u.usageTabIndex != 1 {
		t.Fatal("Calls tab must activate the calls index")
	}
}
