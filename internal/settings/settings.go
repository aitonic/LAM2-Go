package settings

import (
	"context"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"strings"
)

type Service struct{ Store *storage.Store }

func in(v string, options ...string) bool {
	for _, o := range options {
		if v == o {
			return true
		}
	}
	return false
}
func Validate(s model.Settings) error {
	if s.AccountMode != "profile" && s.AccountMode != "pat" && s.AccountMode != "" {
		return fault.New("SETTING_INVALID", "Invalid account mode")
	}
	if !in(s.ModeAvailability, "", "profile", "pat", "both") {
		return fault.New("SETTING_INVALID", "Invalid mode availability")
	}
	if s.TrayOpacity != 0 && (s.TrayOpacity < 0.2 || s.TrayOpacity > 1) {
		return fault.New("SETTING_INVALID", "Tray opacity must be between 0.2 and 1")
	}
	bad := func(msg string) error { return fault.New("SETTINGS_INVALID", msg) }
	if !in(s.Theme, "system", "light", "dark") || !in(s.Language, "system", "en", "zh-Hans") {
		return bad("Unknown theme or language")
	}
	if !in(s.Terminal, "terminal", "ghostty", "cmux", "embedded", "direct", "codex_app") {
		return bad("Unsupported terminal")
	}
	if s.CodexPath == "" || strings.ContainsAny(s.CodexPath, "\r\n\x00") {
		return bad("Set a valid Codex executable path")
	}
	if s.RefreshSeconds < 15 || s.RefreshSeconds > 86400 {
		return bad("Refresh interval must be 15 to 86400 seconds")
	}
	if !in(s.ApprovalPolicy, "on-request", "untrusted", "never", "on-failure") || !in(s.SandboxMode, "read-only", "workspace-write", "danger-full-access") {
		return bad("Invalid execution policy")
	}
	if s.AntigravityPort < 0 || s.AntigravityPort > 65535 {
		return bad("Antigravity port must be 1 to 65535, or 0 to disable")
	}
	if s.GatewayPort < 0 || s.GatewayPort > 65535 {
		return bad("Invalid Gateway port")
	}
	g := s.Gateway
	if g.MaxConcurrent < 1 || g.MaxConcurrent > 256 || g.PerBinding < 1 || g.PerBinding > g.MaxConcurrent || g.MaxQueued < 0 || g.MaxQueued > 1024 || g.PerBindingQueued < 0 || g.PerBindingQueued > g.MaxQueued {
		return bad("Invalid Gateway concurrency or queue limits")
	}
	if g.BodyBytes < 1024 || g.BodyBytes > 32<<20 || g.FrameBytes < 1024 || g.FrameBytes > 4<<20 || g.ToolArgumentBytes < 1024 || g.ToolArgumentBytes > 4<<20 || g.MaxTools < 1 || g.MaxTools > 256 || g.ResponseBytes < 1024 || g.ResponseBytes > 64<<20 {
		return bad("Invalid Gateway size limits")
	}
	if g.FirstResponseSeconds < 10 || g.FirstResponseSeconds > 600 {
		return bad("Gateway first response timeout must be between 10 and 600 seconds")
	}
	if g.RequestSeconds < 300 || g.RequestSeconds > 3600 {
		return bad("Gateway request timeout must be between 300 and 3600 seconds")
	}
	if s.UpdateURL != "" || s.UpdatePublicKey != "" {
		return fault.New("UPDATES_BUILD_CONFIG_ONLY", "Configure signed updates in mygo.json and rebuild; runtime feed overrides are not supported")
	}
	return nil
}
func (s Service) Save(ctx context.Context, v model.Settings, expected uint64) error {
	if err := Validate(v); err != nil {
		return err
	}
	_, err := s.Store.Update(ctx, expected, func(st *model.State) error {
		if st.Settings.GatewayPort != v.GatewayPort {
			return fault.New("PORT_MIGRATION_REQUIRED", "Change the Gateway port with its migration preview")
		}
		st.Settings = v
		return nil
	})
	return err
}
func (s Service) SavePrices(ctx context.Context, rates []model.RateCard, expected uint64) error {
	seen := map[string]bool{}
	for _, r := range rates {
		if _, err := r.Cost(model.Tokens{}); err != nil {
			return fault.New("PRICE_INVALID", "Each model price needs a version, USD currency and valid nonnegative rates")
		}
		key := r.Model + "/" + r.Version
		if seen[key] {
			return fault.New("PRICE_DUPLICATE", "Duplicate model price version")
		}
		seen[key] = true
	}
	_, err := s.Store.Update(ctx, expected, func(st *model.State) error { st.Prices = rates; return nil })
	return err
}
