package runtime

import (
	"context"
	"lam.local/lam2/internal/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentIsolation(t *testing.T) {
	p := model.Profile{Home: "/tmp/profile", Proxy: model.Proxy{Mode: "disabled"}}
	env, err := Environment([]string{"PATH=/bin", "CODEX_HOME=/other", "OPENAI_API_KEY=other-secret", "https_proxy=http://old", "HTTPS_PROXY=http://old", "TERM=xterm"}, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "other") || strings.Contains(joined, "old") {
		t.Fatal(joined)
	}
	if !strings.Contains(joined, "CODEX_HOME=/tmp/profile") {
		t.Fatal(joined)
	}
}
func TestLaunchPlanUsesArgvAndCorrectProfile(t *testing.T) {
	p := model.Profile{ID: "a", Home: t.TempDir(), AuthMode: "profile"}
	exe := filepath.Join(t.TempDir(), "fake codex")
	os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0700)
	s := model.DefaultSettings()
	s.CodexPath = exe
	plan, err := Plan(p, s, LaunchOptions{Resume: "session-123", CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Executable != exe || strings.Join(plan.Args, " ") != "--ask-for-approval on-request --sandbox workspace-write resume session-123" {
		t.Fatal(plan)
	}
	if _, err = Plan(p, s, LaunchOptions{Resume: "--evil"}); err == nil {
		t.Fatal("option injection")
	}
}
func TestShellQuoteIsSafe(t *testing.T) {
	q := ShellQuote("a'; touch /tmp/x; echo '")
	if q != "'a'\"'\"'; touch /tmp/x; echo '\"'\"''" {
		t.Fatal(q)
	}
}
func TestWrapperForwardsArgv(t *testing.T) {
	p := filepath.Join(t.TempDir(), "wrapper")
	err := WriteWrapper(context.Background(), p, "/Applications/LAM Test.app/Contents/MacOS/lam", "/tmp/a b", "p-a")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), `"$@"`) || !strings.Contains(string(b), "--profile 'p-a'") {
		t.Fatal(string(b))
	}
}

func TestSDKOverlayExplicitlyClearsParentCredentials(t *testing.T) {
	p := model.Profile{Home: t.TempDir(), Proxy: model.Proxy{Mode: "disabled"}}
	env, err := SDKOverlay([]string{"CODEX_API_KEY=other", "OPENAI_API_KEY=other", "HTTP_PROXY=http://other", "PATH=/bin"}, p)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, entry := range env {
		k, v, _ := strings.Cut(entry, "=")
		values[k] = v
	}
	for _, key := range []string{"CODEX_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
		v, ok := values[key]
		if !ok || v != "" {
			t.Fatalf("%s not explicitly cleared", key)
		}
	}
	if values["CODEX_HOME"] != p.Home || values["PATH"] != "/bin" {
		t.Fatal(values)
	}
}

func TestHeaderTokenDoesNotLeakBetweenProfiles(t *testing.T) {
	p := model.Profile{ID: "one", Home: t.TempDir(), Proxy: model.Proxy{Mode: "disabled"}}
	env, e := Environment([]string{"LAM_PROVIDER_HEADER_TOKEN=secret"}, p, nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range env {
		if strings.HasPrefix(s, "LAM_PROVIDER_HEADER_TOKEN=") {
			t.Fatal("header token inherited")
		}
	}
	overlay, e := SDKOverlay([]string{"LAM_PROVIDER_HEADER_TOKEN=secret"}, p)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, s := range overlay {
		if s == "LAM_PROVIDER_HEADER_TOKEN=" {
			found = true
		}
	}
	if !found {
		t.Fatal("append-only SDK env must explicitly erase inherited header token")
	}
}

func TestCMUXLegacyArgumentsAndFallback(t *testing.T) {
	args := CMUXArguments("echo quoted")
	if strings.Join(args, "|") != "workspace|create|--name|LAM Handoff|--command|echo quoted|--focus|true" {
		t.Fatal(args)
	}
	for _, s := range []string{"access denied", "Only processes started inside cmux", "broken pipe", "failed to write to socket"} {
		if !CMUXFallbackAllowed(s) {
			t.Fatal(s)
		}
	}
	if CMUXFallbackAllowed("unknown subcommand workspace") {
		t.Fatal("must not hide incompatible CLI")
	}
	script := CMUXScript("echo \"test\"; printf 'a\\\\b'")
	if !strings.Contains(script, "new tab") || !strings.Contains(script, "focused terminal of targetTab") || !strings.Contains(script, `\"test\"`) {
		t.Fatal(script)
	}
}
func TestLauncherCommandDoesNotEmbedCredentials(t *testing.T) {
	line, e := LauncherCommand("/Applications/LAM's.app/Contents/MacOS/lam", "/tmp/data home", "profile-a", "session-a", "/tmp/work x")
	if e != nil || !strings.Contains(line, "'--resume' 'session-a'") || !strings.Contains(line, "'--cwd' '/tmp/work x'") {
		t.Fatal(line, e)
	}
	if _, e = LauncherCommand("/app", "/data", "profile-a", "--unsafe", ""); e == nil {
		t.Fatal("option injection")
	}
	if _, e = LauncherCommand("/app", "/data", "profile-a", "session-a", "relative"); e == nil {
		t.Fatal("ambiguous CWD")
	}
}
func TestTerminalTargetsFindBundledApps(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "Cmux.app"), 0700)
	os.MkdirAll(filepath.Join(root, "Codex.app"), 0700)
	targets := DetectTerminalTargets([]string{root}, func(s string) (string, error) { return "", os.ErrNotExist })
	found := map[string]TerminalTarget{}
	for _, v := range targets {
		found[v.ID] = v
	}
	if !found["cmux"].Installed || found["ghostty"].Installed || !found["codex_app"].Installed || found["codex_app"].HandoffSupported {
		t.Fatal(found)
	}
}

func TestChatGPTProcessScopeAndBounds(t *testing.T) {
	raw := "100 501 /Applications/ChatGPT.app/Contents/MacOS/ChatGPT\n101 502 /Applications/ChatGPT.app/Contents/MacOS/ChatGPT\n102 501 /Applications/ChatGPT.app.backup/Contents/MacOS/ChatGPT\n103 501 /usr/bin/echo /Applications/ChatGPT.app/Contents/MacOS/ChatGPT\n"
	ids := ChatGPTProcessIDs(raw, 501)
	if len(ids) != 1 || ids[0] != 100 {
		t.Fatal(ids)
	}
	b, ok := ParseWindowBounds("100, 80, 1280, 820")
	if !ok || b[2] != 1280 {
		t.Fatal(b, ok)
	}
	if _, ok = ParseWindowBounds("no window"); ok {
		t.Fatal("invalid bounds")
	}
}

func TestFinderPathResolvesMiseWithoutInheritedPATH(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, ".local", "share", "mise", "shims", "codex")
	os.MkdirAll(filepath.Dir(exe), 0700)
	os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0700)
	found, e := resolveExecutable("codex", home, func(string) (string, error) { return "", os.ErrNotExist })
	if e != nil || found != exe {
		t.Fatal(found, e)
	}
	if _, e = resolveExecutable("/missing/explicit/codex", home, func(string) (string, error) { return "", os.ErrNotExist }); e == nil {
		t.Fatal("explicit path must not silently switch installation")
	}
}
