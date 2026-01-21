package cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	boundarycli "github.com/coder/boundary/cli"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

// Actually testing the functionality of coder/boundary takes place in the
// coder/boundary repo, since it's a dependency of coder.
// Here we want to test basically that integrating it as a subcommand doesn't break anything.
func TestBoundarySubcommand(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	inv, _ := newCLI(t, "boundary", "--help")
	pty := ptytest.New(t).Attach(inv)

	go func() {
		err := inv.WithContext(ctx).Run()
		assert.NoError(t, err)
	}()

	// Expect the --help output to include the short description.
	// We're simply confirming that `coder boundary --help` ran without a runtime error as
	// a good chunk of serpents self validation logic happens at runtime.
	pty.ExpectMatch(boundarycli.BaseCommand("dev").Short)
}
