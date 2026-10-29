package usage

import "testing"

func TestLegacyDetailLoadLimitPageBoundaries(t *testing.T) {
	for _, tc := range []struct{ limit, offset, want int }{
		{5000, 0, 500}, {5000, 4500, 500}, {5000, 4999, 1}, {5000, 5000, 0},
		{10000, 9800, 200}, {20000, 19800, 200}, {0, 1000000, 500},
	} {
		got := (Filter{DetailLimit: tc.limit}).DetailBatch(tc.offset)
		if got != tc.want {
			t.Fatalf("limit=%d offset=%d got=%d want=%d", tc.limit, tc.offset, got, tc.want)
		}
	}
}
