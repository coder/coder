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
	_, err = f.WriteString("active log\n")
	require.NoError(t, err)
	require.NoError(t, f.Truncate(debugLogsCombinedMaxBytes+1))
	require.NoError(t, f.Close())

	a := &agent{
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug),
		logDir: logDir,
	}
	req := httptest.NewRequest(http.MethodGet, "/debug/logs?after="+time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano), nil)
	res := &countingResponseWriter{header: http.Header{}}

	a.HandleHTTPDebugLogs(res, req)

	require.Equal(t, http.StatusOK, res.status)
	require.Equal(t, int64(debugLogsCombinedMaxBytes), res.bytes)
	require.Contains(t, string(res.prefix), "coder-agent.log")
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

	files, err := rotatedAgentLogFiles(t.Context(), slogtest.Make(t, nil), logDir, time.Now().Add(-time.Minute))

	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "coder-agent-2026-05-18T00-00-00.000.log", filepath.Base(files[0].path))
}

type countingResponseWriter struct {
	header http.Header
	status int
	bytes  int64
	prefix []byte
}

func (w *countingResponseWriter) Header() http.Header {
	return w.header
}

func (w *countingResponseWriter) WriteHeader(status int) {
	w.status = status
}

func (w *countingResponseWriter) Write(p []byte) (int, error) {
	if len(w.prefix) < 1024 {
		remaining := 1024 - len(w.prefix)
		w.prefix = append(w.prefix, p[:min(len(p), remaining)]...)
	}
	w.bytes += int64(len(p))
	return len(p), nil
}

func (*countingResponseWriter) SetWriteDeadline(time.Time) error {
	return nil
}
