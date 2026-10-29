package fault

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestErrorDoesNotExposeCause(t *testing.T) {
	e := Wrap("UPSTREAM_ERROR", "The provider request failed", errors.New("Authorization: Bearer very-secret"))
	b, _ := json.Marshal(e)
	if strings.Contains(e.Error()+string(b), "very-secret") {
		t.Fatal("secret exposed")
	}
	if Code(e) != "UPSTREAM_ERROR" || errors.Unwrap(e) == nil {
		t.Fatal("lost error identity")
	}
}
func TestUnknownErrorIsSanitized(t *testing.T) {
	p := Public(errors.New("key=secret"))
	if p.Code != "INTERNAL_ERROR" || strings.Contains(p.Message, "secret") {
		t.Fatal(p)
	}
}
