package docgenenv_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

// TestYAMLScalarRoundTrip asserts the emitted scalar parses back to the exact
// input string. Unmarshaling into a Go string returns the scalar's text
// regardless of the type YAML would resolve it to, so this catches the values
// that break parsing or resolve away (quotes, colons, newlines, trailing space,
// null and ~) but not the reserved-word or number quoting intent. That intent
// is pinned directly in TestYAMLScalarBareWhenSafe.
func TestYAMLScalarRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []string{
		// Values seen in practice today.
		"server",
		"templates create",
		"Start a Coder server",
		"early access",
		"DEPRECATED: Create a template from the current directory or as specified by flag",
		"./images/icons/api.svg",
		// Special characters that must be escaped.
		`has "quotes" and: a colon`,
		"trailing backtick `code`",
		"line one\nline two",
		// Trailing space: YAML strips it on read, so a bare scalar would not
		// round-trip.
		"trailing space ",
		// Values YAML would otherwise resolve to a non-string type.
		"true",
		"False",
		"NULL",
		"no",
		"on",
		"123",
		"1.5",
		"-42",
		"~",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			doc := "value: " + docgenenv.YAMLScalar(in) + "\n"
			var got struct {
				Value string `yaml:"value"`
			}
			require.NoErrorf(t, yaml.Unmarshal([]byte(doc), &got), "emitted YAML must parse: %q", doc)
			require.Equalf(t, in, got.Value, "scalar must round-trip as a string, doc=%q", doc)
		})
	}
}

// TestYAMLScalarBareWhenSafe pins the bare-vs-quoted decision directly against
// the emitted text, which the round-trip test cannot do for the reserved-word
// and number class (unmarshaling into a Go string hands back the text whether
// or not YAMLScalar quoted it). Common values stay bare so regenerated pages
// don't churn; anything a YAML reader would resolve to a bool, null, or number
// is quoted. Trimming the isBareScalar switch or the number check fails here.
func TestYAMLScalarBareWhenSafe(t *testing.T) {
	t.Parallel()

	// Safe, stringy values stay bare.
	for _, in := range []string{
		"server",
		"templates create",
		"Start a Coder server",
		"early access",
	} {
		require.Equalf(t, in, docgenenv.YAMLScalar(in), "safe value %q must stay bare", in)
	}

	// Values that look bare but a YAML reader would resolve to a non-string
	// type must be quoted. json.Marshal wraps each of these verbatim, so the
	// expected form is simply the input in double quotes.
	for _, in := range []string{
		// YAML 1.1 bool aliases, in the casings isBareScalar folds.
		"true", "false", "yes", "no", "on", "off", "y", "n", "none",
		"True", "False", "Yes", "No", "On", "Off", "None", "NULL",
		// Null forms.
		"null", "~",
		// Numbers (integer, float, signed, scientific).
		"123", "1.5", "-42", "+7", "1e3",
	} {
		require.Equalf(t, `"`+in+`"`, docgenenv.YAMLScalar(in), "ambiguous value %q must be quoted", in)
	}

	// Values with characters YAML would misparse are quoted too.
	require.Equal(t, `"./images/icons/api.svg"`, docgenenv.YAMLScalar("./images/icons/api.svg"))
	// A trailing space forces quoting so the value round-trips.
	require.Equal(t, `"foo "`, docgenenv.YAMLScalar("foo "))
}
