package settings

import (
	"context"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"testing"
)

func TestAccountModeReadWriteAndInvalidValue(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := Service{Store: store}
	for _, mode := range []string{"pat", "profile"} {
		state, err := store.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		value := state.Settings
		value.AccountMode = mode
		if err := svc.Save(ctx, value, state.Revision); err != nil {
			t.Fatal(err)
		}
		got, err := store.Read(ctx)
		if err != nil || got.Settings.AccountMode != mode {
			t.Fatalf("mode did not round trip: %q %v", got.Settings.AccountMode, err)
		}
	}
	state, err := store.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	value := state.Settings
	value.AccountMode = "unsupported"
	if err := svc.Save(ctx, value, state.Revision); err == nil {
		t.Fatal("invalid account mode accepted")
	}
	after, _ := store.Read(ctx)
	if after.Revision != state.Revision || after.Settings.AccountMode != "profile" {
		t.Fatal("invalid update modified state")
	}
}

func TestGatewayTimeoutSettingsReadWriteAndBounds(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := Service{Store: store}
	for _, bounds := range [][2]int{{10, 300}, {120, 1800}, {600, 3600}} {
		state, err := store.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		value := state.Settings
		value.Gateway.FirstResponseSeconds = bounds[0]
		value.Gateway.RequestSeconds = bounds[1]
		if err := svc.Save(ctx, value, state.Revision); err != nil {
			t.Fatal(err)
		}
		after, err := store.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if after.Settings.Gateway.FirstResponseSeconds != bounds[0] || after.Settings.Gateway.RequestSeconds != bounds[1] {
			t.Fatalf("timeout read mismatch: %+v", after.Settings.Gateway)
		}
	}
	for _, pair := range [][2]int{{9, 1000}, {601, 1000}, {30, 299}, {30, 3601}} {
		state, _ := store.Read(ctx)
		v := state.Settings
		v.Gateway.FirstResponseSeconds, v.Gateway.RequestSeconds = pair[0], pair[1]
		if err := svc.Save(ctx, v, state.Revision); err == nil {
			t.Fatalf("invalid timeouts accepted: %v", pair)
		}
		after, _ := store.Read(ctx)
		if after.Revision != state.Revision {
			t.Fatal("invalid timeout mutated revision")
		}
	}
}

func TestLaunchPermissionPresetSettingsReadWriteAndRejection(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := Service{Store: store}
	for _, policy := range []string{"on-request", "untrusted", "never", "on-failure"} {
		for _, sandbox := range []string{"read-only", "workspace-write", "danger-full-access"} {
			state, _ := store.Read(ctx)
			v := state.Settings
			v.ApprovalPolicy, v.SandboxMode = policy, sandbox
			if err := svc.Save(ctx, v, state.Revision); err != nil {
				t.Fatal(err)
			}
			after, _ := store.Read(ctx)
			if after.Settings.ApprovalPolicy != policy || after.Settings.SandboxMode != sandbox {
				t.Fatalf("permission round-trip failed: %+v", after.Settings)
			}
		}
	}
	for _, invalid := range []model.Settings{
		func() model.Settings { v := model.DefaultSettings(); v.ApprovalPolicy = "bypass"; return v }(),
		func() model.Settings { v := model.DefaultSettings(); v.SandboxMode = "unsafe"; return v }(),
	} {
		st, _ := store.Read(ctx)
		if err := svc.Save(ctx, invalid, st.Revision); err == nil {
			t.Fatal("invalid launch permissions accepted")
		}
		after, _ := store.Read(ctx)
		if after.Revision != st.Revision {
			t.Fatal("invalid permissions were saved")
		}
	}
}

func TestAntigravityPortSettingsReadWriteAndBounds(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := Service{Store: store}
	for _, port := range []int{0, 1, 51677, 65535} {
		st, _ := store.Read(ctx)
		value := st.Settings
		value.AntigravityPort = port
		if err := svc.Save(ctx, value, st.Revision); err != nil {
			t.Fatal(err)
		}
		after, _ := store.Read(ctx)
		if after.Settings.AntigravityPort != port {
			t.Fatalf("port not preserved: %d / %d", port, after.Settings.AntigravityPort)
		}
	}
	for _, port := range []int{-1, 65536} {
		st, _ := store.Read(ctx)
		value := st.Settings
		value.AntigravityPort = port
		if err := svc.Save(ctx, value, st.Revision); err == nil {
			t.Fatal("invalid port accepted")
		}
		after, _ := store.Read(ctx)
		if after.Revision != st.Revision {
			t.Fatal("invalid port update changed state")
		}
	}
}

func TestDockVisibilitySettingsReadWriteAndCAS(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := Service{Store: store}
	st, _ := store.Read(ctx)
	if !st.Settings.ShowDock {
		t.Fatal("default dock visibility incorrect")
	}
	v := st.Settings
	v.ShowDock = false
	if err := svc.Save(ctx, v, st.Revision); err != nil {
		t.Fatal(err)
	}
	after, _ := store.Read(ctx)
	if after.Settings.ShowDock {
		t.Fatal("dock flag not saved")
	}
	if err := svc.Save(ctx, st.Settings, st.Revision); err == nil {
		t.Fatal("stale write accepted")
	}
	v = after.Settings
	v.ShowDock = true
	if err := svc.Save(ctx, v, after.Revision); err != nil {
		t.Fatal(err)
	}
	after, _ = store.Read(ctx)
	if !after.Settings.ShowDock {
		t.Fatal("restoring dock flag failed")
	}
}
