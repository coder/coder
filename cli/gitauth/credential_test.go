package gitauth_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/gitauth"
)

func TestParseCredentialRequest(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		input   string
		match   string
		matchOK bool
	}{
		{
			name:    "HTTPS",
			input:   "capability[]=authtype\nprotocol=https\nhost=github.com\nwwwauth[]=Basic realm=\"GitHub\"\n\n",
			match:   "https://github.com",
			matchOK: true,
		},
		{
			name:    "WithPort",
			input:   "protocol=http\nhost=git.example.com:8080\n",
			match:   "http://git.example.com:8080",
			matchOK: true,
		},
		{
			name:    "WithPath",
			input:   "protocol=https\nhost=github.com\npath=coder/coder.git\n\n",
			match:   "https://github.com/coder/coder.git",
			matchOK: true,
		},
		{
			name:    "CRLF",
			input:   "protocol=https\r\nhost=github.com\r\n\r\n",
			match:   "https://github.com",
			matchOK: true,
		},
		{
			name:    "StopsAtBlankLine",
			input:   "protocol=https\nhost=github.com\n\nhost=example.com\n",
			match:   "https://github.com",
			matchOK: true,
		},
		{
			name:  "SSH",
			input: "protocol=ssh\nhost=github.com\n",
		},
		{
			name:  "NoHost",
			input: "protocol=https\n",
		},
		{
			name: "Empty",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req, err := gitauth.ParseCredentialRequest(strings.NewReader(tc.input))
			require.NoError(t, err)
			match, ok := req.MatchURL()
			require.Equal(t, tc.matchOK, ok)
			require.Equal(t, tc.match, match)
		})
	}
}
