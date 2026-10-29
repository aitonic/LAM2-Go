package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTokenAccounting(t *testing.T) {
	v := Tokens{Input: 100, CachedInput: 40, Output: 25, ReasoningOutput: 10}
	if v.Uncached() != 60 || v.Total() != 125 {
		t.Fatal(v)
	}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, v := range []Tokens{{Input: -1}, {Input: 3, CachedInput: 4}, {Output: 1, ReasoningOutput: 2}} {
		if v.Validate() == nil {
			t.Fatalf("accepted invalid tokens: %+v", v)
		}
	}
}
func TestStateAndSecretReference(t *testing.T) {
	s := NewState()
	if s.SchemaVersion != SchemaVersion || s.Settings.Gateway.MaxConcurrent != 32 {
		t.Fatal(s)
	}
	s.Providers["p"] = Provider{ID: "p", Credential: CredentialRef{Kind: "keychain", Service: "lam.remote-provider", Account: "p-v1", Version: 1}}
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), "apiKey") {
		t.Fatal("plaintext credential field")
	}
}
func TestPriceIsVersionedAndUnknownIsNotFree(t *testing.T) {
	tok := Tokens{Input: 100, CachedInput: 40, Output: 10}
	p := RateCard{Model: "m", Version: "2026-10-27", Currency: "USD", InputPerMillion: 2, CachedPerMillion: 1, OutputPerMillion: 8}
	cost, err := p.Cost(tok)
	if err != nil || cost < 0.0002399 || cost > 0.0002401 {
		t.Fatal(cost, err)
	}
	if _, err := (RateCard{}).Cost(tok); err == nil {
		t.Fatal("unknown price treated as free")
	}
}
