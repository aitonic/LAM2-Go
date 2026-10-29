package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This test enforces the clean Go application tree on both the complete
// repository and the offline domain-test staging tree.
func TestNoRustReactTauriApplicationSource(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]bool{"Cargo.toml": true, "Cargo.lock": true, "package.json": true, "tauri.conf.json": true}
	extensions := []string{".rs", ".tsx", ".jsx", ".ts", ".js"}
	err = filepath.Walk(root, func(p string, info os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "build" || info.Name() == "reports" {
				return filepath.SkipDir
			}
			if info.Name() == "src-tauri" {
				t.Errorf("deprecated Tauri directory: %s", p)
			}
			return nil
		}
		if forbidden[info.Name()] {
			t.Errorf("forbidden legacy build artifact: %s", p)
		}
		for _, ext := range extensions {
			if strings.HasSuffix(p, ext) {
				t.Errorf("forbidden legacy application source: %s", p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
