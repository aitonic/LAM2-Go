package visualspec

import "fmt"

// FormatStorageBytes matches the original LAM views.tsx formatStorageBytes:
// multiples of 1024 labeled KB/MB/GB/TB, one decimal under 10 and integer
// precision from 10 onward. It deliberately uses the original "KB" spelling.
func FormatStorageBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	val := float64(bytes) / 1024
	unit := units[0]
	for idx := 1; idx < len(units) && val >= 1024; idx++ {
		val /= 1024
		unit = units[idx]
	}
	if val >= 10 {
		return fmt.Sprintf("%.0f %s", val, unit)
	}
	return fmt.Sprintf("%.1f %s", val, unit)
}
