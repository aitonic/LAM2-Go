package usage

import (
	"lam.local/lam2/internal/model"
	"testing"
)

func TestInsightsUsesFullEventSetAndLegacySemantics(t *testing.T) {
	a := NewInsightsAccumulator()
	for _, e := range []model.UsageEvent{{ID: "a", SessionID: "a", ProfileID: "p", Effort: "medium", SubagentType: "Planner"}, {ID: "b", SessionID: "a", ProfileID: "p", Effort: "low", AgentRole: "Coder"}, {ID: "c", SessionID: "b", ProfileID: "p", Effort: "medium", AgentNickname: "planner"}} {
		a.Add(e)
	}
	v := a.Result()
	if v.FastModePercent == nil || *v.FastModePercent != 1.0/3.0 || v.MostUsedReasoning != "Medium" || v.MostUsedReasoningPercent == nil || *v.MostUsedReasoningPercent != 2.0/3.0 || v.SkillsExplored != 2 || v.TotalSkillsUsed != 3 || v.TotalThreads != 2 {
		t.Fatal(v)
	}
	if v := NewInsightsAccumulator().Result(); v.FastModePercent != nil {
		t.Fatal("empty data fabricated as zero")
	}
}
func TestContextMetadataKeepsEffortAndPerCallFields(t *testing.T) {
	effort := ""
	v := CallMetadata([]byte(`{"type":"turn_context","payload":{"effort":"high"}}`), &effort)
	if v.Effort != "high" {
		t.Fatal(v)
	}
	v = CallMetadata([]byte(`{"type":"event_msg","payload":{"type":"token_count","agent_role":"reviewer","info":{"model_context_window":200000}}}`), &effort)
	if v.Effort != "high" || v.AgentRole != "reviewer" || v.ModelContextWindow != 200000 {
		t.Fatal(v)
	}
}

func TestDiagnosticsMatchesOriginalThresholds(t *testing.T) {
	a := NewDiagnosticsAccumulator()
	a.Add(model.UsageEvent{ID: "high", ProfileID: "p", SessionID: "s", Model: "unknown-model", Tokens: model.Tokens{Input: 80000, CachedInput: 1000}, ModelContextWindow: 100000})
	d := a.Result()
	if len(d.LowCacheThreads) != 1 || len(d.HighContextCalls) != 1 || len(d.UnknownModels) != 1 {
		t.Fatal(d)
	}
	a = NewDiagnosticsAccumulator()
	a.Add(model.UsageEvent{ID: "small", ProfileID: "p", SessionID: "s", Tokens: model.Tokens{Input: 1000}})
	if len(a.Result().LowCacheThreads) != 0 {
		t.Fatal("small thread misclassified")
	}
}
func TestLegacyGlobalAccountSnapshotStaysUnattributed(t *testing.T) {
	a, e := ParseLegacyAccountUsage([]byte(`{"snapshot_id":"global","fetched_at":"2026-10-20T00:00:00Z","lifetime_tokens":1000,"peak_daily_tokens":null}`), [][]byte{[]byte(`{"start_date":"2026-10-19","tokens":12}`)})
	if e != nil || a.Summary.LifetimeTokens == nil || *a.Summary.LifetimeTokens != 1000 || a.Summary.PeakDailyTokens != nil || len(a.DailyUsageBuckets) != 1 {
		t.Fatal(a, e)
	}
}
