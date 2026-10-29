// Package mygosecrets uses the framework keychain directly, including headless modes.
package mygosecrets

import (
	"context"
	"github.com/egoist/mygo"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"strings"
)

type Secrets struct{}

var _ storage.Secrets = Secrets{}

func valid(ref model.CredentialRef) error {
	if ref.Service == "" || ref.Account == "" || strings.ContainsRune(ref.Service+ref.Account, 0) {
		return fault.New("CREDENTIAL_REFERENCE_INVALID", "Keychain service and account are required")
	}
	return nil
}
func (Secrets) Get(ctx context.Context, r model.CredentialRef) (string, error) {
	if e := ctx.Err(); e != nil {
		return "", e
	}
	if e := valid(r); e != nil {
		return "", e
	}
	b, e := mygo.Secrets.Service(r.Service).Get(r.Account)
	if e != nil {
		return "", fault.Wrap("CREDENTIAL_UNAVAILABLE", "Keychain access failed or the secret is missing", e)
	}
	return string(b), nil
}
func (Secrets) Set(ctx context.Context, r model.CredentialRef, value string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := valid(r); e != nil {
		return e
	}
	if len(value) > 2560 {
		return fault.New("CREDENTIAL_TOO_LARGE", "Use the encrypted vault for long credential documents")
	}
	if e := mygo.Secrets.Service(r.Service).Set(r.Account, []byte(value)); e != nil {
		return fault.Wrap("CREDENTIAL_UNAVAILABLE", "Keychain write failed", e)
	}
	return nil
}
func (Secrets) Delete(ctx context.Context, r model.CredentialRef) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := valid(r); e != nil {
		return e
	}
	if e := mygo.Secrets.Service(r.Service).Delete(r.Account); e != nil {
		return fault.Wrap("CREDENTIAL_UNAVAILABLE", "Keychain deletion failed", e)
	}
	return nil
}
