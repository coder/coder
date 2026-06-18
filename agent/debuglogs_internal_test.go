package agent

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
)

func TestHandleHTTPDebugLogsWithAfterCapsResponse(t *testing.T) {
	t.Parallel()

	logDir := t.TempDir()
	activePath := filepath.Join(logDir, "coder-agent.log")
	f, err := os.Create(activePath)
	require.NoError(t, err)
	// A huge active log must not starve the rotated logs.
	require.NoError(t, f.Truncate(debugLogsCombinedMaxBytes+1))
	require.NoError(t, f.Close())

	rotatedPath := filepath.Join(logDir, "coder-agent-2026-05-17T20-00-00.000.log")
	require.NoError(t, os.WriteFile(rotatedPath, []byte("rotated marker\n"), 0o600))
	rotatedModTime := time.Now().Add(-time.Minute)
	require.NoError(t, os.Chtimes(rotatedPath, rotatedModTime, rotatedModTime))

	a := &agent{
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug),
		logDir: logDir,
	}
	req := httptest.NewRequest(http.MethodGet, "/debug/logs?after="+time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano), nil)
	res := httptest.NewRecorder()

	a.HandleHTTPDebugLogs(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	body := res.Body.String()
	// Active is capped at its own limit, so the rotated log still fits.
	require.Less(t, int64(len(body)), int64(debugLogsCombinedMaxBytes))
	require.Contains(t, body, "coder-agent.log")
	require.Contains(t, body, "rotated marker")
}

func TestHandleHTTPDebugLogsWithAfterOpenFailure(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets only")
	}

	logDir, err := os.MkdirTemp("/tmp", "coder-debuglogs-")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(logDir)
	})
	activePath := filepath.Join(logDir, "coder-agent.log")
	listener, err := net.Listen("unix", activePath)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listener.Close()
	})

	a := &agent{
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug),
		logDir: logDir,
	}
	req := httptest.NewRequest(http.MethodGet, "/debug/logs?after="+time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano), nil)
	res := httptest.NewRecorder()

	a.HandleHTTPDebugLogs(res, req)

	require.Equal(t, http.StatusInternalServerError, res.Code)
	require.Contains(t, res.Body.String(), "could not open log file")
}

func TestRotatedAgentLogFilesReadsLogDirLiterally(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	logDir := filepath.Join(root, "logs[abc]")
	require.NoError(t, os.Mkdir(logDir, 0o700))
	activePath := filepath.Join(logDir, "coder-agent.log")
	rotatedPath := filepath.Join(logDir, "coder-agent-2026-05-18T00-00-00.000.log")
	require.NoError(t, os.WriteFile(activePath, []byte("active log"), 0o600))
	require.NoError(t, os.WriteFile(rotatedPath, []byte("rotated log"), 0o600))

	dirRoot, err := os.OpenRoot(logDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dirRoot.Close() })

	files, err := rotatedAgentLogFiles(t.Context(), slogtest.Make(t, nil), dirRoot, time.Now().Add(-time.Minute))

	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "coder-agent-2026-05-18T00-00-00.000.log", files[0].name)
}
