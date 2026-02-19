package useragent

import "strings"

// ParseOS extracts a simplified OS name from a User-Agent string.
// Returns an empty string if the OS cannot be determined.
func ParseOS(ua string) string {
	lower := strings.ToLower(ua)
	// Check order matters: "android" must come before "linux"
	// because Android UAs contain "Linux".
	switch {
	case strings.Contains(lower, "windows"):
		return "windows"
	case strings.Contains(lower, "iphone") || strings.Contains(lower, "ipad"):
		return "iOS"
	case strings.Contains(lower, "macintosh") || strings.Contains(lower, "mac os"):
		return "macOS"
	case strings.Contains(lower, "android"):
		return "android"
	case strings.Contains(lower, "cros"):
		return "chromeos"
	case strings.Contains(lower, "linux"):
		return "linux"
	default:
		return ""
	}
}
