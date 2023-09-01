package agentproc_test

import (
	"fmt"
	"strings"
	"syscall"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentproc"
	"github.com/coder/coder/v2/agent/agentproc/agentproctest"
)

func TestList(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			fs            = afero.NewMemMapFs()
			sc            = agentproctest.NewMockSyscaller(gomock.NewController(t))
			expectedProcs = make(map[int32]agentproc.Process)
			rootDir       = agentproc.DefaultProcDir
		)

		for i := 0; i < 4; i++ {
			proc := agentproctest.GenerateProcess(t, fs, rootDir)
			expectedProcs[proc.PID] = proc

			sc.EXPECT().
				Kill(proc.PID, syscall.Signal(0)).
				Return(nil)
		}

		actualProcs, err := agentproc.List(fs, sc, rootDir)
		require.NoError(t, err)
		require.Len(t, actualProcs, 4)
		for _, proc := range actualProcs {
			expected, ok := expectedProcs[proc.PID]
			require.True(t, ok)
			require.Equal(t, expected.PID, proc.PID)
			require.Equal(t, expected.CmdLine, proc.CmdLine)
			require.Equal(t, expected.Dir, proc.Dir)
		}
	})

	t.Run("FinishedProcess", func(t *testing.T) {
		t.Parallel()

		var (
			fs            = afero.NewMemMapFs()
			sc            = agentproctest.NewMockSyscaller(gomock.NewController(t))
			expectedProcs = make(map[int32]agentproc.Process)
			rootDir       = agentproc.DefaultProcDir
		)

		for i := 0; i < 3; i++ {
			proc := agentproctest.GenerateProcess(t, fs, rootDir)
			expectedProcs[proc.PID] = proc

			sc.EXPECT().
				Kill(proc.PID, syscall.Signal(0)).
				Return(nil)
		}

		// Create a process that's already finished. We're not adding
		// it to the map because it should be skipped over.
		proc := agentproctest.GenerateProcess(t, fs, rootDir)
		sc.EXPECT().
			Kill(proc.PID, syscall.Signal(0)).
			Return(xerrors.New("os: process already finished"))

		actualProcs, err := agentproc.List(fs, sc, rootDir)
		require.NoError(t, err)
		require.Len(t, actualProcs, 3)
		for _, proc := range actualProcs {
			expected, ok := expectedProcs[proc.PID]
			require.True(t, ok)
			require.Equal(t, expected.PID, proc.PID)
			require.Equal(t, expected.CmdLine, proc.CmdLine)
			require.Equal(t, expected.Dir, proc.Dir)
		}
	})

	t.Run("NoSuchProcess", func(t *testing.T) {
		t.Parallel()

		var (
			fs            = afero.NewMemMapFs()
			sc            = agentproctest.NewMockSyscaller(gomock.NewController(t))
			expectedProcs = make(map[int32]agentproc.Process)
			rootDir       = agentproc.DefaultProcDir
		)

		for i := 0; i < 3; i++ {
			proc := agentproctest.GenerateProcess(t, fs, rootDir)
			expectedProcs[proc.PID] = proc

			sc.EXPECT().
				Kill(proc.PID, syscall.Signal(0)).
				Return(nil)
		}

		// Create a process that doesn't exist. We're not adding
		// it to the map because it should be skipped over.
		proc := agentproctest.GenerateProcess(t, fs, rootDir)
		sc.EXPECT().
			Kill(proc.PID, syscall.Signal(0)).
			Return(syscall.ESRCH)

		actualProcs, err := agentproc.List(fs, sc, rootDir)
		require.NoError(t, err)
		require.Len(t, actualProcs, 3)
		for _, proc := range actualProcs {
			expected, ok := expectedProcs[proc.PID]
			require.True(t, ok)
			require.Equal(t, expected.PID, proc.PID)
			require.Equal(t, expected.CmdLine, proc.CmdLine)
			require.Equal(t, expected.Dir, proc.Dir)
		}
	})
}

// These tests are not very interesting but they provide some modicum of
// confidence.
func TestProcess(t *testing.T) {
	t.Parallel()

	t.Run("SetOOMAdj", func(t *testing.T) {
		t.Parallel()

		var (
			fs            = afero.NewMemMapFs()
			dir           = agentproc.DefaultProcDir
			proc          = agentproctest.GenerateProcess(t, fs, agentproc.DefaultProcDir)
			expectedScore = -1000
		)

		err := proc.SetOOMAdj(expectedScore)
		require.NoError(t, err)

		actualScore, err := afero.ReadFile(fs, fmt.Sprintf("%s/%d/oom_score_adj", dir, proc.PID))
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("%d", expectedScore), strings.TrimSpace(string(actualScore)))
	})

	t.Run("SetNiceness", func(t *testing.T) {
		t.Parallel()

		var (
			sc   = agentproctest.NewMockSyscaller(gomock.NewController(t))
			proc = &agentproc.Process{
				PID: 32,
			}
			score = 20
		)

		sc.EXPECT().SetPriority(proc.PID, score).Return(nil)
		err := proc.SetNiceness(sc, score)
		require.NoError(t, err)
	})

	t.Run("Name", func(t *testing.T) {
		t.Parallel()

		var (
			proc = &agentproc.Process{
				CmdLine: "helloworld\x00--arg1\x00--arg2",
			}
			expectedName = "helloworld"
		)

		require.Equal(t, expectedName, proc.Name())
	})
}
