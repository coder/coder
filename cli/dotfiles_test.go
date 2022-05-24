package cli_test

import (
	"testing"

	"github.com/coder/coder/cli/clitest"
	"github.com/stretchr/testify/assert"
)

func TestDotfiles(t *testing.T) {
	t.Run("MissingArg", func(t *testing.T) {
		t.Parallel()
		cmd, _ := clitest.New(t, "dotfiles")
		err := cmd.Execute()
		assert.Error(t, err)
	})
}
