package coderd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeUsageAppName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		appName string
		want    string
	}{
		{"Empty", "", ""},
		{"Whitespace", "   \t\n ", ""},
		{"NullBytes", "\x00\x00", ""},
		// Control characters carry no name, so they must read as an absent
		// app name rather than a session under the unknown family.
		{"ControlOnly", "\x1b\x07\x01", ""},
		{"ControlAndWhitespace", " \x1b \x07\t", ""},
		{"EscapeSequenceKeepsPrintableText", "\x1b[31m", "[31m"},
		{"Normalized", "Some-Future-IDE", "some_future_ide"},
		{"ControlStripped", "cur\x1bsor", "cursor"},
		{"LongWhitespace", strings.Repeat(" ", 300), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, normalizeUsageAppName(tc.appName))
		})
	}
}
