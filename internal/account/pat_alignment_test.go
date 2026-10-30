package account

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPATAlignment_ExportCPA(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()

	p, err := s.Create(ctx, "Luna", "pat", 1)
	if err != nil {
		t.Fatalf("failed to create profile: %v", err)
	}

	sessionJSON := `{"tokens":{"access_token":"sec-token-123","account_id":"acc-456","refresh_token":"ref-789"},"plan_type":"pro","email":"luna@example.com"}`
	normalized, err := NormalizePAT([]byte(sessionJSON), "pat-secret-token", "")
	if err != nil {
		t.Fatalf("NormalizePAT failed: %v", err)
	}

	if err := s.ImportAuth(ctx, p.ID, normalized, true); err != nil {
		t.Fatalf("ImportAuth failed: %v", err)
	}

	cpaBytes, err := s.ExportCPA(ctx, p.ID, true)
	if err != nil {
		t.Fatalf("ExportCPA failed: %v", err)
	}

	var content map[string]any
	if err := json.Unmarshal(cpaBytes, &content); err != nil {
		t.Fatalf("invalid CPA JSON: %v", err)
	}

	headers, ok := content["headers"].(map[string]any)
	if !ok {
		t.Fatalf("expected headers in CPA export, got %v", content["headers"])
	}
	if headers["authorization"] != "Bearer pat-secret-token" {
		t.Errorf("expected Bearer pat-secret-token, got %v", headers["authorization"])
	}
	if content["access_token"] != "sec-token-123" {
		t.Errorf("expected access_token sec-token-123, got %v", content["access_token"])
	}
	if content["email"] != "luna@example.com" {
		t.Errorf("expected email luna@example.com, got %v", content["email"])
	}
	_ = st
}

func TestPATAlignment_ActivateDiscoveredPAT(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()

	// Create an external discovered directory ~/.codex-discovered
	homeDir := filepath.Dir(s.SharedHome)
	discoveredDir := filepath.Join(homeDir, ".codex-discovered")
	if err := os.MkdirAll(discoveredDir, 0700); err != nil {
		t.Fatal(err)
	}
	discoveredAuth := `{"personal_access_token":"discovered-pat-123"}`
	if err := os.WriteFile(filepath.Join(discoveredDir, "auth.json"), []byte(discoveredAuth), 0600); err != nil {
		t.Fatal(err)
	}

	// Activate discovered account "discovered"
	if err := s.ActivatePAT(ctx, "discovered", true); err != nil {
		t.Fatalf("failed to activate discovered PAT: %v", err)
	}

	// Verify shared ~/.codex/auth.json contains the discovered auth
	sharedAuthBytes, err := os.ReadFile(filepath.Join(s.SharedHome, "auth.json"))
	if err != nil {
		t.Fatalf("failed to read shared auth.json: %v", err)
	}
	if string(sharedAuthBytes) != discoveredAuth {
		t.Errorf("expected shared auth to match discovered auth, got %s", string(sharedAuthBytes))
	}

	// Verify DetectActivePAT detects it
	activeID, err := s.DetectActivePAT(ctx)
	if err != nil {
		t.Fatalf("DetectActivePAT failed: %v", err)
	}
	if activeID != "discovered" {
		t.Errorf("expected activeID to be 'discovered', got %s", activeID)
	}
}

func TestPATAlignment_DetectActivePAT(t *testing.T) {
	s, st := fixture(t)
	ctx := context.Background()

	p, err := s.Create(ctx, "Alpha", "pat", 1)
	if err != nil {
		t.Fatal(err)
	}

	authJSON := `{"tokens":{"account_id":"user-abcd1234"},"personal_access_token":"alpha-token"}`
	normalized, err := NormalizePAT([]byte(authJSON), "alpha-token", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ImportAuth(ctx, p.ID, normalized, true); err != nil {
		t.Fatal(err)
	}

	// Before activation, DetectActivePAT should return empty
	id, err := s.DetectActivePAT(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if id != "" {
		t.Errorf("expected empty active ID, got %s", id)
	}

	// Activate PAT
	if err := s.ActivatePAT(ctx, p.ID, true); err != nil {
		t.Fatal(err)
	}

	// After activation, DetectActivePAT should detect p.ID
	id, err = s.DetectActivePAT(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if id != p.ID {
		t.Errorf("expected detected active ID %s, got %s", p.ID, id)
	}

	// Restore PAT
	if err := s.RestorePAT(ctx, true); err != nil {
		t.Fatal(err)
	}
	id, err = s.DetectActivePAT(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if id != "" {
		t.Errorf("expected empty active ID after restore, got %s", id)
	}
	_ = st
}
