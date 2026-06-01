package cli

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestCmdExec(t *testing.T) {
	ctx := testutil.Context(t, testutil.WaitShort)
	out, err := exec.CommandContext(ctx, "cmd.exe", "/c", "dir").CombinedOutput()
	t.Log(string(out))
	require.Error(t, err)
}
