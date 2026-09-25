package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/adrg/xdg"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/serpent"
)

const (
	// defaultCLILogBufferSize is the default number of below-level (debug) log
	// entries a session command keeps in memory and writes to its log file on a
	// connection failure.
	defaultCLILogBufferSize = 1000
	maxCLILogBufferSize = 10000
	// keepSessionLogFiles is the number of session log files retained per log
	// directory; older files are pruned when a new session starts.
	keepSessionLogFiles = 50
)

// logBufferSizeOption returns the shared --log-buffer-size option used by the
// session commands (ssh, start, stop, update).
func logBufferSizeOption(value *int64) serpent.Option {
	return serpent.Option{
		Flag:    "log-buffer-size",
		Env:     "CODER_LOG_BUFFER_SIZE",
		Default: strconv.Itoa(defaultCLILogBufferSize),
		Description: "Number of debug log entries to keep in memory and write to the " +
			"session log file if the command fails. Set to 0 to disable buffering. " +
			"Ignored with --verbose, which writes debug logs unconditionally.",
		Value: serpent.Int64Of(value),
	}
}

// logDirOption returns the --log-dir option for a session command. The env
// differs per command so ssh keeps its historical CODER_SSH_LOG_DIR.
func logDirOption(value *string, env string) serpent.Option {
	return serpent.Option{
		Flag: "log-dir",
		Env:  env,
		Description: "Directory to write session diagnostic log files to. " +
			"Defaults to the user state directory.",
		Value: serpent.StringOf(value),
	}
}

// defaultSessionLogDir returns the default directory for CLI session logs.
// Following the XDG Base Directory spec, logs are state data, so they live under
// the state directory rather than the cache or data directory.
func defaultSessionLogDir() string {
	return filepath.Join(xdg.StateHome, "coder", "logs")
}

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

// newSessionLogger builds the logger for a session command. It writes to a
// per-invocation file in logDir (defaulting to the user state directory) and,
// unless verbose is enabled, runs at Info while keeping a rolling in-memory
// history of debug entries via a flight recorder. The history is flushed to the
// file automatically when the command logs an error on exit, so the detail
// leading up to a failure is available without writing debug logs during normal
// operation. Verbose runs at Debug and bypasses the recorder.
//
// The returned closer must be called when the command finishes to flush and
// close the log file.
func (r *RootCmd) newSessionLogger(inv *serpent.Invocation, cmdName, logDir string, bufferSize int64) (slog.Logger, func(), error) {
	if logDir == "" {
		logDir = defaultSessionLogDir()
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return slog.Logger{}, nil, xerrors.Errorf("create log dir %q: %w", logDir, err)
	}
	pruneSessionLogs(logDir, keepSessionLogFiles)

	nonce, err := cryptorand.StringCharset(cryptorand.Lower, 5)
	if err != nil {
		return slog.Logger{}, nil, xerrors.Errorf("generate log file nonce: %w", err)
	}
	// The time portion makes it easier to find the right log file, and the nonce
	// prevents collisions between invocations that start in the same second.
	name := fmt.Sprintf("coder-%s-%s-%s.log", cmdName, time.Now().Format("20060102-150405"), nonce)

	logFile, err := os.OpenFile(
		filepath.Join(logDir, name),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_EXCL,
		0o600,
	)
	if err != nil {
		return slog.Logger{}, nil, xerrors.Errorf("open log file: %w", err)
	}
	dc := cliutil.DiscardAfterClose(logFile)

	logger := inv.Logger.AppendSinks(sloghuman.Sink(dc))
	if r.verbose {
		logger = logger.Leveled(slog.LevelDebug)
	} else {
		logger = logger.Leveled(slog.LevelInfo).FlightRecorder(int(clampLogBufferSize(bufferSize)))
	}
	return logger, func() { _ = dc.Close() }, nil
}
