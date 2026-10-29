package session

import (
	"context"
	"lam.local/lam2/internal/fault"
)

type StorageSummary struct {
	Sessions, Deletable   int
	Bytes, DeletableBytes int64
}

// SelectFiltered applies the whole query, not just the currently visible page.
// The subsequent deletion preview is the authority for protection and hashes.
func (s *Service) SelectFiltered(ctx context.Context, q Query, deletableOnly bool) ([]Ref, error) {
	q.Offset = 0
	q.Limit = 1000
	out := []Ref{}
	seen := map[string]bool{}
	for {
		page, e := s.Query(ctx, q)
		if e != nil {
			return nil, e
		}
		for _, item := range page.Items {
			key := item.ProfileID + "/" + item.ID
			if seen[key] {
				return nil, fault.New("SESSION_AMBIGUOUS", "Duplicate session identity in filtered selection")
			}
			seen[key] = true
			if !deletableOnly || !item.Protected {
				out = append(out, Ref{item.ProfileID, item.ID})
			}
		}
		q.Offset += len(page.Items)
		if q.Offset >= page.Total || len(page.Items) == 0 {
			return out, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
}
func (s *Service) StorageSummary(ctx context.Context, profileID string) (StorageSummary, error) {
	q := Query{ProfileID: profileID, Limit: 1000}
	var out StorageSummary
	for {
		page, e := s.Query(ctx, q)
		if e != nil {
			return out, e
		}
		for _, v := range page.Items {
			out.Sessions++
			out.Bytes += v.Size
			if !v.Protected {
				out.Deletable++
				out.DeletableBytes += v.Size
			}
		}
		q.Offset += len(page.Items)
		if q.Offset >= page.Total || len(page.Items) == 0 {
			return out, nil
		}
	}
}
