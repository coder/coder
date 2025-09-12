package cli_test

import (
	"testing"

	"github.com/coder/coder/v2/cli/clitest"
)

// Actually testing the functionality of coder/jail takes place in the
// coder/jail repo, since it's a dependency of coder.
// Here we want to test basically that integrating it as a subcommand doesn't break anything.
func TestJailSubcommand(t *testing.T) {
	_, _ = clitest.New(t, "jail", "--help")
}
