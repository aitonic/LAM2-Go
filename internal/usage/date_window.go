package usage

import (
	"time"

	"lam.local/lam2/internal/fault"
)

// PresetWindow reproduces the legacy Usage date presets as half-open UTC day
// windows. "all" has no time filter; custom dates remain caller-owned.
// A half-open end avoids dropping calls in the last displayed day.
func PresetWindow(preset string, now time.Time) (start, end string, err error) {
	if preset == "all" || preset == "custom" {
		return "", "", nil
	}
	day := now.UTC().Truncate(24 * time.Hour)
	from := day
	switch preset {
	case "today":
	case "this-week":
		days := (int(day.Weekday()) + 6) % 7
		from = day.AddDate(0, 0, -days)
	case "last-7-days":
		from = day.AddDate(0, 0, -6)
	case "this-month":
		from = time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		return "", "", fault.New("USAGE_PRESET_INVALID", "Unsupported Usage time preset")
	}
	return from.Format("2006-01-02"), day.AddDate(0, 0, 1).Format("2006-01-02"), nil
}
