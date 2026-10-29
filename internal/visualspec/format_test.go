package visualspec

import "testing"

func TestOriginalStorageBytesLabelsAndThresholds(t *testing.T) {
	for _, tc := range []struct {
		b    int64
		want string
	}{
		{0, "0 B"}, {100, "100 B"}, {1023, "1023 B"}, {1024, "1.0 KB"},
		{9 * 1024, "9.0 KB"}, {10 * 1024, "10 KB"}, {1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"}, {15 * 1024 * 1024, "15 MB"}, {1024 * 1024 * 1024, "1.0 GB"},
	} {
		if got := FormatStorageBytes(tc.b); got != tc.want {
			t.Errorf("bytes=%d got %q, want %q", tc.b, got, tc.want)
		}
	}
}
