package catalog

import (
	"bufio"
	"context"
	"io"
	"time"

	"cdr.dev/slog/v3"
)

// LogWriter returns an io.WriteCloser that logs each line written
// to it at the given level. The caller must close the returned
// writer when done to terminate the internal goroutine.
func LogWriter(logger slog.Logger, level slog.Level, containerName string) io.WriteCloser {
	pr, pw := io.Pipe()
	go func() {
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			logger.Log(context.Background(), slog.SinkEntry{
				Time:    time.Now(),
				Level:   level,
				Message: scanner.Text(),
				Fields:  slog.M(slog.F("container", containerName)),
			})
		}
		_ = pr.Close()
	}()
	return pw
}
