package usage

import (
	"bufio"
	"context"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"os"
	"path/filepath"
	"time"
)

type Activity struct {
	ActiveDays, CurrentStreak, LongestStreak int
	Peak                                     Daily
	TotalTokens                              int64
}

func AnalyzeActivity(days []Daily, today time.Time) Activity {
	a := Activity{}
	streak := 0
	byDate := map[string]Daily{}
	for _, d := range days {
		byDate[d.Date] = d
		a.TotalTokens += d.Tokens
		if d.Tokens > a.Peak.Tokens {
			a.Peak = d
		}
		if d.Records > 0 || d.Tokens > 0 {
			a.ActiveDays++
			streak++
			a.LongestStreak = max(a.LongestStreak, streak)
		} else {
			streak = 0
		}
	}
	cursor := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	if d := byDate[cursor.Format("2006-01-02")]; d.Records == 0 && d.Tokens == 0 {
		cursor = cursor.AddDate(0, 0, -1)
	}
	for n := 0; n < len(days); n++ {
		d, ok := byDate[cursor.Format("2006-01-02")]
		if !ok || d.Records == 0 && d.Tokens == 0 {
			break
		}
		a.CurrentStreak++
		cursor = cursor.AddDate(0, 0, -1)
	}
	return a
}

type RawCall struct {
	Source           string
	FromLine, ToLine int64
	Lines            []string
	Truncated        bool
}

// Raw contents are read only for an explicitly selected call. They are never
// logged, and a legacy DB record cannot authorize an arbitrary filesystem read.
func ReadRawCall(ctx context.Context, e model.UsageEvent, homes []string) (RawCall, error) {
	var out RawCall
	roots := []string{}
	for _, h := range homes {
		roots = append(roots, filepath.Join(h, "sessions"), filepath.Join(h, "archived_sessions"))
	}
	if !storage.Within(e.SourceFile, roots) || filepath.Ext(e.SourceFile) != ".jsonl" {
		return out, fault.New("PATH_INVALID", "Call source is outside registered session roots")
	}
	if err := storage.RejectSymlinks(e.SourceFile); err != nil {
		return out, err
	}
	f, err := os.Open(e.SourceFile)
	if err != nil {
		return out, fault.New("RAW_SOURCE_MISSING", "Original session is unavailable; the imported usage row is still preserved")
	}
	defer f.Close()
	out.Source = e.SourceFile
	out.FromLine = max(int64(1), e.LineNumber-12)
	end := e.LineNumber + 4
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64<<10), 8<<20)
	var n int64
	size := 0
	for scan.Scan() {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		n++
		if n < out.FromLine {
			continue
		}
		if n > end {
			break
		}
		line := scan.Text()
		if size+len(line) > 4<<20 {
			out.Truncated = true
			break
		}
		out.Lines = append(out.Lines, line)
		size += len(line)
		out.ToLine = n
	}
	return out, scan.Err()
}
