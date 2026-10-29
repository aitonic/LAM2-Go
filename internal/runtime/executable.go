package runtime

import (
	"lam.local/lam2/internal/fault"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Finder-launched GUI applications often do not inherit the user's tool-manager
// PATH. Resolve common installation paths without evaluating shell startup files.
func ResolveExecutable(name string) (string, error) {
	home, _ := os.UserHomeDir()
	return resolveExecutable(name, home, exec.LookPath)
}
func resolveExecutable(name, home string, look func(string) (string, error)) (string, error) {
	if strings.ContainsAny(name, "\r\n\x00") || name == "" {
		return "", fault.New("CODEX_NOT_FOUND", "Invalid executable path")
	}
	if p, e := look(name); e == nil {
		return filepath.Abs(p)
	}
	if strings.ContainsAny(name, "/\\") {
		return "", fault.New("CODEX_NOT_FOUND", "The configured executable path is not available")
	}
	roots := []string{filepath.Join(home, ".local/share/mise/shims"), filepath.Join(home, ".local/bin"), filepath.Join(home, "bin"), filepath.Join(home, ".asdf/shims"), "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"}
	for _, root := range roots {
		p := filepath.Join(root, name)
		s, e := os.Stat(p)
		if e == nil && s.Mode().IsRegular() && s.Mode().Perm()&0111 != 0 {
			return p, nil
		}
	}
	return "", fault.New("CODEX_NOT_FOUND", "Install Codex or configure its absolute executable path")
}
