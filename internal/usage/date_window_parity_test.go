package usage

import (
	"testing"
	"time"
)

func TestLegacyUsagePresetUTCWindowsAndAllTimeDefault(t *testing.T) {
	now := time.Date(2026, 10, 28, 23, 0, 0, 0, time.UTC) // Wednesday
	cases := []struct{ preset, start, end string }{
		{"all", "", ""},
		{"today", "2026-10-28", "2026-10-29"},
		{"this-week", "2026-10-26", "2026-10-29"},
		{"last-7-days", "2026-10-22", "2026-10-29"},
		{"this-month", "2026-10-01", "2026-10-29"},
	}
	for _, tc := range cases {
		start, end, err := PresetWindow(tc.preset, now)
		if err != nil || start != tc.start || end != tc.end {
			t.Fatalf("%s: got %s..%s / %v", tc.preset, start, end, err)
		}
	}
	if _, _, err := PresetWindow("made-up", now); err == nil {
		t.Fatal("unknown preset silently mapped to default")
	}
}
