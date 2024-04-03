package agentproctest

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentproc"
	"github.com/coder/coder/v2/cryptorand"
)

func GenerateProcess(t *testing.T, fs afero.Fs, muts ...func(*agentproc.Process)) agentproc.Process {
	t.Helper()

	pid, err := cryptorand.Intn(1<<31 - 1)
	require.NoError(t, err)

	arg1, err := cryptorand.String(5)
	require.NoError(t, err)

	arg2, err := cryptorand.String(5)
	require.NoError(t, err)

	arg3, err := cryptorand.String(5)
	require.NoError(t, err)

	cmdline := fmt.Sprintf("%s\x00%s\x00%s", arg1, arg2, arg3)

	process := agentproc.Process{
		CmdLine:     cmdline,
		PID:         int32(pid),
		OOMScoreAdj: 0,
	}

	for _, mut := range muts {
		mut(&process)
	}

	process.Dir = fmt.Sprintf("%s/%d", "/proc", process.PID)

	err = fs.MkdirAll(process.Dir, 0o555)
	require.NoError(t, err)

	err = afero.WriteFile(fs, fmt.Sprintf("%s/cmdline", process.Dir), []byte(process.CmdLine), 0o444)
	require.NoError(t, err)

	score := strconv.Itoa(process.OOMScoreAdj)
	err = afero.WriteFile(fs, fmt.Sprintf("%s/oom_score_adj", process.Dir), []byte(score), 0o444)
	require.NoError(t, err)

	return process
}
