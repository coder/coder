package cli_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/testutil"
)

// This just tests that the stat command is recognized and does not output
// an empty string. Actually testing the output of the stat command is
// fraught with all sorts of fun.
func TestStatCmd(t *testing.T) {
	t.Run("JSON", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)
		inv, _ := clitest.New(t, "stat", "--output=json")
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.NotEmpty(t, buf.String())
	})
	t.Run("Text", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)
		inv, _ := clitest.New(t, "stat", "--output=text")
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.NotEmpty(t, buf.String())
	})
}
