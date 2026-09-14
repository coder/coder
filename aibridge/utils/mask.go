package utils

// MaskSecret masks the middle of a secret string, revealing a small
// prefix and suffix for identification. The number of characters
// revealed scales with string length.
func MaskSecret(s string) string {
	if s == "" {
		return ""
	}

	runes := []rune(s)
	reveal := revealLength(len(runes))

	if len(runes) <= reveal*2 {
		return "..."
	}

	prefix := string(runes[:reveal])
	suffix := string(runes[len(runes)-reveal:])
	return prefix + "..." + suffix
}

// revealLength returns the number of runes to show at each end.
func revealLength(n int) int {
	switch {
	case n >= 20:
		return 4
	case n >= 10:
		return 2
	case n >= 5:
		return 1
	default:
		return 0
	}
}
