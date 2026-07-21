package docgenenv

import (
	"encoding/json"
	"regexp"
	"strings"
)

// safeScalarRegex matches values that can be emitted as a bare (unquoted) YAML
// scalar: they start with an alphanumeric and contain only alphanumerics,
// spaces, and a small set of punctuation YAML never treats specially. A
// trailing space is disallowed because YAML strips it on read, so a bare scalar
// ending in a space would not round-trip back to the original string.
var safeScalarRegex = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9 ._/-]*[A-Za-z0-9._/-])?$`)

// numberScalarRegex matches values YAML would resolve to an integer or float.
var numberScalarRegex = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`)

// YAMLScalar renders s as a YAML scalar suitable for a front matter value.
// Simple values are emitted verbatim. Anything YAML could misparse, such as
// special characters or a bare word/number YAML would otherwise resolve to a
// bool, null, or number, is JSON-encoded, which is valid YAML that quotes and
// escapes the value so it round-trips back to the original string.
//
// Both the CLI and API documentation generators share this helper so their
// front-matter escaping cannot silently diverge.
func YAMLScalar(s string) string {
	if isBareScalar(s) {
		return s
	}
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// isBareScalar reports whether s can be emitted unquoted without YAML
// reinterpreting it as a non-string type.
func isBareScalar(s string) bool {
	if !safeScalarRegex.MatchString(s) {
		return false
	}
	// Even when every character is safe, quote values YAML would resolve to a
	// bool or null (e.g. a title of "true" or "null") so they stay strings.
	switch strings.ToLower(s) {
	case "true", "false", "yes", "no", "on", "off", "y", "n", "null", "none", "~":
		return false
	}
	// Likewise quote anything that parses as a number (e.g. "123", "1.5").
	return !numberScalarRegex.MatchString(s)
}
