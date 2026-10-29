package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"lam.local/lam2/internal/fault"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const MissingHash = "missing"

func Hash(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func NewID() string {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		panic("system random source failed")
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
func ValidID(id string) bool {
	if id == "" || len(id) > 128 || id == "." || id == ".." {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

// SafeJoin refuses symlinks in existing components, including the root itself.
func SafeJoin(root, relative string) (string, error) {
	if root == "" || filepath.IsAbs(relative) {
		return "", fault.New("PATH_UNSAFE", "An absolute relative-path is not allowed")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(relative)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fault.New("PATH_UNSAFE", "Path escapes its allowed root")
	}
	p := filepath.Join(root, clean)
	if err := RejectSymlinks(p); err != nil {
		return "", err
	}
	return p, nil
}
func RejectSymlinks(path string) error {
	p, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	vol := filepath.VolumeName(p)
	cur := vol + string(os.PathSeparator)
	for _, part := range strings.Split(strings.TrimPrefix(p, cur), string(os.PathSeparator)) {
		if part == "" {
			continue
		}
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fault.Wrap("PATH_UNREADABLE", "Cannot inspect a path component", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// macOS owns these immutable root aliases. No user-controlled link
			// below them is trusted; the walk continues for every component.
			if runtime.GOOS == "darwin" {
				expected := map[string]string{"/var": "private/var", "/tmp": "private/tmp", "/etc": "private/etc"}[cur]
				target, e := os.Readlink(cur)
				if expected != "" && e == nil && (target == expected || target == "/"+expected) {
					continue
				}
			}
			return fault.New("PATH_UNSAFE", "Symbolic links are not accepted for managed data")
		}
	}
	return nil
}
func Within(path string, roots []string) bool {
	p, e := filepath.Abs(path)
	if e != nil {
		return false
	}
	for _, root := range roots {
		r, e := filepath.Abs(root)
		if e != nil {
			continue
		}
		rel, e := filepath.Rel(r, p)
		if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}
func EnsurePrivateDir(path string) error {
	if err := RejectSymlinks(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return fault.Wrap("DIRECTORY_UNWRITABLE", "Cannot create the private application directory", err)
	}
	return os.Chmod(path, 0700)
}
func ReadFile(path string, maxBytes int64) ([]byte, error) {
	if err := RejectSymlinks(path); err != nil {
		return nil, err
	}
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fault.New("PATH_UNSAFE", "Expected a regular file")
	}
	if before.Size() > maxBytes {
		return nil, fault.New("FILE_TOO_LARGE", "The file exceeds the operation size limit")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		return nil, fault.New("FILE_CONFLICT", "The file changed while it was opened")
	}
	b, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if int64(len(b)) > maxBytes {
		return nil, fault.New("FILE_TOO_LARGE", "The file grew beyond the size limit")
	}
	return b, err
}
func FileHash(path string) (string, error) {
	if err := RejectSymlinks(path); err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return MissingHash, nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return "", fault.New("PATH_UNSAFE", "Expected a regular file")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func SyncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func AtomicWrite(ctx context.Context, path string, data []byte, mode fs.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := RejectSymlinks(path); err != nil {
		return err
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(parent, ".lam-write-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if mode == 0 {
		mode = 0600
	}
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = RejectSymlinks(path); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	return SyncDir(parent)
}
func ResolveHome(value string) (string, error) {
	if value == "" {
		value = os.Getenv("LAM_HOME")
	}
	if value == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		value = filepath.Join(home, ".lam-next")
	}
	p, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	if filepath.Base(p) == ".lam" {
		return "", fault.New("LEGACY_HOME_READ_ONLY", "Use a separate LAM_HOME; the legacy .lam directory is read-only")
	}
	if err = RejectSymlinks(p); err != nil {
		return "", err
	}
	return p, nil
}
