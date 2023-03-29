package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
)

func TestStat(t *testing.T) {
	t.Parallel()

	t.Run("cpu", func(t *testing.T) {
		t.Parallel()
		inv, _ := clitest.New(t, "stat", "cpu")
		var out bytes.Buffer
		inv.Stdout = &out
		clitest.Run(t, inv)

		require.Regexp(t, `^[\d]{2}\n$`, out.String())
	})
}
