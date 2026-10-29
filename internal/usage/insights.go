package usage

import (
	"encoding/json"
	"lam.local/lam2/internal/model"
	"sort"
	"strings"
	"unicode"
)

type Insights struct {
	FastModePercent          *float64 `json:"fastModePercent"`
	MostUsedReasoning        string   `json:"mostUsedReasoning,omitempty"`
	MostUsedReasoningPercent *float64 `json:"mostUsedReasoningPercent"`
	SkillsExplored           int      `json:"skillsExplored"`
	TotalSkillsUsed          int      `json:"totalSkillsUsed"`
	TotalThreads             int      `json:"totalThreads"`
}
type InsightsAccumulator struct {
	total, fast, used int
	efforts           map[string]int
	skills, threads   map[string]bool
}

func NewInsightsAccumulator() *InsightsAccumulator {
	return &InsightsAccumulator{efforts: map[string]int{}, skills: map[string]bool{}, threads: map[string]bool{}}
}
func (a *InsightsAccumulator) Add(e model.UsageEvent) {
	a.total++
	switch strings.ToLower(e.Effort) {
	case "fast", "low", "minimal", "none":
		a.fast++
	}
	if strings.TrimSpace(e.Effort) != "" {
		a.efforts[e.Effort]++
	}
	key := e.ThreadKey
	if key == "" {
		key = e.ProfileID + ":" + e.SessionID
	}
	a.threads[key] = true
	skill := e.SubagentType
	if skill == "" {
		skill = e.AgentRole
	}
	if skill == "" {
		skill = e.AgentNickname
	}
	if skill != "" {
		a.skills[strings.ToLower(skill)] = true
		a.used++
	}
}
func (a *InsightsAccumulator) Result() Insights {
	v := Insights{SkillsExplored: len(a.skills), TotalSkillsUsed: a.used, TotalThreads: len(a.threads)}
	if a.total == 0 {
		return v
	}
	ratio := float64(a.fast) / float64(a.total)
	v.FastModePercent = &ratio
	keys := []string{}
	for k := range a.efforts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if a.efforts[keys[i]] == a.efforts[keys[j]] {
			return keys[i] < keys[j]
		}
		return a.efforts[keys[i]] > a.efforts[keys[j]]
	})
	if len(keys) > 0 {
		key := keys[0]
		label := []rune(strings.ToLower(key))
		label[0] = unicode.ToUpper(label[0])
		v.MostUsedReasoning = string(label)
		fraction := float64(a.efforts[key]) / float64(a.total)
		v.MostUsedReasoningPercent = &fraction
	}
	return v
}

// This is the LAM-specific metadata projection, not a replacement for the
// selected agent-parser. It preserves fields needed by original Usage Insights.
func CallMetadata(raw []byte, effort *string) model.UsageEvent {
	var root struct {
		Type    string                     `json:"type"`
		Payload map[string]json.RawMessage `json:"payload"`
	}
	_ = json.Unmarshal(raw, &root)
	read := func(keys ...string) string {
		for _, k := range keys {
			var s string
			if json.Unmarshal(root.Payload[k], &s) == nil && len(s) <= 4096 {
				return s
			}
		}
		return ""
	}
	if root.Type == "turn_context" {
		*effort = read("effort", "reasoning_effort")
	}
	var info struct {
		Window int64 `json:"model_context_window"`
	}
	_ = json.Unmarshal(root.Payload["info"], &info)
	return model.UsageEvent{Effort: *effort, SubagentType: read("subagent_type", "subagentType"), AgentRole: read("agent_role", "agentRole"), AgentNickname: read("agent_nickname", "agentNickname"), ModelContextWindow: info.Window}
}
