package settings

import (
	"context"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"testing"
)

func TestSettingsValidationAndPersistence(t *testing.T) {
	s, _ := storage.Open(t.TempDir())
	svc := Service{Store: s}
	v := model.DefaultSettings()
	v.Theme = "dark"
	if err := svc.Save(context.Background(), v, 1); err != nil {
		t.Fatal(err)
	}
	v.RefreshSeconds = 0
	if err := svc.Save(context.Background(), v, 2); err == nil {
		t.Fatal("invalid timer accepted")
	}
	st, _ := s.Read(context.Background())
	if st.Revision != 2 || st.Settings.RefreshSeconds != 180 {
		t.Fatal(st.Settings)
	}
}
func TestInvalidGatewayLimits(t *testing.T) {
	v := model.DefaultSettings()
	v.Gateway.PerBinding = 100
	if err := Validate(v); err == nil {
		t.Fatal("per binding exceeds global limit")
	}
	v = model.DefaultSettings()
	v.ApprovalPolicy = "always bypass everything"
	if err := Validate(v); err == nil {
		t.Fatal("invalid approval policy")
	}
}

func TestRuntimeUpdateFeedCannotPretendToConfigureNativeUpdater(t *testing.T) {
	v := model.DefaultSettings()
	v.UpdateURL = "https://updates.example.test/feed"
	v.UpdatePublicKey = "untrusted"
	if err := Validate(v); err == nil {
		t.Fatal("runtime feed accepted although updater uses signed build configuration")
	}
}

func TestLegacyGatewayTimeoutRanges(t *testing.T) {
	for _, pair := range [][2]int{{10, 300}, {600, 3600}, {600, 300}} {
		v := model.DefaultSettings()
		v.Gateway.FirstResponseSeconds = pair[0]
		v.Gateway.RequestSeconds = pair[1]
		if e := Validate(v); e != nil {
			t.Fatalf("legacy accepted pair %v: %v", pair, e)
		}
	}
	for _, pair := range [][2]int{{9, 1200}, {601, 1200}, {60, 299}, {60, 3601}} {
		v := model.DefaultSettings()
		v.Gateway.FirstResponseSeconds = pair[0]
		v.Gateway.RequestSeconds = pair[1]
		if Validate(v) == nil {
			t.Fatalf("out of legacy range accepted: %v", pair)
		}
	}
}
