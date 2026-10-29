package provider

import (
	"context"
	"encoding/json"
	"lam.local/lam2/internal/fault"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProviderHealthPersistsObservationWithoutMutatingDefinition(t *testing.T) {
	ctx := context.Background()
	s, _ := setup(t)
	status := 200
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v1/models" {
			t.Error("probe attempted model generation", r.Method, r.URL.Path)
		}
		w.WriteHeader(status)
		if status == 200 {
			w.Write([]byte(`{"data":[{"id":"a"}]}`))
		} else {
			w.Write([]byte(`{"error":"do not log this private upstream body"}`))
		}
	}))
	defer up.Close()
	p := candidate()
	p.AuthKind = "none"
	p.BaseURL = up.URL + "/v1"
	st, _ := s.Store.Read(ctx)
	p, e := s.Save(ctx, p, "", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(s.Store.Path())
	r, e := s.TestConnection(ctx, p.ID)
	if e != nil || !r.OK || r.ModelCount != 1 {
		t.Fatal(r, e)
	}
	after, _ := os.ReadFile(s.Store.Path())
	if string(before) != string(after) {
		t.Fatal("health check mutated provider definition")
	}
	h, e := s.Health(ctx, p.ID)
	if e != nil || !h.OK {
		t.Fatal(h, e)
	}
	status = 429
	r, e = s.TestConnection(ctx, p.ID)
	if e == nil || r.OK {
		t.Fatal(r, e)
	}
	h, e = s.Health(ctx, p.ID)
	if e != nil || h.OK || h.ErrorCode == "" {
		t.Fatal(h, e)
	}
	raw, _ := os.ReadFile(filepath.Join(s.Store.Root, "provider-health", p.ID+".json"))
	var v map[string]any
	if json.Unmarshal(raw, &v) != nil {
		t.Fatal("invalid persisted observation")
	}
	if string(raw) == "" || len(raw) > 4096 {
		t.Fatal("unbounded observation")
	}
	p.Protocol = "chat_completions"
	st, _ = s.Store.Read(ctx)
	p, e = s.Save(ctx, p, "", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.TestConnection(ctx, p.ID); fault.Code(e) != "PROVIDER_DIRECT_ROUTE_REQUIRED" {
		t.Fatal(e)
	}
}
func TestDiscoverRejectsSuccessfulNonCatalogJSON(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"error":"unexpected success status"}`)) }))
	defer up.Close()
	s, _ := setup(t)
	p := candidate()
	p.BaseURL = up.URL
	p.AuthKind = "none"
	if _, e := s.Discover(context.Background(), p); e == nil {
		t.Fatal("arbitrary JSON was accepted as healthy model list")
	}
}
