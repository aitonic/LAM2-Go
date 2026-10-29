package commands

import (
	"lam.local/lam2/internal/fault"
	"testing"
)

func TestParseModesAndPassthrough(t *testing.T) {
	v, e := Parse([]string{"--home", "/private/lam", "launch", "--profile", "a", "--resume", "s", "--", "--model", "x"})
	if e != nil || v.Home != "/private/lam" || v.Mode != "launch" || len(v.Extra) != 2 || v.Extra[0] != "--model" {
		t.Fatal(v, e)
	}
	v, e = Parse(nil)
	if e != nil || v.Mode != "desktop" {
		t.Fatal(v, e)
	}
}
func TestRejectUnknownFlagsAndOperations(t *testing.T) {
	for _, args := range [][]string{{"billing"}, {"auth", "--api-key", "secret"}, {"profile", "destroy"}, {"provider", "save", "--unknown"}} {
		if _, e := Parse(args); fault.Code(e) != "ARGUMENT_INVALID" {
			t.Fatal(args, e)
		}
	}
}
func TestPlansRequireExplicitConfirmation(t *testing.T) {
	v, e := Parse([]string{"handoff", "execute", "--id", "x", "--fingerprint", "abc"})
	if e != nil || v.Confirm {
		t.Fatal(v, e)
	}
	v, e = Parse([]string{"session", "archive", "--id", "x", "--confirm"})
	if e != nil || !v.Confirm {
		t.Fatal(v, e)
	}
}

func TestRelayProfileCommandsAreNotBulkSessionSync(t *testing.T) {
	for _, action := range []string{"plan-relay", "create-relay"} {
		v, e := Parse([]string{"profile", action, "--id", "plan-1", "--fingerprint", "abc", "--confirm"})
		if e != nil || v.Action != action {
			t.Fatal(v, e)
		}
	}
	if _, e := Parse([]string{"session", "sync"}); e == nil {
		t.Fatal("removed bulk sync revived")
	}
}
