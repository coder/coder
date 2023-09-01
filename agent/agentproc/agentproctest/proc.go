package agentproctest

import (
	"fmt"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentproc"
	"github.com/coder/coder/v2/cryptorand"
)

func GenerateProcess(t *testing.T, fs afero.Fs, dir string) agentproc.Process {
	t.Helper()

	pid, err := cryptorand.Intn(1<<31 - 1)
	require.NoError(t, err)

	err = fs.MkdirAll(fmt.Sprintf("/%s/%d", dir, pid), 0555)
	require.NoError(t, err)

	arg1, err := cryptorand.String(5)
	require.NoError(t, err)

	arg2, err := cryptorand.String(5)
	require.NoError(t, err)

	arg3, err := cryptorand.String(5)
	require.NoError(t, err)

	cmdline := fmt.Sprintf("%s\x00%s\x00%s", arg1, arg2, arg3)

	err = afero.WriteFile(fs, fmt.Sprintf("/%s/%d/cmdline", dir, pid), []byte(cmdline), 0444)
	require.NoError(t, err)

	return agentproc.Process{
		PID:     int32(pid),
		CmdLine: cmdline,
		Dir:     fmt.Sprintf("%s/%d", dir, pid),
		FS:      fs,
	}
}
