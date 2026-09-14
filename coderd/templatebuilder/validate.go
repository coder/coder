package templatebuilder

import (
	"encoding/json"
	"regexp"
	"strings"

	"golang.org/x/xerrors"
)

// maxStringValueLen is the maximum byte length for a string variable value.
const maxStringValueLen = 4096

// numberPattern matches valid HCL number literals (integers and decimals).
var numberPattern = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

// validateVariableValue checks that the caller-supplied value is valid for
// the variable's declared type. String values are plain text (not
// pre-quoted); quoting for HCL happens later in toHCLLiteral.
// The literal "null" is accepted for any type.
func validateVariableValue(v ModuleVariable, value string) error {
	if value == "null" {
		return nil
	}
	switch v.Type {
	case "string":
		return validateStringValue(value)
	case "number":
		return validateNumberValue(value)
	case "bool":
		return validateBoolValue(value)
	default:
		return xerrors.Errorf("unsupported variable type %q", v.Type)
	}
}

// toHCLLiteral converts a validated caller value into an HCL literal.
// The literal "null" is passed through for any type. Strings are wrapped
// in quotes with interior characters escaped; bools and numbers are
// already valid HCL literals.
func toHCLLiteral(v ModuleVariable, value string) string {
	if value == "null" {
		return value
	}
	if v.Type == "string" {
		return hclQuote(value)
	}
	return value
}

// validateStringValue checks that a raw (unquoted) string value is safe
// to embed in an HCL quoted string. It rejects HCL interpolation/directive
// markers and values that exceed the maximum length.
func validateStringValue(value string) error {
	if len(value) > maxStringValueLen {
		return xerrors.Errorf("value exceeds maximum length of %d bytes", maxStringValueLen)
	}
	if strings.Contains(value, "${") || strings.Contains(value, "%{") {
		return xerrors.New("must not contain HCL interpolation or directive sequences")
	}
	return nil
}

// hclQuote wraps a raw string in HCL double-quotes, escaping backslashes,
// double-quotes, and newlines so the result is a valid HCL string literal.
func hclQuote(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	_, _ = b.WriteRune('"')
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			_, _ = b.WriteString("\\\\")
		case '"':
			_, _ = b.WriteString("\\\"")
		case '\n':
			_, _ = b.WriteString("\\n")
		case '\r':
			_, _ = b.WriteString("\\r")
		default:
			_ = b.WriteByte(s[i])
		}
	}
	_, _ = b.WriteRune('"')
	return b.String()
}

// validateNumberValue checks that value is a valid HCL number literal.
func validateNumberValue(value string) error {
	if !numberPattern.MatchString(value) {
		return xerrors.Errorf("invalid number value %q, must be a numeric literal (e.g. 42, 3.14)", value)
	}
	return nil
}

// validateBoolValue checks that value is exactly "true" or "false".
func validateBoolValue(value string) error {
	if value != "true" && value != "false" {
		return xerrors.Errorf("invalid bool value %q, must be true or false", value)
	}
	return nil
}

// isSimpleJSONValue returns true if raw is a valid JSON string, number,
// bool, or null. Arrays and objects are rejected; the template builder
// only supports simple variable types.
func isSimpleJSONValue(raw json.RawMessage) bool {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	switch v.(type) {
	case string, float64, bool, nil:
		return true
	default:
		return false
	}
}
