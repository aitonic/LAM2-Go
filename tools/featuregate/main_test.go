package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingOrUnexecutedTestCannotPassFeatureGate(t *testing.T) {
	id := feature{ID: "CMD-004", LegacyStatus: "DOMAIN_PASS", Layer: "domain", Assertion: "session pagination"}
	inv := map[string]bool{"session:TestPaged": true}
	pass := map[string]bool{"session:TestPaged": true}
	if got := classify(id, inv, pass); got != "TEST_MISSING" {
		t.Fatal(got)
	}
	id.Tests = "session:TestPaged"
	if got := classify(id, inv, map[string]bool{}); got != "TEST_NOT_EXECUTED" {
		t.Fatal(got)
	}
	if got := classify(id, inv, pass); got != "TEST_PASS" {
		t.Fatal(got)
	}
	id.Tests = "session:TestWrong"
	if got := classify(id, inv, pass); got != "TEST_TARGET_NOT_FOUND" {
		t.Fatal(got)
	}
}
func TestNativeAndPartialCannotInheritDomainPass(t *testing.T) {
	f := feature{ID: "FLOW-093", LegacyStatus: "NATIVE_UNVERIFIED", Tests: "desktop:TestTray", Layer: "native"}
	inv := map[string]bool{"desktop:TestTray": true}
	pass := map[string]bool{"desktop:TestTray": true}
	if got := classify(f, inv, pass); got != "DOMAIN_ONLY_NOT_NATIVE_VERIFIED" {
		t.Fatal(got)
	}
	f.Layer = "domain"
	f.LegacyStatus = "PARTIAL"
	if got := classify(f, inv, pass); got != "PARTIAL_TEST_PASS_FEATURE_INCOMPLETE" {
		t.Fatal(got)
	}
	f.LegacyStatus = "GAP"
	if got := classify(f, inv, pass); got != "IMPLEMENTATION_GAP" {
		t.Fatal(got)
	}
}
func TestPassEventsRejectsSkippedFailedAndIncompletePackages(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tests.jsonl")
	events := []string{
		`{"Action":"pass","Package":"lam.local/lam2/internal/account","Test":"TestGood"}`,
		`{"Action":"pass","Package":"lam.local/lam2/internal/account"}`,
		`{"Action":"pass","Package":"lam.local/lam2/internal/usage","Test":"TestSkipped"}`,
		`{"Action":"skip","Package":"lam.local/lam2/internal/usage","Test":"TestSkipped/subcase"}`,
		`{"Action":"pass","Package":"lam.local/lam2/internal/usage"}`,
		`{"Action":"pass","Package":"lam.local/lam2/internal/session","Test":"TestIncomplete"}`,
		`{"Action":"pass","Package":"lam.local/lam2/internal/provider","Test":"TestFailed"}`,
		`{"Action":"fail","Package":"lam.local/lam2/internal/provider"}`,
	}
	if e := os.WriteFile(p, []byte(strings.Join(events, "\n")), 0600); e != nil {
		t.Fatal(e)
	}
	pass, e := passedTests(p)
	if e != nil {
		t.Fatal(e)
	}
	if !pass["account:TestGood"] || pass["usage:TestSkipped"] || pass["session:TestIncomplete"] || pass["provider:TestFailed"] {
		t.Fatal(pass)
	}
}
func TestDuplicateManifestIDRejected(t *testing.T) {
	p := filepath.Join(t.TempDir(), "matrix.csv")
	contents := "id,status,test_targets,test_layer,test_assertion\nCMD-004,DOMAIN_PASS,session:TestPage,domain,page\nCMD-004,DOMAIN_PASS,session:TestPage,domain,page\n"
	if e := os.WriteFile(p, []byte(contents), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := readFeatures(p); e == nil {
		t.Fatal("duplicate feature silently accepted")
	}
}
