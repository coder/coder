package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/testutil"
)

// We need to override the global color profile to test escape codes.
//
//nolint:tparallel,paralleltest
func TestVersion(t *testing.T) {
	ogColorProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(ogColorProfile)
	})

	expectedText := `Coder v0.0.0-devel
https://github.com/coder/coder

Full build of Coder, supports the [40m [0m[91;40mserver[0m[40m [0m subcommand.
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
			Expected: expectedText,
		},
		{
			Name:     "JSON output",
			Args:     []string{"version", "--output=json"},
			Expected: expectedJSON,
		},
		{
			Name:     "Text output",
			Args:     []string{"version", "--output=text"},
			Expected: expectedText,
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
			actual := buf.String()
			actual = strings.ReplaceAll(actual, "\r\n", "\n")
			require.Equal(t, tt.Expected, actual)
		})
	}
}
