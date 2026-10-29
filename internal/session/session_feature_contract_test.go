package session

import (
	"context"
	"fmt"
	"lam.local/lam2/internal/account"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This test traces legacy list_sessions, list_sessions_page, query_sessions_page,
// cross-profile aggregation, stable pagination, sort modes and detail reads.
func TestMultiProfileQueryStablePaginationSortAndDetails(t *testing.T) {
	ctx := context.Background()
	s, first := setup(t)
	state, err := s.Store.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := (&account.Service{Store: s.Store}).Create(ctx, "Other", "profile", state.Revision)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	type entry struct {
		p     string
		id    string
		title string
		size  int
	}
	data := []struct {
		pID   string
		pHome string
		id    string
		title string
		extra int
	}{
		{first.ID, first.Home, "a1", "Alpha", 10},
		{first.ID, first.Home, "a2", "Echo", 60},
		{second.ID, second.Home, "b1", "Beta", 5},
		{second.ID, second.Home, "b2", "Delta", 100},
	}
	entries := map[string]entry{}
	for _, it := range data {
		p := first
		if it.pID == second.ID {
			p = second
		}
		path := writeSession(t, p, it.id, at)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		raw = []byte(strings.Replace(string(raw), "Investigate parser", it.title, 1))
		raw = append(raw, []byte(fmt.Sprintf("{\"type\":\"system\",\"padding\":%q}\n", strings.Repeat("x", it.extra)))...)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		// Keep timestamps tied to exercise the final deterministic ID tiebreak.
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
		entries[it.id] = entry{p: it.pID, id: it.id, title: it.title, size: len(raw)}
	}
	all, err := s.Query(ctx, Query{Limit: 20, Sort: "title"})
	if err != nil || all.Total != 4 || len(all.Items) != 4 {
		t.Fatalf("cross profile total: %+v %v", all, err)
	}
	for i, id := range []string{"a1", "b1", "b2", "a2"} {
		got := all.Items[i]
		if got.ID != id || got.ProfileID != entries[id].p || got.Title != entries[id].title {
			t.Fatalf("incorrect title sort/attribution %+v", got)
		}
	}
	sizeSorted, err := s.Query(ctx, Query{Limit: 20, Sort: "size", Descending: true})
	if err != nil || len(sizeSorted.Items) != 4 {
		t.Fatal(err, sizeSorted)
	}
	for i := 1; i < len(sizeSorted.Items); i++ {
		if sizeSorted.Items[i-1].Size < sizeSorted.Items[i].Size {
			t.Fatal("size descending order not respected")
		}
	}
	firstPage, err := s.Query(ctx, Query{Limit: 2, Sort: "modified", Offset: 0})
	if err != nil || len(firstPage.Items) != 2 {
		t.Fatal(err, firstPage)
	}
	secondPage, err := s.Query(ctx, Query{Limit: 2, Sort: "modified", Offset: 2})
	if err != nil || len(secondPage.Items) != 2 {
		t.Fatal(err, secondPage)
	}
	byKey := map[string]bool{}
	for _, p := range append(firstPage.Items, secondPage.Items...) {
		key := p.ProfileID + "/" + p.ID
		if byKey[key] {
			t.Fatalf("duplicate cross-profile page item: %s", key)
		}
		byKey[key] = true
	}
	repeat, err := s.Query(ctx, Query{Limit: 2, Sort: "modified", Offset: 2})
	if err != nil || repeat.Items[0].ID != secondPage.Items[0].ID || repeat.Items[1].ID != secondPage.Items[1].ID {
		t.Fatal("tied pagination unstable")
	}
	one, err := s.Query(ctx, Query{ProfileID: first.ID, Limit: 20})
	if err != nil || one.Total != 2 {
		t.Fatal("profile filtering leaked", one, err)
	}
	detail, err := s.Read(ctx, second.ID, "b1")
	if err != nil || detail.Session.ProfileID != second.ID || detail.Session.Title != "Beta" || len(detail.Lines) != 3 {
		t.Fatalf("detail not loaded: %+v %v", detail, err)
	}
	if _, err := s.Read(ctx, first.ID, "b1"); err == nil {
		t.Fatal("session detail read crossed profile")
	}
}

func TestStorageSummaryCountsBytesAndProtectedRanges(t *testing.T) {
	ctx := context.Background()
	s, p := setup(t)
	now := time.Now()
	var total int64
	for i := 0; i < 24; i++ {
		path := writeSession(t, p, fmt.Sprintf("summary-%02d", i), now.Add(-time.Duration(i+20)*24*time.Hour))
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
	}
	got, err := s.StorageSummary(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sessions != 24 || got.Bytes != total || got.Deletable != 4 || got.DeletableBytes <= 0 || got.DeletableBytes >= total {
		t.Fatalf("storage totals/protection mismatch %+v expected size=%d", got, total)
	}
	direct, err := s.Query(ctx, Query{ProfileID: p.ID, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	var deletable int64
	for _, item := range direct.Items {
		if !item.Protected {
			deletable += item.Size
		}
	}
	if got.DeletableBytes != deletable {
		t.Fatalf("deletable size %d vs %d", got.DeletableBytes, deletable)
	}
	empty, err := s.StorageSummary(ctx, "missing")
	if err == nil || empty.Sessions != 0 {
		t.Fatalf("nonexistent profile accepted: %+v %v", empty, err)
	}
	if _, err := os.Stat(filepath.Join(p.Home, "auth.json")); err == nil {
		t.Fatal("summary touched authentication unexpectedly")
	}
}
