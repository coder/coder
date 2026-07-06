package agent

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

const (
	activeAgentLogName        = "coder-agent.log"
	debugLogsActiveMaxBytes   = 10 * 1024 * 1024
	debugLogsCombinedMaxBytes = 100 * 1024 * 1024
	// debugLogsWriteTimeout gives slow links well over the server's 20s
	// WriteTimeout to stream the combined logs.
	debugLogsWriteTimeout = 5 * time.Minute
)

// coderAgentRotatedLogPattern matches lumberjack's rotated filenames, e.g.
// coder-agent-2026-05-17T20-00-00.000.log.
var coderAgentRotatedLogPattern = regexp.MustCompile(`^coder-agent-\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}\.\d{3}\.log$`)

type agentLogFile struct {
	name    string
	size    int64
	modTime time.Time
}

func (a *agent) HandleHTTPDebugLogs(w http.ResponseWriter, r *http.Request) {
	after, hasAfter, err := parseDebugLogsAfter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Confine reads to logDir so a symlink there cannot escape it.
	root, err := os.OpenRoot(a.logDir)
	if err != nil {
		a.logger.Error(r.Context(), "open agent log dir", slog.Error(err), slog.F("log_dir", a.logDir))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "could not open log dir: %s", err)
		return
	}
	defer root.Close()

	if !hasAfter {
		a.writeActiveDebugLog(w, r, root)
		return
	}

	// Streaming the combined logs can exceed the server's 20s WriteTimeout,
	// so extend the deadline for this response.
	extendDebugLogWriteDeadline(r.Context(), a.logger, w)

	// Open the required active log before the 200 so failures return 500.
	active, err := root.Open(activeAgentLogName)
	if err != nil {
		a.logger.Error(r.Context(), "open agent log file", slog.Error(err), slog.F("name", activeAgentLogName))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "could not open log file: %s", err)
		return
	}
	activeInfo, err := active.Stat()
	if err != nil {
		_ = active.Close()
		a.logger.Error(r.Context(), "stat agent log file", slog.Error(err), slog.F("name", activeAgentLogName))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "could not stat log file: %s", err)
		return
	}
	w.WriteHeader(http.StatusOK)
	remaining := int64(debugLogsCombinedMaxBytes)
	// Cap the active log at its own limit so it can't consume the whole
	// budget and starve the rotated logs.
	n, truncated, err := writeAgentLogSection(w, active, activeAgentLogName, activeInfo.Size(), activeInfo.ModTime(), "", min(remaining, debugLogsActiveMaxBytes))
	remaining -= n
	_ = active.Close()
	if err != nil {
		a.logger.Error(r.Context(), "read agent log file", slog.Error(err), slog.F("name", activeAgentLogName))
		return
	}

	// Then rotated logs after the cutoff, newest first.
	rotated, err := rotatedAgentLogFiles(r.Context(), a.logger, root, after)
	if err != nil {
		a.logger.Error(r.Context(), "find rotated agent log files", slog.Error(err), slog.F("log_dir", a.logDir))
		return
	}
	for _, file := range rotated {
		if remaining <= 0 {
			truncated = true
			break
		}
		f, err := root.Open(file.name)
		if err != nil {
			a.logger.Warn(r.Context(), "open rotated agent log file", slog.Error(err), slog.F("name", file.name))
			continue
		}
		var fileTruncated bool
		n, fileTruncated, err = writeAgentLogSection(w, f, file.name, file.size, file.modTime, "\n", remaining)
		remaining -= n
		truncated = truncated || fileTruncated
		_ = f.Close()
		if err != nil {
			a.logger.Error(r.Context(), "read rotated agent log file", slog.Error(err), slog.F("name", file.name))
			return
		}
	}
	if truncated {
		a.logger.Warn(r.Context(), "agent debug logs response truncated", slog.F("limit_bytes", debugLogsCombinedMaxBytes))
	}
}

func parseDebugLogsAfter(r *http.Request) (after time.Time, hasAfter bool, err error) {
	raw := strings.TrimSpace(r.URL.Query().Get("after"))
	if raw == "" {
		return time.Time{}, false, nil
	}
	after, err = time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false, xerrors.Errorf("after must be an RFC3339 timestamp: %w", err)
	}
	return after, true, nil
}

func (a *agent) writeActiveDebugLog(w http.ResponseWriter, r *http.Request, root *os.Root) {
	f, err := root.Open(activeAgentLogName)
	if err != nil {
		a.logger.Error(r.Context(), "open agent log file", slog.Error(err), slog.F("name", activeAgentLogName))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "could not open log file: %s", err)
		return
	}
	defer f.Close()

	w.WriteHeader(http.StatusOK)
	_, err = io.Copy(w, io.LimitReader(f, debugLogsActiveMaxBytes))
	if err != nil {
		a.logger.Error(r.Context(), "read agent log file", slog.Error(err))
		return
	}
}

// extendDebugLogWriteDeadline gives slow links well over the server's 20s
// WriteTimeout to stream a debug log response.
func extendDebugLogWriteDeadline(ctx context.Context, logger slog.Logger, w http.ResponseWriter) {
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(debugLogsWriteTimeout)); err != nil {
		logger.Warn(ctx, "extend debug log write deadline", slog.Error(err))
	}
}

// writeAgentLogSection writes a separator and header for the file, then streams
// up to budget bytes of r. It returns the bytes written and, from size (r's
// full length), whether r was truncated.
func writeAgentLogSection(w io.Writer, r io.Reader, name string, size int64, modTime time.Time, separator string, budget int64) (written int64, truncated bool, err error) {
	header := separator + fmt.Sprintf("=== %s (mtime %s) ===\n", name, modTime.UTC().Format(time.RFC3339Nano))
	if int64(len(header)) > budget {
		return 0, size > 0, nil
	}
	if _, err := io.WriteString(w, header); err != nil {
		return 0, false, err
	}
	contentBudget := budget - int64(len(header))
	n, err := io.Copy(w, io.LimitReader(r, contentBudget))
	return int64(len(header)) + n, size > contentBudget, err
}

// rotatedAgentLogFiles returns rotated logs after the cutoff, newest first,
// excluding the active log and any non-regular files such as symlinks.
func rotatedAgentLogFiles(ctx context.Context, logger slog.Logger, root *os.Root, after time.Time) ([]agentLogFile, error) {
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, xerrors.Errorf("read log directory: %w", err)
	}
	rotated := make([]agentLogFile, 0, len(entries))
	for _, entry := range entries {
		base := entry.Name()
		if !coderAgentRotatedLogPattern.MatchString(base) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			logger.Warn(ctx, "stat rotated agent log file", slog.Error(err), slog.F("name", base))
			continue
		}
		if !info.Mode().IsRegular() || info.ModTime().Before(after) {
			continue
		}
		rotated = append(rotated, agentLogFile{
			name:    base,
			size:    info.Size(),
			modTime: info.ModTime(),
		})
	}
	slices.SortFunc(rotated, func(a, b agentLogFile) int {
		return b.modTime.Compare(a.modTime)
	})
	return rotated, nil
}
