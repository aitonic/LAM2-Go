// Package testutil is used exclusively by tests. It is not an application backend.
package testutil

import (
	"context"
	"errors"
	"lam.local/lam2/internal/model"
	"sync"
)

type Secrets struct {
	mu     sync.Mutex
	Values map[string]string
	Fail   bool
}

func (s *Secrets) Get(_ context.Context, r model.CredentialRef) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Fail {
		return "", errors.New("secret store denied")
	}
	v, ok := s.Values[r.Service+"/"+r.Account]
	if !ok {
		return "", errors.New("missing secret")
	}
	return v, nil
}
func (s *Secrets) Set(_ context.Context, r model.CredentialRef, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Fail {
		return errors.New("secret store denied")
	}
	if s.Values == nil {
		s.Values = map[string]string{}
	}
	s.Values[r.Service+"/"+r.Account] = v
	return nil
}
func (s *Secrets) Delete(_ context.Context, r model.CredentialRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Values, r.Service+"/"+r.Account)
	return nil
}
