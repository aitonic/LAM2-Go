package usage

import (
	"lam.local/lam2/internal/model"
	"strings"
	"time"
)

// These are compatibility data from LAM 0.4.4 usage.rs::rate_for_model,
// NOT a claim about current official API prices. User-versioned cards win.
func legacyRate(name string, input int64) (model.RateCard, bool) {
	name = strings.ToLower(name)
	alias := name == "codex-auto-review"
	if alias {
		name = "gpt-5.3-codex"
	}
	p := model.RateCard{Model: name, Version: "legacy-0.4.4", Currency: "USD", Source: "LAM 0.4.4 services/usage.rs::rate_for_model (historical compatibility data)", EffectiveAt: time.Unix(0, 0)}
	long := input > 128000
	switch name {
	case "gpt-5.5":
		p.InputPerMillion = 5
		p.CachedPerMillion = .5
		p.OutputPerMillion = 30
		if long {
			p.InputPerMillion = 10
			p.CachedPerMillion = 1
			p.OutputPerMillion = 45
		}
	case "gpt-5.5-pro", "gpt-5.4-pro":
		p.InputPerMillion = 30
		p.CachedPerMillion = 30
		p.OutputPerMillion = 180
		if long {
			p.InputPerMillion = 60
			p.CachedPerMillion = 60
			p.OutputPerMillion = 270
		}
	case "gpt-5.4":
		p.InputPerMillion = 2.5
		p.CachedPerMillion = .25
		p.OutputPerMillion = 15
		if long {
			p.InputPerMillion = 5
			p.CachedPerMillion = .5
			p.OutputPerMillion = 22.5
		}
	case "gpt-5.4-mini":
		p.InputPerMillion = .75
		p.CachedPerMillion = .075
		p.OutputPerMillion = 4.5
	case "gpt-5.4-nano":
		p.InputPerMillion = .2
		p.CachedPerMillion = .02
		p.OutputPerMillion = 1.25
	case "gpt-5.3-codex", "gpt-5.2", "gpt-5":
		p.InputPerMillion = 4.375
		p.CachedPerMillion = .4375
		p.OutputPerMillion = 35
	default:
		return p, false
	}
	return p, true
}

type LegacyPriceRow struct {
	Rate      model.RateCard
	Context   string
	Estimated bool
}

func LegacyPriceTable() []LegacyPriceRow {
	out := []LegacyPriceRow{}
	for _, m := range []string{"gpt-5.5", "gpt-5.5-pro", "gpt-5.4", "gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.4-pro", "gpt-5.3-codex", "gpt-5.2", "gpt-5", "codex-auto-review"} {
		a, _ := legacyRate(m, 128000)
		a.Model = m
		b, _ := legacyRate(m, 128001)
		b.Model = m
		if a.InputPerMillion != b.InputPerMillion {
			out = append(out, LegacyPriceRow{a, "input <= 128000", false}, LegacyPriceRow{b, "input > 128000", false})
		} else {
			out = append(out, LegacyPriceRow{a, "all", m == "codex-auto-review"})
		}
	}
	return out
}
