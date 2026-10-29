package settings

import (
	"lam.local/lam2/internal/model"
	"testing"
)

func TestVisualSettingsAvailabilityAndCardLayoutPersistable(t *testing.T) {
	s := model.DefaultSettings()
	for _, mode := range []string{"profile", "pat", "both"} {
		s.ModeAvailability = mode
		if e := Validate(s); e != nil {
			t.Fatalf("%s should be valid: %v", mode, e)
		}
	}
	s.ExpandedCardActions = true
	if e := Validate(s); e != nil {
		t.Fatalf("expanded account cards rejected: %v", e)
	}
	s.ModeAvailability = "invalid"
	if e := Validate(s); e == nil {
		t.Fatal("unknown availability accepted")
	}
}
