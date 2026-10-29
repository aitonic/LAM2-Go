package provider

import "lam.local/lam2/internal/model"

// configEdits is shared by initial attach and edits of an existing API account.
// Replacing the owned provider table also removes stale headers/auth options.
func (s *Service) configEdits(pr model.Provider, b model.Binding) []Edit {
	base := pr.BaseURL
	if b.RouteKind == "gateway" {
		base = b.GatewayURL
	}
	display := pr.Codex.DisplayName
	if display == "" {
		display = pr.Name
	}
	config := map[string]any{"name": display, "base_url": base, "wire_api": "responses"}
	if pr.AuthKind != "none" || b.RouteKind == "gateway" {
		config["auth"] = map[string]any{"command": s.Executable, "args": []string{"--home", s.Store.Root, "auth", "--binding", b.ID}}
	}
	if b.RouteKind == "direct" {
		headers := map[string]string{}
		for k, v := range pr.EnvHeaders {
			headers[k] = v
		}
		if pr.AuthKind == "api_key" {
			delete(config, "auth")
			headers[pr.AuthHeader] = "LAM_PROVIDER_HEADER_TOKEN"
		}
		if len(headers) > 0 {
			config["env_http_headers"] = headers
		}
		if len(pr.QueryParams) > 0 {
			config["query_params"] = pr.QueryParams
		}
	}
	if pr.Codex.StreamIdleTimeoutMS != nil {
		config["stream_idle_timeout_ms"] = *pr.Codex.StreamIdleTimeoutMS
	}
	if pr.Codex.RequestMaxRetries != nil {
		config["request_max_retries"] = *pr.Codex.RequestMaxRetries
	}
	if pr.Codex.StreamMaxRetries != nil {
		config["stream_max_retries"] = *pr.Codex.StreamMaxRetries
	}
	edits := []Edit{{KeyPath: "model", Value: b.Model, MergeStrategy: "replace"}, {KeyPath: "model_provider", Value: "lam_" + pr.ID, MergeStrategy: "replace"}, {KeyPath: "model_providers.lam_" + pr.ID, Value: config, MergeStrategy: "replace"}}
	return append(edits, modelSettingsEdits(pr, b.Model, b.CatalogPath)...)
}
