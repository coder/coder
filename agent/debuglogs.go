package agent

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

const (
	debugLogsActiveLimitBytes        = 10 * 1024 * 1024
	debugLogsRotatedLimitBytes       = 100 * 1024 * 1024
	debugLogFilesDefaultMaxAge       = 72 * time.Hour
	debugLogFilesMaxFiles            = 200
	debugLogFilesMaxBytes      int64 = 200 * 1024 * 1024
)

var coderAgentBackupLogName = regexp.MustCompile(`^coder-agent-\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}\.\d{3}\.log$`)

type agentLogFile struct {
	path    string
	name    string
	modTime time.Time
	size    int64
}

func (a *agent) HandleHTTPDebugLogs(w http.ResponseWriter, r *http.Request) {
	disableWriteDeadline(w)

	var includeRotated bool
	if raw := r.URL.Query().Get("include_rotated"); raw != "" {
		var err error
		includeRotated, err = strconv.ParseBool(raw)
		if err != nil {
			http.Error(w, "include_rotated must be a boolean", http.StatusBadRequest)
			return
		}
	}

	if !includeRotated {
		a.writeActiveDebugLog(w, r)
		return
	}

	files, err := agentDebugLogFiles(a.logDir)
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

func agentDebugLogFiles(logDir string) ([]agentLogFile, error) {
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
		if err != nil || !info.Mode().IsRegular() {
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

type debugLogFile struct {
	path    string
	relPath string
	modTime time.Time
	size    int64
}

func (a *agent) HandleHTTPDebugLogFiles(w http.ResponseWriter, r *http.Request) {
	disableWriteDeadline(w)

	maxAge, err := parseDebugLogFilesMaxAge(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		a.logger.Error(r.Context(), "resolve home directory", slog.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "could not resolve home directory: %s", err)
		return
	}
	if home == "" {
		a.logger.Error(r.Context(), "resolve home directory", slog.Error(xerrors.New("empty home directory")))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, "could not resolve home directory")
		return
	}
	home, err = filepath.Abs(home)
	if err != nil {
		a.logger.Error(r.Context(), "resolve home directory", slog.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "could not resolve home directory: %s", err)
		return
	}

	patterns := debugLogFilePatterns(home)
	if manifest := a.manifest.Load(); manifest != nil {
		patterns = append(patterns, manifest.SupportBundleAdditionalLogPaths...)
	}
	files, truncated, err := collectDebugLogFiles(r.Context(), a.logger, patterns, home, a.clock.Now().Add(-maxAge))
	if err != nil {
		a.logger.Error(r.Context(), "collect debug log files", slog.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "could not collect log files: %s", err)
		return
	}
	if truncated {
		w.Header().Set(codersdk.SupportBundleLogsTruncatedHeader, "true")
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.WriteHeader(http.StatusOK)
	if err := writeDebugLogFilesArchive(w, files); err != nil {
		a.logger.Error(r.Context(), "write debug log files archive", slog.Error(err))
		return
	}
}

func disableWriteDeadline(w http.ResponseWriter) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
}

func parseDebugLogFilesMaxAge(r *http.Request) (time.Duration, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("max_age"))
	if raw == "" {
		return debugLogFilesDefaultMaxAge, nil
	}
	maxAge, err := time.ParseDuration(raw)
	if err != nil {
		return 0, xerrors.Errorf("max_age must be a duration: %w", err)
	}
	if maxAge <= 0 {
		return 0, xerrors.New("max_age must be greater than zero")
	}
	return maxAge, nil
}

func debugLogFilePatterns(home string) []string {
	patterns := []string{filepath.Join(home, ".*-server", "data", "logs")}
	if xdgDataHome := os.Getenv("XDG_DATA_HOME"); xdgDataHome != "" {
		patterns = append(patterns, filepath.Join(xdgDataHome, "code-server", "coder-logs"))
	} else {
		patterns = append(patterns, filepath.Join(home, ".local", "share", "code-server", "coder-logs"))
	}
	return patterns
}

func collectDebugLogFiles(ctx context.Context, logger slog.Logger, patterns []string, home string, cutoff time.Time) ([]debugLogFile, bool, error) {
	seen := map[string]struct{}{}
	files := make([]debugLogFile, 0)
	var totalBytes int64
	truncated := false
	currentPattern := ""
	addFile := func(p string, info os.FileInfo) {
		if truncated {
			return
		}
		file, outsideHome, ok := debugLogFileFromPath(p, info, home, cutoff)
		if outsideHome {
			logger.Warn(ctx, "support bundle log file outside home ignored",
				slog.F("path", p), slog.F("pattern", currentPattern), slog.F("home", home))
		}
		if !ok {
			return
		}
		if _, ok := seen[file.path]; ok {
			return
		}
		if len(files) >= debugLogFilesMaxFiles || totalBytes+file.size > debugLogFilesMaxBytes {
			truncated = true
			return
		}
		seen[file.path] = struct{}{}
		files = append(files, file)
		totalBytes += file.size
	}

	for _, pattern := range patterns {
		if truncated {
			break
		}
		pattern = strings.TrimSpace(os.ExpandEnv(pattern))
		if pattern == "" {
			continue
		}
		currentPattern = pattern
		matches, err := filepath.Glob(pattern)
		if err != nil {
			logger.Warn(ctx, "support bundle log file pattern ignored", slog.F("pattern", pattern), slog.Error(err))
			continue
		}
		for _, match := range matches {
			if truncated {
				break
			}
			info, err := os.Lstat(match)
			if err != nil {
				continue
			}
			switch {
			case info.IsDir():
				err = filepath.WalkDir(match, func(p string, d os.DirEntry, err error) error {
					if err != nil || d.IsDir() {
						return nil
					}
					info, err := d.Info()
					if err != nil {
						return nil
					}
					addFile(p, info)
					if truncated {
						return filepath.SkipAll
					}
					return nil
				})
				if err != nil {
					return nil, false, xerrors.Errorf("walk %q: %w", match, err)
				}
			case info.Mode().IsRegular():
				addFile(match, info)
			}
		}
	}
	slices.SortFunc(files, func(a, b debugLogFile) int {
		return strings.Compare(a.relPath, b.relPath)
	})
	return files, truncated, nil
}

func debugLogFileFromPath(p string, info os.FileInfo, home string, cutoff time.Time) (file debugLogFile, outsideHome bool, ok bool) {
	if !info.Mode().IsRegular() {
		return debugLogFile{}, false, false
	}
	if filepath.Ext(p) != ".log" {
		return debugLogFile{}, false, false
	}
	if info.ModTime().Before(cutoff) {
		return debugLogFile{}, false, false
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return debugLogFile{}, false, false
	}
	rel, ok := safeHomeRelPath(home, abs)
	if !ok {
		return debugLogFile{}, true, false
	}
	return debugLogFile{path: abs, relPath: rel, modTime: info.ModTime(), size: info.Size()}, false, true
}

func safeHomeRelPath(home, p string) (string, bool) {
	rel, err := filepath.Rel(home, p)
	if err != nil || rel == "." || rel == "" {
		return "", false
	}
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	clean := path.Clean(filepath.ToSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", false
	}
	return clean, true
}

func writeDebugLogFilesArchive(w io.Writer, files []debugLogFile) error {
	gzw := gzip.NewWriter(w)
	tw := tar.NewWriter(gzw)
	for _, file := range files {
		f, err := os.Open(file.path)
		if err != nil {
			continue
		}
		info, err := f.Stat()
		if err != nil {
			_ = f.Close()
			continue
		}
		hdr := &tar.Header{
			Name:    file.relPath,
			Mode:    0o600,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			_ = f.Close()
			return xerrors.Errorf("write tar header for %s: %w", file.relPath, err)
		}
		_, copyErr := io.Copy(tw, io.LimitReader(f, info.Size()))
		closeErr := f.Close()
		if copyErr != nil {
			return xerrors.Errorf("write tar data for %s: %w", file.relPath, copyErr)
		}
		if closeErr != nil {
			return xerrors.Errorf("close %s: %w", file.path, closeErr)
		}
	}
	if err := tw.Close(); err != nil {
		return xerrors.Errorf("close tar writer: %w", err)
	}
	if err := gzw.Close(); err != nil {
		return xerrors.Errorf("close gzip writer: %w", err)
	}
	return nil
}
