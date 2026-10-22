package cli

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/adrg/xdg"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

const (
	// defaultCLIFlightRecorderSize is the default number of log entries below the
	// current log level that a command keeps in memory and emits to stderr when
	// it fails. It is intentionally small so a failure prints a few lines of
	// context rather than flooding the terminal.
	defaultCLIFlightRecorderSize = 10
	// maxCLIFlightRecorderSize caps the configurable flight recorder size to bound
	// memory use.
	maxCLIFlightRecorderSize = 10000
	// keepSessionLogFiles is the number of ssh session log files retained per log
	// directory; older files are pruned when a new session starts.
	keepSessionLogFiles = 50
)

// flightRecorder returns logger writing to sink at the current display level
// (Info by default, Debug under --verbose) with a flight recorder attached. The
// recorder keeps entries below the current level in memory and emits them when
// an error is logged, so the detail leading up to a failure is captured without
// logging it during normal operation. The recorder is always attached so it
// keeps working if the level changes; at Debug it simply has nothing to record.
func (r *RootCmd) flightRecorder(logger slog.Logger, sink slog.Sink, size int64) slog.Logger {
	level := slog.LevelInfo
	if r.verbose {
		level = slog.LevelDebug
	}
	return logger.AppendSinks(sink).Leveled(level).FlightRecorder(int(clampFlightRecorderSize(size)))
}

// defaultSessionLogDir returns the default directory for ssh session logs.
// Following the XDG Base Directory spec, logs are state data, so they live under
// the state directory rather than the cache or data directory.
func defaultSessionLogDir() string {
	return filepath.Join(xdg.StateHome, "coder", "logs")
}

// clampFlightRecorderSize bounds a requested size to [0, maxCLIFlightRecorderSize].
func clampFlightRecorderSize(size int64) int64 {
	if size < 0 {
		return 0
	}
	if size > maxCLIFlightRecorderSize {
		return maxCLIFlightRecorderSize
	}
	return size
}

// pruneSessionLogs removes the oldest coder-*.log files in dir, keeping at most
// keep of the most recent. It is best effort: it continues past individual
// failures and returns them joined so the caller can surface them.
func pruneSessionLogs(dir string, keep int) error {
	matches, err := filepath.Glob(filepath.Join(dir, "coder-*.log"))
	if err != nil {
		return xerrors.Errorf("glob session logs in %q: %w", dir, err)
	}
	type logFile struct {
		path    string
		modTime time.Time
	}
	var errs []error
	files := make([]logFile, 0, len(matches))
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			errs = append(errs, xerrors.Errorf("stat %q: %w", path, err))
			continue
		}
		files = append(files, logFile{path: path, modTime: info.ModTime()})
	}
	if len(files) <= keep {
		return errors.Join(errs...)
	}
	// Newest first, then remove everything past the keep threshold.
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	for _, f := range files[keep:] {
		if err := os.Remove(f.path); err != nil {
			errs = append(errs, xerrors.Errorf("remove %q: %w", f.path, err))
		}
	}
	return errors.Join(errs...)
}
