package provider

import (
	"lam.local/lam2/internal/model"
	"testing"
)

func TestPerModelCapabilityPreflightUsesExplicitSupportAndUnknown(t *testing.T) {
	p := candidate()
	p.Capabilities.FunctionTools = "unsupported"
	p.Capabilities.CustomTools = "unknown"
	if err := Validate(p); err != nil {
		t.Fatal(err)
	}
	if model.Supported(p.ModelCapabilities("model-a").FunctionTools) {
		t.Fatal("tools incorrectly enabled by unsupported provider")
	}
	override := model.UnknownCapabilities()
	override.FunctionTools = "supported"
	override.CustomTools = "partial"
	p.Models[0].Capabilities = &override
	if err := Validate(p); err != nil {
		t.Fatal(err)
	}
	if !model.Supported(p.ModelCapabilities("model-a").FunctionTools) || !model.Supported(p.ModelCapabilities("model-a").CustomTools) {
		t.Fatal("per model override ignored")
	}
	if model.Supported(p.ModelCapabilities("missing-model").FunctionTools) {
		t.Fatal("unknown model must not infer supported tools")
	}
	p.Capabilities.FunctionTools = "invented"
	if err := Validate(p); err == nil {
		t.Fatal("unrecognized provider capability accepted")
	}
}
