package cli_test

import (
	"context"
	"testing"

	boundarycli "github.com/coder/boundary/cli"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
	"github.com/stretchr/testify/assert"
)

// Actually testing the functionality of coder/boundary takes place in the
// coder/boundary repo, since it's a dependency of coder.
// Here we want to test basically that integrating it as a subcommand doesn't break anything.
func TestJailSubcommand(t *testing.T) {
	inv, _ := clitest.New(t, "boundary", "--help")
	pty := ptytest.New(t).Attach(inv)
	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancelFunc()
	done := make(chan any)
	go func() {
		err := inv.WithContext(ctx).Run()
		assert.NoError(t, err)
		close(done)
	}()

	// Expect the --help output to include the short description.
	// We're simply confirming that `coder boundary --help` ran without a runtime error as
	// a good chunk of serpents self validation logic happens at runtime.
	pty.ExpectMatch(boundarycli.BaseCommand().Short)
	cancelFunc()
	<-done
}
