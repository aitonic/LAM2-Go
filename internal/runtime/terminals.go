package runtime

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/storage"
)

type TerminalTarget struct {
	ID, Name                    string
	Installed, HandoffSupported bool
}

func DetectTerminalTargets(appRoots []string, lookPath func(string) (string, error)) []TerminalTarget {
	installed := func(names []string, cmd string) bool {
		if _, e := lookPath(cmd); e == nil {
			return true
		}
		for _, root := range appRoots {
			for _, name := range names {
				if s, e := os.Stat(filepath.Join(root, name+".app")); e == nil && s.IsDir() {
					return true
				}
			}
		}
		return false
	}
	return []TerminalTarget{{"terminal", "Terminal.app", true, true}, {"ghostty", "Ghostty", installed([]string{"Ghostty"}, "ghostty"), true}, {"cmux", "cmux", installed([]string{"cmux", "Cmux"}, "cmux"), true}, {"codex_app", "Codex.app", installed([]string{"Codex"}, "codex-app"), false}, {"embedded", "MyGo terminal", true, true}}
}
func TerminalTargets() []TerminalTarget {
	h, _ := os.UserHomeDir()
	return DetectTerminalTargets([]string{"/Applications", filepath.Join(h, "Applications")}, exec.LookPath)
}
func CMUXArguments(line string) []string {
	return []string{"workspace", "create", "--name", "LAM Handoff", "--command", line, "--focus", "true"}
}
func CMUXFallbackAllowed(detail string) bool {
	s := strings.ToLower(detail)
	for _, term := range []string{"access denied", "only processes started inside cmux", "broken pipe", "failed to write to socket"} {
		if strings.Contains(s, term) {
			return true
		}
	}
	return false
}
func CMUXScript(line string) string {
	q, _ := json.Marshal(line)
	return "tell application \"cmux\"\nactivate\nset targetTab to new tab\ndelay 0.2\nset targetTerminal to focused terminal of targetTab\ninput text (" + string(q) + " & return) to targetTerminal\nend tell"
}
func launcherArguments(exe, home, id, resume, cwd string) ([]string, error) {
	if !storage.ValidID(id) || !filepath.IsAbs(exe) || !filepath.IsAbs(home) || strings.ContainsAny(exe+home+cwd, "\x00\r\n") {
		return nil, fault.New("LAUNCH_INVALID", "Invalid launcher identity")
	}
	args := []string{exe, "--home", home, "launch", "--profile", id}
	if resume != "" {
		if !storage.ValidID(resume) || strings.HasPrefix(resume, "-") {
			return nil, fault.New("SESSION_ID_INVALID", "Invalid resume identifier")
		}
		args = append(args, "--resume", resume)
	}
	if cwd != "" {
		if !filepath.IsAbs(cwd) {
			return nil, fault.New("WORKSPACE_NOT_FOUND", "Use an absolute workspace path")
		}
		args = append(args, "--cwd", cwd)
	}
	return args, nil
}
func LauncherCommand(exe, home, id, resume, cwd string) (string, error) {
	args, e := launcherArguments(exe, home, id, resume, cwd)
	if e != nil {
		return "", e
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = ShellQuote(a)
	}
	return strings.Join(parts, " "), nil
}
func cmuxBinary() (string, error) {
	h, _ := os.UserHomeDir()
	for _, root := range []string{"/Applications", filepath.Join(h, "Applications")} {
		for _, name := range []string{"cmux", "Cmux"} {
			for _, rel := range []string{"Contents/Resources/bin/cmux", "Contents/MacOS/cmux"} {
				p := filepath.Join(root, name+".app", rel)
				if s, e := os.Stat(p); e == nil && !s.IsDir() && s.Mode()&0111 != 0 {
					return p, nil
				}
			}
		}
	}
	return exec.LookPath("cmux")
}

type terminalOutput struct{ data []byte }

func (b *terminalOutput) Write(v []byte) (int, error) {
	n := len(v)
	remaining := 8192 - len(b.data)
	if remaining > 0 {
		if len(v) > remaining {
			v = v[:remaining]
		}
		b.data = append(b.data, v...)
	}
	return n, nil
}
func openCMUX(ctx context.Context, line string) error {
	bin, e := cmuxBinary()
	if e != nil {
		return fault.New("TERMINAL_NOT_FOUND", "Install cmux or select another terminal")
	}
	call, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(call, bin, CMUXArguments(line)...)
	cmd.Env = append(os.Environ(), "CMUX_QUIET=1")
	var output terminalOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	if e = cmd.Run(); e == nil {
		return nil
	}
	if !CMUXFallbackAllowed(string(output.data)) {
		return fault.Wrap("TERMINAL_LAUNCH_FAILED", "cmux rejected the workspace command; check its CLI version", e)
	}
	// Restrict fallback to the same socket/permission cases as the original LAM.
	if e = exec.CommandContext(call, "/usr/bin/osascript", "-e", CMUXScript(line)).Run(); e != nil {
		return fault.Wrap("TERMINAL_PERMISSION_DENIED", "Allow LAM to automate cmux in macOS settings, or select another terminal", e)
	}
	return nil
}
