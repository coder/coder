package chatd

// TruncateRunes truncates a string to at most max runes.
func TruncateRunes(value string, max int) string {
	return truncateRunes(value, max)
}

func truncateRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= max {
		return value
	}

	return string(runes[:max])
}
