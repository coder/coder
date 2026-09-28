package cli

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/adrg/xdg"

	"cdr.dev/slog/v3"
)

const (
	// defaultCLILogBufferSize is the default number of log entries below the
	// current log level that a command keeps in memory and emits to stderr when
	// it fails. It is intentionally small so a failure prints a few lines of
	// context rather than flooding the terminal.
	defaultCLILogBufferSize = 10
	// maxCLILogBufferSize caps the configurable buffer size to bound memory use.
	maxCLILogBufferSize = 10000
	// keepSessionLogFiles is the number of ssh session log files retained per log
	// directory; older files are pruned when a new session starts.
	keepSessionLogFiles = 50
)

// bufferedLogger returns logger writing to sink at the current display level
// (Info by default, Debug under --verbose) with a flight recorder attached. The
// recorder buffers entries below the current level and emits them when an error
// is logged, so the detail leading up to a failure is captured without logging
// it during normal operation. The recorder is always attached so buffering keeps
// working if the level changes; at Debug it simply has nothing to buffer.
func (r *RootCmd) bufferedLogger(logger slog.Logger, sink slog.Sink, bufferSize int64) slog.Logger {
	level := slog.LevelInfo
	if r.verbose {
		level = slog.LevelDebug
	}
	return logger.AppendSinks(sink).Leveled(level).FlightRecorder(int(clampLogBufferSize(bufferSize)))
}

// defaultSessionLogDir returns the default directory for ssh session logs.
// Following the XDG Base Directory spec, logs are state data, so they live under
// the state directory rather than the cache or data directory.
func defaultSessionLogDir() string {
	return filepath.Join(xdg.StateHome, "coder", "logs")
}

// clampLogBufferSize bounds a requested buffer size to [0, maxCLILogBufferSize].
func clampLogBufferSize(size int64) int64 {
	if size < 0 {
		return 0
	}
	if size > maxCLILogBufferSize {
		return maxCLILogBufferSize
	}
	return size
}

// pruneSessionLogs removes the oldest coder-*.log files in dir, keeping at most
// keep of the most recent. Errors are ignored: pruning is best effort.
func pruneSessionLogs(dir string, keep int) {
	matches, err := filepath.Glob(filepath.Join(dir, "coder-*.log"))
	if err != nil {
		return
	}
	type logFile struct {
		path    string
		modTime time.Time
	}
	files := make([]logFile, 0, len(matches))
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		files = append(files, logFile{path: path, modTime: info.ModTime()})
	}
	if len(files) <= keep {
		return
	}
	// Newest first, then remove everything past the keep threshold.
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	for _, f := range files[keep:] {
		_ = os.Remove(f.path)
	}
}
