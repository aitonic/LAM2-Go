package usage

import (
	"strings"
	"testing"
)

func TestLegacyUsageFiltersAreParameterizedAndSortIsWhitelisted(t *testing.T) {
	f := Filter{Search: "50%_' OR 1=1 --", Effort: "high", PricingConfidence: "legacy-alias-estimate", SortKey: "cached", SortAscending: true}
	sql, args := f.SQL()
	for _, word := range []string{"json_extract(payload,'$.effort')", "json_extract(payload,'$.pricingConfidence')", "instr("} {
		if !strings.Contains(sql, word) {
			t.Fatalf("missing %s in %q", word, sql)
		}
	}
	if len(args) != 3 || args[2] != "50%_' OR 1=1 --" || strings.Contains(sql, "OR 1=1") {
		t.Fatalf("unsafe filter %q %#v", sql, args)
	}
	if got := f.CallOrderSQL(); !strings.Contains(got, "cached_input_tokens ASC") {
		t.Fatal(got)
	}
	f.SortKey = "anything; DROP TABLE usage_events;"
	f.SortAscending = false
	if got := f.CallOrderSQL(); strings.Contains(got, "DROP") || !strings.Contains(got, "timestamp DESC") {
		t.Fatal(got)
	}
}
