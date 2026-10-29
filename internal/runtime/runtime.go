// Package runtime builds explicit argv/environment plans for external programs.
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"lam.local/lam2/internal/account"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type LaunchOptions struct {
	Resume      string
	CWD         string
	Extra       []string
	Environment map[string]string
}
type LaunchPlan struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
	CWD        string   `json:"cwd"`
	Env        []string `json:"-"`
	ProfileID  string   `json:"profileId"`
}

func EnvName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
func Environment(base []string, p model.Profile, extra map[string]string) ([]string, error) {
	if p.Proxy.Mode == "" {
		p.Proxy.Mode = "inherit"
	}
	if err := account.ValidateProxy(p.Proxy); err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, entry := range base {
		k, v, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		switch strings.ToUpper(k) {
		case "CODEX_HOME", "CODEX_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "LAM_PROVIDER_HEADER_TOKEN":
			continue
		case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY":
			if p.Proxy.Mode != "inherit" {
				continue
			}
		}
		m[k] = v
	}
	m["CODEX_HOME"] = p.Home
	if p.Proxy.Mode == "manual" {
		for k, v := range map[string]string{"HTTP_PROXY": p.Proxy.HTTP, "HTTPS_PROXY": p.Proxy.HTTPS, "ALL_PROXY": p.Proxy.All, "NO_PROXY": p.Proxy.NoProxy} {
			if v != "" {
				m[k] = v
				m[strings.ToLower(k)] = v
			}
		}
	}
	for k, v := range extra {
		if !EnvName(k) || k == "CODEX_HOME" || strings.ContainsRune(v, 0) {
			return nil, fault.New("INVALID_ENVIRONMENT", "Invalid environment override")
		}
		m[k] = v
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+m[k])
	}
	return out, nil
}
func Plan(p model.Profile, s model.Settings, o LaunchOptions) (LaunchPlan, error) {
	var plan LaunchPlan
	if p.Home == "" || !filepath.IsAbs(p.Home) {
		return plan, fault.New("PROFILE_HOME_INVALID", "An absolute Codex home is required")
	}
	if err := storage.RejectSymlinks(p.Home); err != nil {
		return plan, err
	}
	bin, err := ResolveExecutable(s.CodexPath)
	if err != nil {
		return plan, fault.New("CODEX_NOT_FOUND", "Configure the path to an installed Codex CLI")
	}
	bin, err = filepath.Abs(bin)
	if err != nil {
		return plan, err
	}
	if o.Resume != "" && (!storage.ValidID(o.Resume) || strings.HasPrefix(o.Resume, "-")) {
		return plan, fault.New("SESSION_ID_INVALID", "Invalid resume session identifier")
	}
	if !oneOf(s.ApprovalPolicy, "on-request", "untrusted", "never", "on-failure") || !oneOf(s.SandboxMode, "read-only", "workspace-write", "danger-full-access") {
		return plan, fault.New("INVALID_LAUNCH_POLICY", "Invalid CLI execution policy")
	}
	cwd := o.CWD
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return plan, err
		}
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return plan, err
	}
	info, err := os.Stat(cwd)
	if err != nil || !info.IsDir() {
		return plan, fault.New("WORKSPACE_NOT_FOUND", "The selected workspace does not exist")
	}
	env, err := Environment(os.Environ(), p, o.Environment)
	if err != nil {
		return plan, err
	}
	args := []string{"--ask-for-approval", s.ApprovalPolicy, "--sandbox", s.SandboxMode}
	if o.Resume != "" {
		args = append(args, "resume", o.Resume)
	}
	args = append(args, o.Extra...)
	return LaunchPlan{Executable: bin, Args: args, CWD: cwd, Env: env, ProfileID: p.ID}, nil
}
func oneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func Run(ctx context.Context, plan LaunchPlan, in io.Reader, out, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, plan.Executable, plan.Args...)
	cmd.Dir = plan.CWD
	cmd.Env = plan.Env
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fault.Wrap("CODEX_EXITED", "Codex exited unsuccessfully", err)
	}
	return nil
}
func ShellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func WriteWrapper(ctx context.Context, path, exe, home, profileID string) error {
	if !filepath.IsAbs(exe) || !filepath.IsAbs(home) || !storage.ValidID(profileID) {
		return fault.New("WRAPPER_INVALID", "A wrapper needs absolute paths and a profile ID")
	}
	script := "#!/bin/sh\nexec " + ShellQuote(exe) + " --home " + ShellQuote(home) + " launch --profile " + ShellQuote(profileID) + " -- \"$@\"\n"
	return storage.AtomicWrite(ctx, path, []byte(script), 0700)
}

// OpenTerminal delegates UI/PTY ownership to an existing external terminal.
// Embedded terminals are provided separately by MyGo's official terminal plugin.
func OpenTerminal(ctx context.Context, kind, exe, home, profileID, resume, cwd string, extra ...string) error {
	args, err := launcherArguments(exe, home, profileID, resume, cwd)
	if err != nil {
		return err
	}
	if len(extra) > 0 {
		args = append(args, "--")
		args = append(args, extra...)
	}
	parts := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsRune(a, 0) {
			return fault.New("LAUNCH_INVALID", "Argument contains NUL")
		}
		parts[i] = ShellQuote(a)
	}
	line := strings.Join(parts, " ")
	var cmd *exec.Cmd
	switch kind {
	case "terminal":
		quoted, _ := json.Marshal(line)
		cmd = exec.CommandContext(ctx, "/usr/bin/osascript", "-e", `tell application "Terminal" to activate`, "-e", "tell application \"Terminal\" to do script "+string(quoted))
	case "ghostty":
		cmd = exec.CommandContext(ctx, "/usr/bin/open", append([]string{"-na", "Ghostty", "--args", "-e"}, args...)...)
	case "cmux":
		return openCMUX(ctx, line)
	case "codex_app":
		return fault.New("CODEX_APP_HANDOFF_UNSUPPORTED", "Codex.app has no verified handoff API; choose a terminal or copy the resume command")
	default:
		return fault.New("TERMINAL_UNSUPPORTED", "Choose Terminal, Ghostty, cmux or the MyGo embedded terminal")
	}
	if err := cmd.Run(); err != nil {
		return fault.Wrap("TERMINAL_LAUNCH_FAILED", fmt.Sprintf("The %s terminal could not be opened", kind), err)
	}
	return nil
}

// SDKOverlay differs from a replacement environment: SpawnOptions.Env appends
// to the inherited environment. Explicit empty entries remove inherited keys.
func SDKOverlay(base []string, p model.Profile) ([]string, error) {
	env, err := Environment(base, p, nil)
	if err != nil {
		return nil, err
	}
	clear := []string{"CODEX_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "LAM_PROVIDER_HEADER_TOKEN"}
	if p.Proxy.Mode != "" && p.Proxy.Mode != "inherit" {
		clear = append(clear, "HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy")
	}
	prefix := make([]string, 0, len(clear)+len(env))
	for _, k := range clear {
		prefix = append(prefix, k+"=")
	}
	return append(prefix, env...), nil
}
