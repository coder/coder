//nolint:revive,gocritic,errname,unconvert
package log

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/enterprise/cli/boundary/config"
)

// SetupLogging creates a slog logger with the specified level
func SetupLogging(config config.AppConfig) (*slog.Logger, error) {
	var level slog.Level
	switch strings.ToLower(config.LogLevel) {
	case "error":
		level = slog.LevelError
	case "warn":
		level = slog.LevelWarn
	case "info":
		level = slog.LevelInfo
	case "debug":
		level = slog.LevelDebug
	default:
		level = slog.LevelWarn // Default to warn if invalid level
	}

	logTarget := os.Stderr

	logDir := config.LogDir
	if logDir != "" {
		// Set up the logging directory if it doesn't exist yet
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return nil, xerrors.Errorf("could not set up log dir %s: %v", logDir, err)
		}

		// Create a logfile (timestamp and pid to avoid race conditions with multiple boundary calls running)
		logFilePath := fmt.Sprintf("boundary-%s-%d.log",
			time.Now().Format("2006-01-02_15-04-05"),
			os.Getpid())

		logFile, err := os.Create(filepath.Join(logDir, logFilePath))
		if err != nil {
			return nil, xerrors.Errorf("could not create log file %s: %v", logFilePath, err)
		}

		// Set the log target to the file rather than stderr.
		logTarget = logFile
	}

	// Create a standard slog logger with the appropriate level
	handler := slog.NewTextHandler(logTarget, &slog.HandlerOptions{
		Level: level,
	})

	return slog.New(handler), nil
}
