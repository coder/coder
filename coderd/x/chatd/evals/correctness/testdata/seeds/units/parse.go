// Package units converts between byte counts and strings such as "1.5GiB".
package units

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseSize parses a size such as "512", "10KiB", or "1.5GiB" into bytes.
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	multiplier := int64(1)
	switch {
	case strings.HasSuffix(s, "GiB"):
		multiplier, s = 1<<30, strings.TrimSuffix(s, "GiB")
	case strings.HasSuffix(s, "MiB"):
		multiplier, s = 1<<20, strings.TrimSuffix(s, "MiB")
	case strings.HasSuffix(s, "KiB"):
		multiplier, s = 1<<10, strings.TrimSuffix(s, "KiB")
	case strings.HasSuffix(s, "B"):
		s = strings.TrimSuffix(s, "B")
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	return int64(n * float64(multiplier)), nil
}
