// Featuregate binds LAM legacy feature IDs to real, named Go test results.
// It is a standalone standard-library Go tool: not part of the MyGo app.
package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

type feature struct {
	ID, LegacyStatus, Tests, Layer, Assertion string
}
type outcome struct{ ID, LegacyStatus, CoverageStatus, Layer, Tests, Assertion string }

var validTest = regexp.MustCompile(`^[a-z][a-z0-9_-]*:Test[A-Za-z0-9_]+$`)
var namedTest = regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]+)\(t \*testing\.T\)`)

func readCSV(path string) ([][]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	return reader.ReadAll()
}
func readFeatures(path string) ([]feature, error) {
	records, err := readCSV(path)
	if err != nil {
		return nil, err
	}
	want := []string{"id", "status", "test_targets", "test_layer", "test_assertion"}
	if len(records) < 2 {
		return nil, errors.New("feature manifest has no entries")
	}
	idx := map[string]int{}
	for i, v := range records[0] {
		idx[v] = i
	}
	for _, v := range want {
		if _, ok := idx[v]; !ok {
			return nil, fmt.Errorf("missing column %q", v)
		}
	}
	out := make([]feature, 0, len(records)-1)
	seen := map[string]bool{}
	for n, rec := range records[1:] {
		for _, v := range want {
			if idx[v] >= len(rec) {
				return nil, fmt.Errorf("line %d: missing %s", n+2, v)
			}
		}
		id := rec[idx["id"]]
		if id == "" || seen[id] {
			return nil, fmt.Errorf("line %d: missing/duplicated ID %q", n+2, id)
		}
		seen[id] = true
		layer := rec[idx["test_layer"]]
		if layer != "domain" && layer != "integration" && layer != "native" && layer != "system" {
			return nil, fmt.Errorf("line %d: invalid verification layer %q", n+2, layer)
		}
		out = append(out, feature{id, rec[idx["status"]], rec[idx["test_targets"]], rec[idx["test_layer"]], rec[idx["test_assertion"]]})
	}
	// The frozen baseline is 111 IPC operations and 128 observed flows.
	for n := 1; n <= 111; n++ {
		id := fmt.Sprintf("CMD-%03d", n)
		if !seen[id] {
			return nil, fmt.Errorf("frozen baseline missing %s", id)
		}
	}
	for n := 1; n <= 128; n++ {
		id := fmt.Sprintf("FLOW-%03d", n)
		if !seen[id] {
			return nil, fmt.Errorf("frozen baseline missing %s", id)
		}
	}
	return out, nil
}
func testInventory(root string) (map[string]bool, error) {
	inventory := map[string]bool{}
	err := filepath.Walk(filepath.Join(root, "internal"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		packageName := filepath.Base(filepath.Dir(path))
		for _, m := range namedTest.FindAllStringSubmatch(string(src), -1) {
			inventory[packageName+":"+m[1]] = true
		}
		return nil
	})
	return inventory, err
}
func passedTests(path string) (map[string]bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tests := map[string]bool{}
	failed := map[string]bool{}
	packages := map[string]bool{}
	packageFailures := map[string]bool{}
	for n, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev struct{ Action, Package, Test string }
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("test JSON line %d: %w", n+1, err)
		}
		pkg := filepath.Base(ev.Package)
		if ev.Test == "" {
			if ev.Action == "pass" {
				packages[pkg] = true
			}
			if ev.Action == "fail" {
				packageFailures[pkg] = true
			}
			continue
		}
		top := strings.Split(ev.Test, "/")[0]
		key := pkg + ":" + top
		switch ev.Action {
		case "fail", "skip":
			failed[key] = true
		case "pass":
			if ev.Test == top {
				tests[key] = true
			}
		}
	}
	for key := range tests {
		pkg := strings.SplitN(key, ":", 2)[0]
		if failed[key] || !packages[pkg] || packageFailures[pkg] {
			delete(tests, key)
		}
	}
	return tests, nil
}

func classify(f feature, inventory, passed map[string]bool) string {
	return classifyWithEvidence(f, inventory, passed, nil, nil, false)
}

// Only real package-level Go test success may promote a feature. System
// evidence is a separate artifact and can never replace an automated test.
func classifyWithEvidence(f feature, inventory, domainPass, integrationPass map[string]bool, systemProof map[string]bool, macARM bool) string {
	if f.LegacyStatus == "EXCLUDED" {
		return "EXCLUDED"
	}
	if strings.TrimSpace(f.Tests) == "" {
		return "TEST_MISSING"
	}
	for _, ref := range strings.Split(f.Tests, ";") {
		ref = strings.TrimSpace(ref)
		if !validTest.MatchString(ref) || !inventory[ref] {
			return "TEST_TARGET_NOT_FOUND"
		}
	}
	allPassed := func(set map[string]bool) bool {
		for _, ref := range strings.Split(f.Tests, ";") {
			if !set[strings.TrimSpace(ref)] {
				return false
			}
		}
		return true
	}
	verifiedDomain := allPassed(domainPass)
	verifiedNative := allPassed(integrationPass)
	if !verifiedDomain && !verifiedNative {
		return "TEST_NOT_EXECUTED"
	}
	switch f.LegacyStatus {
	case "PARTIAL":
		return "PARTIAL_TEST_PASS_FEATURE_INCOMPLETE"
	case "GAP", "TEST_GAP":
		return "IMPLEMENTATION_GAP"
	}
	if f.Layer == "domain" {
		return "TEST_PASS"
	}
	// Never use a domain-only proof for a system, UI or external integration.
	if !verifiedNative {
		return "DOMAIN_ONLY_NOT_NATIVE_VERIFIED"
	}
	if f.Layer == "native" && !macARM {
		return "NATIVE_PLATFORM_UNVERIFIED"
	}
	if f.Layer == "system" && (!macARM || !systemProof[f.ID]) {
		return "SYSTEM_ACCEPTANCE_MISSING"
	}
	if f.LegacyStatus != "VERIFIED" {
		return "INTEGRATION_EXECUTED_PENDING_PARITY_REVIEW"
	}
	return "TEST_PASS"
}

func readSystemProof(path, root string) (map[string]bool, error) {
	out := map[string]bool{}
	if path == "" {
		return out, nil
	}
	rows, err := readCSV(filepath.Join(root, path))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || len(rows[0]) != 3 || rows[0][0] != "id" || rows[0][1] != "result" || rows[0][2] != "evidence" {
		return nil, errors.New("system evidence must have id,result,evidence header")
	}
	for i, row := range rows[1:] {
		if len(row) != 3 {
			return nil, fmt.Errorf("invalid system acceptance row %d", i+2)
		}
		if row[1] != "PASS" {
			continue
		}
		if !strings.HasPrefix(row[2], "reports/") || filepath.IsAbs(row[2]) || strings.Contains(row[2], "..") {
			return nil, fmt.Errorf("unsafe system acceptance path at row %d", i+2)
		}
		st, e := os.Stat(filepath.Join(root, row[2]))
		if e != nil || st.IsDir() || st.Size() == 0 {
			return nil, fmt.Errorf("missing acceptance artifact at row %d", i+2)
		}
		out[row[0]] = true
	}
	return out, nil
}

func writeResults(path string, rows []outcome) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write([]string{"id", "original_status", "test_coverage", "layer", "test_targets", "assertion"}); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.Write([]string{r.ID, r.LegacyStatus, r.CoverageStatus, r.Layer, r.Tests, r.Assertion}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
func main() {
	matrix := flag.String("matrix", "tests/feature-contracts.csv", "manifest (ID -> test cases)")
	results := flag.String("results", "reports/feature-pass-events.jsonl", "go test -json output")
	root := flag.String("root", ".", "project root")
	nativeResults := flag.String("integration-results", "", "real full-package go test -json output")
	systemEvidence := flag.String("system-evidence", "", "id,result,evidence records from macOS acceptance")
	output := flag.String("out", "reports/feature-test-results.csv", "machine readable outcome")
	mode := flag.String("mode", "domain", "domain or release")
	flag.Parse()
	if *mode != "domain" && *mode != "release" {
		fmt.Fprintln(os.Stderr, "invalid mode")
		os.Exit(2)
	}
	features, e := readFeatures(filepath.Join(*root, *matrix))
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	inventory, e := testInventory(*root)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	passed, e := passedTests(filepath.Join(*root, *results))
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	nativePassed := map[string]bool{}
	if *nativeResults != "" {
		nativePassed, e = passedTests(filepath.Join(*root, *nativeResults))
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(2)
		}
	}
	proof, e := readSystemProof(*systemEvidence, *root)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	macARM := runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"
	statuses := map[string]int{}
	audit := []outcome{}
	failures := []string{}
	for _, f := range features {
		state := classifyWithEvidence(f, inventory, passed, nativePassed, proof, macARM)
		if f.LegacyStatus == "DOMAIN_PASS" && state != "TEST_PASS" {
			failures = append(failures, f.ID+": "+state)
		}
		if f.LegacyStatus == "DOMAIN_PASS" && strings.TrimSpace(f.Assertion) == "" {
			failures = append(failures, f.ID+": no expected behavior")
		}
		if *mode == "release" && state != "TEST_PASS" && state != "EXCLUDED" {
			failures = append(failures, f.ID+": release blocker: "+state)
		}
		statuses[state]++
		audit = append(audit, outcome{f.ID, f.LegacyStatus, state, f.Layer, f.Tests, f.Assertion})
	}
	if e := writeResults(filepath.Join(*root, *output), audit); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	keys := []string{}
	for k := range statuses {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("Feature test evidence: %d features, %d named Go test functions, %d passing top-level test results\n", len(features), len(inventory), len(passed))
	for _, key := range keys {
		fmt.Printf("%-44s %d\n", key, statuses[key])
	}
	if len(failures) > 0 {
		for _, v := range failures {
			fmt.Fprintln(os.Stderr, "FAIL", v)
		}
		os.Exit(1)
	}
	if *mode == "domain" {
		fmt.Println("DOMAIN GATE PASS. Native/integration/release gaps remain blockers; not a product release pass.")
	}
}
