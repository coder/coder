package chatutil

import "strings"

// NormalizedStringPointer trims a string pointer and returns nil for nil or
// empty values.
func NormalizedStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// NormalizedEnumValue returns the canonical allowed value matching value after
// case normalization, or nil when no value matches.
func NormalizedEnumValue(value string, allowed ...string) *string {
	for _, candidate := range allowed {
		if value == strings.ToLower(candidate) {
			match := candidate
			return &match
		}
	}
	return nil
}
