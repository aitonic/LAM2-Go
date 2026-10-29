package provider

import (
	"context"
	"lam.local/lam2/internal/account"
	"lam.local/lam2/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestAPIAccountCreateSwitchAndDelete(t *testing.T) {
	ctx := context.Background()
	p, _ := setup(t)
	p.Config = &editor{}
	st, _ := p.Store.Read(ctx)
	v := candidate()
	v.Models = append(v.Models, model.ProviderModel{ID: "model-b"})
	v, e := p.Save(ctx, v, "test-key", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	a := APIAccountService{Providers: p, Accounts: &account.Service{Store: p.Store}}
	plan, e := a.PlanCreate(ctx, "API account", v.ID, "model-a")
	if e != nil {
		t.Fatal(e)
	}
	got, e := a.ExecuteCreate(ctx, plan.ID, plan.Fingerprint)
	if e != nil {
		t.Fatal(e)
	}
	if got.Binding.Model != "model-a" || !got.Profile.Managed {
		t.Fatal(got)
	}
	again, e := a.ExecuteCreate(ctx, plan.ID, plan.Fingerprint)
	if e != nil || again.Profile.ID != got.Profile.ID {
		t.Fatal(again, e)
	}
	switchPlan, e := a.PlanModelSwitch(ctx, got.Profile.ID, "model-b")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = p.CommitBind(ctx, switchPlan.ID, switchPlan.Fingerprint); e != nil {
		t.Fatal(e)
	}
	got, e = a.Connection(ctx, got.Profile.ID)
	if e != nil || got.Binding.Model != "model-b" {
		t.Fatal(got, e)
	}
	if e = a.Delete(ctx, got.Profile.ID, true); e != nil {
		t.Fatal(e)
	}
	st, _ = p.Store.Read(ctx)
	if _, ok := st.Profiles[got.Profile.ID]; ok {
		t.Fatal("profile retained")
	}
	if _, ok := st.Providers[v.ID]; !ok {
		t.Fatal("shared provider removed")
	}
	if _, e = os.Stat(filepath.Join(got.Profile.Home, ".managed-by-lam2.json")); !os.IsNotExist(e) {
		t.Fatal("owned home not removed from active location", e)
	}
}
func TestAPIAccountBindingFailureDoesNotLeaveHalfAccount(t *testing.T) {
	ctx := context.Background()
	p, _ := setup(t)
	p.Config = &editor{fail: true}
	st, _ := p.Store.Read(ctx)
	v, e := p.Save(ctx, candidate(), "test", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	a := APIAccountService{Providers: p, Accounts: &account.Service{Store: p.Store}}
	plan, e := a.PlanCreate(ctx, "API", v.ID, "model-a")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.ExecuteCreate(ctx, plan.ID, plan.Fingerprint); e == nil {
		t.Fatal("failed RPC hidden")
	}
	st, _ = p.Store.Read(ctx)
	for _, p := range st.Profiles {
		if p.Name == "API" {
			t.Fatal("half-created account")
		}
	}
}

func TestAPIAccountFailedDeletionDoesNotDetachBinding(t *testing.T) {
	ctx := context.Background()
	p, _ := setup(t)
	p.Config = &editor{}
	st, _ := p.Store.Read(ctx)
	v, e := p.Save(ctx, candidate(), "test-key", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	a := APIAccountService{Providers: p, Accounts: &account.Service{Store: p.Store}}
	plan, e := a.PlanCreate(ctx, "owned", v.ID, "model-a")
	if e != nil {
		t.Fatal(e)
	}
	conn, e := a.ExecuteCreate(ctx, plan.ID, plan.Fingerprint)
	if e != nil {
		t.Fatal(e)
	}
	os.Remove(filepath.Join(conn.Profile.Home, ".managed-by-lam2.json"))
	if e = a.Delete(ctx, conn.Profile.ID, true); e == nil {
		t.Fatal("missing ownership accepted")
	}
	if _, e = a.Connection(ctx, conn.Profile.ID); e != nil {
		t.Fatal("failed deletion silently detached binding", e)
	}
}
