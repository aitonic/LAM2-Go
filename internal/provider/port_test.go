package provider

import (
	"context"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"net"
	"testing"
)

func TestGatewayPortPreviewCommitAndOccupiedPort(t *testing.T) {
	ctx := context.Background()
	s, p := setup(t)
	s.Config = &editor{}
	s.Gateway = portGateway{}
	s.BeforePortChange = func(context.Context) error { return nil }
	st, _ := s.Store.Read(ctx)
	v := candidate()
	v.Protocol = "chat_completions"
	v.Capabilities.Streaming = "supported"
	v.Capabilities.FunctionTools = "supported"
	pr, e := s.Save(ctx, v, "key", st.Revision)
	if e != nil {
		t.Fatal(e)
	}
	bplan, e := s.PlanBind(ctx, p.ID, pr.ID, pr.DefaultModel)
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.CommitBind(ctx, bplan.ID, bplan.Fingerprint)
	if e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	plan, e := s.PlanPortChange(ctx, port)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.CommitPortChange(ctx, plan.ID, plan.Fingerprint); fault.Code(e) != "GATEWAY_PORT_BUSY" {
		t.Fatal(e)
	}
	listener.Close()
	if e = s.CommitPortChange(ctx, plan.ID, plan.Fingerprint); e != nil {
		t.Fatal(e)
	}
	st, _ = s.Store.Read(ctx)
	next := st.Bindings[p.ID]
	if next.GatewayURL == b.GatewayURL || next.Generation != b.Generation+1 || st.Settings.GatewayPort != port {
		t.Fatal(next, st.Settings)
	}
	if e = s.CommitPortChange(ctx, plan.ID, plan.Fingerprint); e != nil {
		t.Fatal("retry", e)
	}
}

type portGateway struct{}

func (portGateway) Ensure(context.Context, model.Binding) (string, error) {
	return "http://127.0.0.1:43177/v1", nil
}
