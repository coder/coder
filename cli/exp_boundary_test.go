package cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

// Here we want to test that integrating boundary as a subcommand doesn't break anything.
// The full boundary functionality is tested in enterprise/cli.
func TestBoundarySubcommand(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	inv, _ := clitest.New(t, "exp", "boundary", "--help")
	pty := ptytest.New(t).Attach(inv)

	go func() {
		err := inv.WithContext(ctx).Run()
		assert.NoError(t, err)
	}()

	// Expect the --help output to include the short description.
	pty.ExpectMatch("Network isolation tool")
}
