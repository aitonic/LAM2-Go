package storage

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"os"
	"path/filepath"
)

type Secrets interface {
	Get(context.Context, model.CredentialRef) (string, error)
	Set(context.Context, model.CredentialRef, string) error
	Delete(context.Context, model.CredentialRef) error
}
type Vault struct {
	Root    string
	Secrets Secrets
}
type encryptedRecord struct {
	Version    int                 `json:"version"`
	Key        model.CredentialRef `json:"key"`
	Nonce      string              `json:"nonce"`
	Ciphertext string              `json:"ciphertext"`
}

func (v *Vault) path(id string) (string, error) {
	if !ValidID(id) {
		return "", fault.New("INVALID_ID", "Invalid credential identifier")
	}
	return SafeJoin(v.Root, filepath.Join("credentials", id+".enc"))
}
func (v *Vault) Put(ctx context.Context, id string, data []byte) error {
	p, err := v.path(id)
	if err != nil {
		return err
	}
	if len(data) > 4<<20 {
		return fault.New("CREDENTIAL_TOO_LARGE", "Credential import exceeds the limit")
	}
	if v.Secrets == nil {
		return fault.New("KEYCHAIN_UNAVAILABLE", "A system secret store is required")
	}
	key := make([]byte, 32)
	if _, err = io.ReadFull(rand.Reader, key); err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ref := model.CredentialRef{Kind: "keychain", Service: "lam2.vault", Account: id + "-" + NewID(), Version: 1}
	if err = v.Secrets.Set(ctx, ref, base64.StdEncoding.EncodeToString(key)); err != nil {
		return fault.Wrap("KEYCHAIN_UNAVAILABLE", "Cannot protect the imported credential", err)
	}
	old, _ := ReadFile(p, 6<<20)
	record := encryptedRecord{1, ref, base64.StdEncoding.EncodeToString(nonce), base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, data, []byte(id)))}
	out, _ := json.Marshal(record)
	if err = AtomicWrite(ctx, p, out, 0600); err != nil {
		_ = v.Secrets.Delete(context.Background(), ref)
		return err
	}
	var previous encryptedRecord
	if json.Unmarshal(old, &previous) == nil && previous.Key.Account != "" {
		_ = v.Secrets.Delete(context.Background(), previous.Key)
	}
	return nil
}
func (v *Vault) Get(ctx context.Context, id string) ([]byte, error) {
	p, err := v.path(id)
	if err != nil {
		return nil, err
	}
	b, err := ReadFile(p, 6<<20)
	if err != nil {
		return nil, fault.Wrap("CREDENTIAL_MISSING", "The encrypted credential is unavailable", err)
	}
	var r encryptedRecord
	if json.Unmarshal(b, &r) != nil || r.Version != 1 || r.Key.Service != "lam2.vault" {
		return nil, fault.New("CREDENTIAL_CORRUPT", "Invalid encrypted credential")
	}
	if v.Secrets == nil {
		return nil, fault.New("KEYCHAIN_UNAVAILABLE", "The secret store is unavailable")
	}
	encoded, err := v.Secrets.Get(ctx, r.Key)
	if err != nil {
		return nil, fault.Wrap("KEYCHAIN_UNAVAILABLE", "Cannot unlock the credential", err)
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fault.New("CREDENTIAL_CORRUPT", "Invalid encryption key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fault.New("CREDENTIAL_CORRUPT", "Invalid encryption key")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce, e1 := base64.StdEncoding.DecodeString(r.Nonce)
	ct, e2 := base64.StdEncoding.DecodeString(r.Ciphertext)
	if e1 != nil || e2 != nil || len(nonce) != gcm.NonceSize() {
		return nil, fault.New("CREDENTIAL_CORRUPT", "Invalid encrypted credential")
	}
	plain, err := gcm.Open(nil, nonce, ct, []byte(id))
	if err != nil {
		return nil, fault.New("CREDENTIAL_CORRUPT", "Credential authenticity verification failed")
	}
	return plain, nil
}
func (v *Vault) Delete(ctx context.Context, id string) error {
	p, err := v.path(id)
	if err != nil {
		return err
	}
	b, err := ReadFile(p, 6<<20)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var r encryptedRecord
	if err = json.Unmarshal(b, &r); err != nil {
		return err
	}
	if err = os.Remove(p); err != nil {
		return err
	}
	if v.Secrets != nil {
		return v.Secrets.Delete(ctx, r.Key)
	}
	return nil
}
