package agent

import (
	"context"
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
)

const (
	debugLogsActiveLimitBytes      = 10 * 1024 * 1024
	debugLogsWithRotatedLimitBytes = 100 * 1024 * 1024
)

var coderAgentRotatedLogPattern = regexp.MustCompile(`^coder-agent-\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}\.\d{3}\.log$`)

type agentLogFile struct {
	path    string
	modTime time.Time
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

	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		a.logger.Warn(r.Context(), "disable debug log write deadline", slog.Error(err))
	}
	files, err := agentDebugLogFiles(r.Context(), a.logger, a.logDir, after)
	if err != nil {
		a.logger.Error(r.Context(), "find agent log files", slog.Error(err), slog.F("log_dir", a.logDir))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "could not find log files: %s", err)
		return
	}
	remaining := int64(debugLogsWithRotatedLimitBytes)
	for i, file := range files {
		if remaining <= 0 {
			break
		}
		f, err := os.Open(file.path)
		if err != nil {
			// The active log is always first and must be readable.
			if i == 0 {
				a.logger.Error(r.Context(), "open agent log file", slog.Error(err), slog.F("path", file.path))
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = fmt.Fprintf(w, "could not open log file: %s", err)
				return
			}
			a.logger.Warn(r.Context(), "open rotated agent log file", slog.Error(err), slog.F("path", file.path))
			continue
		}
		if i == 0 {
			w.WriteHeader(http.StatusOK)
		}
		// Each file is prefixed with a header naming it. Headers count
		// against the byte cap so the response never exceeds it.
		header := fmt.Sprintf("=== %s (mtime %s) ===\n", filepath.Base(file.path), file.modTime.UTC().Format(time.RFC3339))
		if i > 0 {
			header = "\n" + header
		}
		if int64(len(header)) > remaining {
			_ = f.Close()
			break
		}
		if _, err := io.WriteString(w, header); err != nil {
			_ = f.Close()
			a.logger.Error(r.Context(), "write agent log header", slog.Error(err), slog.F("path", file.path))
			return
		}
		remaining -= int64(len(header))
		n, err := io.Copy(w, io.LimitReader(f, remaining))
		remaining -= n
		_ = f.Close()
		if err != nil {
			a.logger.Error(r.Context(), "read agent log file", slog.Error(err), slog.F("path", file.path))
			return
		}
	}
	if remaining <= 0 {
		a.logger.Warn(r.Context(), "agent debug logs response truncated", slog.F("limit_bytes", debugLogsWithRotatedLimitBytes))
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
	if err != nil {
		a.logger.Error(r.Context(), "read agent log file", slog.Error(err))
		return
	}
}

func agentDebugLogFiles(ctx context.Context, logger slog.Logger, logDir string, after time.Time) ([]agentLogFile, error) {
	activePath := filepath.Join(logDir, "coder-agent.log")
	activeInfo, err := os.Stat(activePath)
	if err != nil {
		return nil, xerrors.Errorf("stat active log: %w", err)
	}
	files := []agentLogFile{{
		path:    activePath,
		modTime: activeInfo.ModTime(),
	}}

	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil, xerrors.Errorf("read log directory: %w", err)
	}
	rotated := make([]agentLogFile, 0, len(entries))
	for _, entry := range entries {
		base := entry.Name()
		if !coderAgentRotatedLogPattern.MatchString(base) {
			continue
		}
		path := filepath.Join(logDir, base)
		info, err := os.Stat(path)
		if err != nil {
			logger.Warn(ctx, "stat rotated agent log file", slog.Error(err), slog.F("path", path))
			continue
		}
		if !info.Mode().IsRegular() || info.ModTime().Before(after) {
			continue
		}
		rotated = append(rotated, agentLogFile{
			path:    path,
			modTime: info.ModTime(),
		})
	}
	slices.SortFunc(rotated, func(a, b agentLogFile) int {
		return b.modTime.Compare(a.modTime)
	})
	files = append(files, rotated...)
	return files, nil
}
