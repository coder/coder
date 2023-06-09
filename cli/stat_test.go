package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/testutil"
)

// This just tests that the stat command is recognized and does not output
// an empty string. Actually testing the output of the stats command is
// fraught with all sorts of fun. Some more detailed testing of the stats
// output is performed in the tests in the clistat package.
func TestStatCmd(t *testing.T) {
	t.Parallel()
	t.Run("JSON", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)
		inv, _ := clitest.New(t, "stat", "--output=json")
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		s := buf.String()
		require.NotEmpty(t, s)
		// Must be valid JSON
		tmp := make([]struct{}, 0)
		require.NoError(t, json.NewDecoder(strings.NewReader(s)).Decode(&tmp))
	})
	t.Run("Table", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)
		inv, _ := clitest.New(t, "stat", "--output=table")
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		s := buf.String()
		require.NotEmpty(t, s)
		require.Contains(t, s, "HOST CPU")
		require.Contains(t, s, "HOST MEMORY")
		require.Contains(t, s, "HOME DISK")
		require.Contains(t, s, "UPTIME")
	})
	t.Run("Default", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)
		inv, _ := clitest.New(t, "stat")
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		s := buf.String()
		require.NotEmpty(t, s)
		require.Contains(t, s, "HOST CPU")
		require.Contains(t, s, "HOST MEMORY")
		require.Contains(t, s, "HOME DISK")
		require.Contains(t, s, "UPTIME")
	})
}
