package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"lam.local/lam2/internal/fault"
	"os"
	"path/filepath"
)

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

// CopyChecked streams a file, verifies source identity/content and CASes destination.
// It never copies directory contents or follows symbolic links.
func CopyChecked(ctx context.Context, source, destination, sourceHash, destinationHash string) (string, error) {
	if err := RejectSymlinks(source); err != nil {
		return "", err
	}
	if err := RejectSymlinks(destination); err != nil {
		return "", err
	}
	st, err := os.Lstat(source)
	if err != nil || !st.Mode().IsRegular() {
		return "", fault.New("SOURCE_INVALID", "The source is not a regular file")
	}
	src, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer src.Close()
	opened, err := src.Stat()
	if err != nil || !os.SameFile(st, opened) {
		return "", fault.New("FILE_CONFLICT", "Source identity changed")
	}
	parent := filepath.Dir(destination)
	if err = os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(parent, ".lam-copy-*")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	defer os.Remove(name)
	defer tmp.Close()
	if err = tmp.Chmod(0600); err != nil {
		return "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), contextReader{ctx, src})
	if err != nil {
		return "", err
	}
	digest := hex.EncodeToString(h.Sum(nil))
	after, err := src.Stat()
	if err != nil || n != st.Size() || after.Size() != st.Size() || digest != sourceHash {
		return "", fault.New("FILE_CONFLICT", "The source changed during copying")
	}
	if err = tmp.Sync(); err != nil {
		return "", err
	}
	if err = tmp.Close(); err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	current, err := FileHash(destination)
	if err != nil {
		return "", err
	}
	if current != destinationHash {
		return "", fault.New("FILE_CONFLICT", "The destination changed during copying")
	}
	if err = os.Rename(name, destination); err != nil {
		return "", err
	}
	if err = SyncDir(parent); err != nil {
		return "", err
	}
	return digest, nil
}
