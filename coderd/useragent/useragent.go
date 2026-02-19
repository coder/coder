package useragent

import "strings"

// ParseOS extracts a simplified OS name from a User-Agent string.
// Returns an empty string if the OS cannot be determined.
//
// It first checks for standard browser-style UA tokens, then falls
// back to heuristics for bare HTTP client libraries (e.g.
// "okhttp/4.12.0" → "android", "CFNetwork/..." → "iOS").
func ParseOS(ua string) string {
	lower := strings.ToLower(ua)

	// Standard browser/webview tokens.
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
	}

	// Heuristics for bare HTTP client libraries that don't embed
	// a full platform token. These are best-guess mappings.
	switch {
	// okhttp is the default Android/Kotlin HTTP stack.
	case strings.HasPrefix(lower, "okhttp/"):
		return "android"
	// CFNetwork is Apple's low-level networking framework.
	// Could be macOS or iOS; iOS is far more common for bare
	// CFNetwork UAs so we default to iOS.
	case strings.HasPrefix(lower, "cfnetwork/"):
		return "iOS"
	// Dart's built-in HTTP client, predominantly Flutter on
	// mobile. Could be either platform; we can't distinguish.
	case strings.HasPrefix(lower, "dart/"):
		return ""
	}

	return ""
}
