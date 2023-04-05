package cli_test

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/testutil"
)

func TestVersion(t *testing.T) {
	t.Parallel()
	ansiExpr := regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")
	clean := func(s string) string {
		s = ansiExpr.ReplaceAllString(s, "")
		s = strings.Replace(s, "\r\n", "\n", -1)
		return s
	}
	expectedHuman := `Coder v0.0.0-devel
https://github.com/coder/coder

Full build of Coder, supports the  server  subcommand.
`
	expectedJSON := `{
  "version": "v0.0.0-devel",
  "build_time": "0001-01-01T00:00:00Z",
  "external_url": "https://github.com/coder/coder",
  "slim": false,
  "agpl": false
}
`
	for _, tt := range []struct {
		Name     string
		Args     []string
		Expected string
	}{
		{
			Name:     "Defaults to human-readable output",
			Args:     []string{"version"},
			Expected: expectedHuman,
		},
		{
			Name:     "JSON output",
			Args:     []string{"version", "--json"},
			Expected: expectedJSON,
		},
	} {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			t.Cleanup(cancel)
			inv, _ := clitest.New(t, tt.Args...)
			buf := new(bytes.Buffer)
			inv.Stdout = buf
			err := inv.WithContext(ctx).Run()
			require.NoError(t, err)
			actual := clean(buf.String())
			require.Equal(t, tt.Expected, actual)
		})
	}
}
