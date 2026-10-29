package handoff

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrefixHistoriesDoNotRequireDivergenceOverride(t *testing.T) {
	s, a, b, path := setup(t)
	ctx := context.Background()
	source, _ := os.ReadFile(path)
	dst := filepath.Join(b.Home, "sessions", "rollout-original.jsonl")
	os.WriteFile(dst, append(append([]byte{}, source...), []byte("{\"type\":\"event_msg\",\"payload\":{\"type\":\"agent_message\",\"message\":\"newer\"}}\n")...), 0600)
	p, e := s.Plan(ctx, a.ID, "original", b.ID, "stop_and_ask", false)
	if e != nil || len(p.Blocked) > 0 {
		t.Fatal(p, e)
	}
	before, _ := os.ReadFile(dst)
	r, e := s.Execute(ctx, p.ID, p.Fingerprint)
	after, _ := os.ReadFile(dst)
	if e != nil || !r.Idempotent || string(before) != string(after) {
		t.Fatal(r, e)
	}
	os.WriteFile(dst, source[:strings.IndexByte(string(source), '\n')+1], 0600)
	p, e = s.Plan(ctx, a.ID, "original", b.ID, "stop_and_ask", false)
	if e != nil || len(p.Blocked) > 0 {
		t.Fatal(p, e)
	}
	r, e = s.Execute(ctx, p.ID, p.Fingerprint)
	after, _ = os.ReadFile(dst)
	if e != nil || string(after) != string(source) || r.Action != "extended" {
		t.Fatal(r, e)
	}
}
func TestAllLegacyDivergedStrategiesKeepTargetWhenRequired(t *testing.T) {
	for _, policy := range []string{"prefer_target", "timeline_merge_to_fork", "summarize_fork_with_target_account"} {
		t.Run(policy, func(t *testing.T) {
			s, a, b, _ := setup(t)
			ctx := context.Background()
			dst := filepath.Join(b.Home, "sessions", "rollout-original.jsonl")
			body := []byte("{\"type\":\"session_meta\",\"payload\":{\"id\":\"original\",\"cwd\":\"/work\"}}\n{\"type\":\"event_msg\",\"payload\":{\"type\":\"agent_message\",\"message\":\"target-branch\"}}\n")
			os.WriteFile(dst, body, 0600)
			p, e := s.Plan(ctx, a.ID, "original", b.ID, policy, false)
			if e != nil || len(p.Blocked) > 0 {
				t.Fatal(p, e)
			}
			r, e := s.Execute(ctx, p.ID, p.Fingerprint)
			if e != nil {
				t.Fatal(e)
			}
			got, _ := os.ReadFile(dst)
			if string(got) != string(body) {
				t.Fatal("target overwritten")
			}
			if r.SessionID != "original" {
				t.Fatal("resume no longer targets old branch")
			}
			if policy == "summarize_fork_with_target_account" {
				content, e := os.ReadFile(r.HandoffPath)
				if e != nil || r.ResumePrompt == "" || !strings.Contains(string(content), "target-branch") {
					t.Fatal(r, e)
				}
			} else {
				content, e := os.ReadFile(r.ForkPath)
				if e != nil || !strings.Contains(string(content), "hello") {
					t.Fatal(r, e)
				}
				if policy == "timeline_merge_to_fork" && !strings.Contains(string(content), "target-branch") {
					t.Fatal(string(content))
				}
			}
		})
	}
}
func TestRelayCompatibilityCannotBeOverriddenForProtocolLoss(t *testing.T) {
	cases := []string{`{"type":"response_item","payload":{"type":"reasoning","encrypted_content":"opaque"}}`, `{"type":"response_item","payload":{"type":"function_call","call_id":"orphan","name":"tool"}}`, `{"type":"response_item","payload":{"type":"custom_tool_call","call_id":"x"}}`, `{"type":"future_unknown"}`}
	for _, raw := range cases {
		r := AnalyzeCompatibility([]byte(raw+"\n"), true)
		if r.Disposition != "blocked" {
			t.Fatal(raw, r)
		}
	}
	r := AnalyzeCompatibility([]byte("{\"type\":\"response_item\",\"payload\":{\"type\":\"function_call\",\"call_id\":\"x\"}}\n{\"type\":\"response_item\",\"payload\":{\"type\":\"function_call_output\",\"call_id\":\"x\"}}\n"), true)
	if r.Disposition != "compatible" {
		t.Fatal(r)
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "secret") {
		t.Fatal("analysis retains bodies")
	}
}
