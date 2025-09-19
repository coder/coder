package cli_test

import (
	"context"
	"testing"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
	jailcli "github.com/coder/jail/cli"
	"github.com/stretchr/testify/assert"
)

// Actually testing the functionality of coder/jail takes place in the
// coder/jail repo, since it's a dependency of coder.
// Here we want to test basically that integrating it as a subcommand doesn't break anything.
func TestJailSubcommand(t *testing.T) {
	inv, _ := clitest.New(t, "jail", "--help")
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
	// We're simply confirming that `coder jail --help` ran without a runtime error as
	// a good chunk of serpents self validation logic happens at runtime.
	pty.ExpectMatch(jailcli.BaseCommand().Short)
	cancelFunc()
	<-done
}
