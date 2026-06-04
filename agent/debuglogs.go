package agent

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

const (
	debugLogsActiveLimitBytes  = 10 * 1024 * 1024
	debugLogsRotatedLimitBytes = 100 * 1024 * 1024
)

var coderAgentBackupLogName = regexp.MustCompile(`^coder-agent-\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}\.\d{3}\.log$`)

type agentLogFile struct {
	path    string
	name    string
	modTime time.Time
	size    int64
}

func (a *agent) HandleHTTPDebugLogs(w http.ResponseWriter, r *http.Request) {
	after, ok, err := parseDebugLogsAfter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !ok {
		a.writeActiveDebugLog(w, r)
		return
	}

	disableWriteDeadline(w)
	files, err := agentDebugLogFiles(a.logDir, after)
	if err != nil {
		a.logger.Error(r.Context(), "find agent log files", slog.Error(err), slog.F("log_dir", a.logDir))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "could not find log files: %s", err)
		return
	}
	truncated := agentLogFilesSize(files) > debugLogsRotatedLimitBytes
	if truncated {
		w.Header().Set(codersdk.SupportBundleLogsTruncatedHeader, "true")
	}

	w.WriteHeader(http.StatusOK)
	remaining := int64(debugLogsRotatedLimitBytes)
	for i, file := range files {
		if remaining <= 0 {
			break
		}
		if i > 0 {
			if !writeLimitedString(w, "\n", &remaining) {
				break
			}
		}
		boundary := agentLogBoundary(file)
		if !writeLimitedString(w, boundary, &remaining) {
			break
		}
		f, err := os.Open(file.path)
		if err != nil {
			a.logger.Warn(r.Context(), "open agent log file", slog.Error(err), slog.F("path", file.path))
			continue
		}
		n, err := io.Copy(w, io.LimitReader(f, remaining))
		_ = f.Close()
		if err != nil && !errors.Is(err, io.EOF) {
			a.logger.Error(r.Context(), "read agent log file", slog.Error(err), slog.F("path", file.path))
			return
		}
		remaining -= n
	}
}

func parseDebugLogsAfter(r *http.Request) (time.Time, bool, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("after"))
	if raw == "" {
		return time.Time{}, false, nil
	}
	after, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false, xerrors.Errorf("after must be an RFC3339 timestamp: %w", err)
	}
	return after, true, nil
}

func (a *agent) writeActiveDebugLog(w http.ResponseWriter, r *http.Request) {
	logPath := filepath.Join(a.logDir, "coder-agent.log")
	f, err := os.Open(logPath)
	if err != nil {
		a.logger.Error(r.Context(), "open agent log file", slog.Error(err), slog.F("path", logPath))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "could not open log file: %s", err)
		return
	}
	defer f.Close()

	w.WriteHeader(http.StatusOK)
	_, err = io.Copy(w, io.LimitReader(f, debugLogsActiveLimitBytes))
	if err != nil && !errors.Is(err, io.EOF) {
		a.logger.Error(r.Context(), "read agent log file", slog.Error(err))
		return
	}
}

func agentDebugLogFiles(logDir string, after time.Time) ([]agentLogFile, error) {
	activePath := filepath.Join(logDir, "coder-agent.log")
	activeInfo, err := os.Stat(activePath)
	if err != nil {
		return nil, xerrors.Errorf("stat active log: %w", err)
	}
	files := []agentLogFile{{
		path:    activePath,
		name:    filepath.Base(activePath),
		modTime: activeInfo.ModTime(),
		size:    activeInfo.Size(),
	}}

	matches, err := filepath.Glob(filepath.Join(logDir, "coder-agent-*.log"))
	if err != nil {
		return nil, xerrors.Errorf("glob rotated logs: %w", err)
	}
	rotated := make([]agentLogFile, 0, len(matches))
	for _, match := range matches {
		base := filepath.Base(match)
		if !coderAgentBackupLogName.MatchString(base) {
			continue
		}
		info, err := os.Stat(match)
		if err != nil || !info.Mode().IsRegular() || info.ModTime().Before(after) {
			continue
		}
		rotated = append(rotated, agentLogFile{
			path:    match,
			name:    base,
			modTime: info.ModTime(),
			size:    info.Size(),
		})
	}
	slices.SortFunc(rotated, func(a, b agentLogFile) int {
		return b.modTime.Compare(a.modTime)
	})
	files = append(files, rotated...)
	return files, nil
}

func agentLogBoundary(file agentLogFile) string {
	return fmt.Sprintf("=== %s (mtime %s) ===\n", file.name, file.modTime.UTC().Format(time.RFC3339))
}

func agentLogFilesSize(files []agentLogFile) int64 {
	var size int64
	for i, file := range files {
		if i > 0 {
			size++
		}
		size += int64(len(agentLogBoundary(file)))
		size += file.size
	}
	return size
}

func writeLimitedString(w io.Writer, s string, remaining *int64) bool {
	if *remaining <= 0 {
		return false
	}
	if int64(len(s)) > *remaining {
		s = s[:int(*remaining)]
	}
	_, _ = io.WriteString(w, s)
	*remaining -= int64(len(s))
	return *remaining > 0
}

func disableWriteDeadline(w http.ResponseWriter) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
}
