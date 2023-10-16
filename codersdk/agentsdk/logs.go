package agentsdk

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/retry"
)

type startupLogsWriter struct {
	buf    bytes.Buffer // Buffer to track partial lines.
	ctx    context.Context
	send   func(ctx context.Context, log ...Log) error
	level  codersdk.LogLevel
	source uuid.UUID
}

func (w *startupLogsWriter) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		nl := bytes.IndexByte(p, '\n')
		if nl == -1 {
			break
		}
		cr := 0
		if nl > 0 && p[nl-1] == '\r' {
			cr = 1
		}

		var partial []byte
		if w.buf.Len() > 0 {
			partial = w.buf.Bytes()
			w.buf.Reset()
		}
		err := w.send(w.ctx, Log{
			CreatedAt: time.Now().UTC(), // UTC, like dbtime.Now().
			Level:     w.level,
			Output:    string(partial) + string(p[:nl-cr]),
		})
		if err != nil {
			return n - len(p), err
		}
		p = p[nl+1:]
	}
	if len(p) > 0 {
		_, err := w.buf.Write(p)
		if err != nil {
			return n - len(p), err
		}
	}
	return n, nil
}

func (w *startupLogsWriter) Close() error {
	if w.buf.Len() > 0 {
		defer w.buf.Reset()
		return w.send(w.ctx, Log{
			CreatedAt: time.Now().UTC(), // UTC, like dbtime.Now().
			Level:     w.level,
			Output:    w.buf.String(),
		})
	}
	return nil
}

// LogsWriter returns an io.WriteCloser that sends logs via the
// provided sender. The sender is expected to be non-blocking. Calling
// Close flushes any remaining partially written log lines but is
// otherwise no-op. If the context passed to LogsWriter is
// canceled, any remaining logs will be discarded.
//
// Neither Write nor Close is safe for concurrent use and must be used
// by a single goroutine.
func LogsWriter(ctx context.Context, sender func(ctx context.Context, log ...Log) error, source uuid.UUID, level codersdk.LogLevel) io.WriteCloser {
	return &startupLogsWriter{
		ctx:    ctx,
		send:   sender,
		level:  level,
		source: source,
	}
}

// LogsSenderFlushTimeout changes the default flush timeout (250ms),
// this is mostly useful for tests.
func LogsSenderFlushTimeout(timeout time.Duration) func(*logsSenderOptions) {
	return func(o *logsSenderOptions) {
		o.flushTimeout = timeout
	}
}

type logsSenderOptions struct {
	flushTimeout time.Duration
}

// LogsSender will send agent startup logs to the server. Calls to
// sendLog are non-blocking and will return an error if flushAndClose
// has been called. Calling sendLog concurrently is not supported. If
// the context passed to flushAndClose is canceled, any remaining logs
// will be discarded.
func LogsSender(sourceID uuid.UUID, patchLogs func(ctx context.Context, req PatchLogs) error, logger slog.Logger, opts ...func(*logsSenderOptions)) (sendLog func(ctx context.Context, log ...Log) error, flushAndClose func(context.Context) error) {
	o := logsSenderOptions{
		flushTimeout: 250 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(&o)
	}

	// The main context is used to close the sender goroutine and cancel
	// any outbound requests to the API. The shutdown context is used to
	// signal the sender goroutine to flush logs and then exit.
	ctx, cancel := context.WithCancel(context.Background())
	shutdownCtx, shutdown := context.WithCancel(ctx)

	// Synchronous sender, there can only be one outbound send at a time.
	sendDone := make(chan struct{})
	send := make(chan []Log, 1)
	go func() {
		// Set flushTimeout and backlogLimit so that logs are uploaded
		// once every 250ms or when 100 logs have been added to the
		// backlog, whichever comes first.
		backlogLimit := 100

		flush := time.NewTicker(o.flushTimeout)

		var backlog []Log
		defer func() {
			flush.Stop()
			if len(backlog) > 0 {
				logger.Warn(ctx, "startup logs sender exiting early, discarding logs", slog.F("discarded_logs_count", len(backlog)))
			}
			logger.Debug(ctx, "startup logs sender exited")
			close(sendDone)
		}()

		done := false
		for {
			flushed := false
			select {
			case <-ctx.Done():
				return
			case <-shutdownCtx.Done():
				done = true

				// Check queued logs before flushing.
				select {
				case logs := <-send:
					backlog = append(backlog, logs...)
				default:
				}
			case <-flush.C:
				flushed = true
			case logs := <-send:
				backlog = append(backlog, logs...)
				flushed = len(backlog) >= backlogLimit
			}

			if (done || flushed) && len(backlog) > 0 {
				flush.Stop() // Lower the chance of a double flush.

				// Retry uploading logs until successful or a specific
				// error occurs. Note that we use the main context here,
				// meaning these requests won't be interrupted by
				// shutdown.
				var err error
				for r := retry.New(time.Second, 5*time.Second); r.Wait(ctx); {
					err = patchLogs(ctx, PatchLogs{
						Logs:        backlog,
						LogSourceID: sourceID,
					})
					if err == nil {
						break
					}

					if errors.Is(err, context.Canceled) {
						break
					}
					// This error is expected to be codersdk.Error, but it has
					// private fields so we can't fake it in tests.
					var statusErr interface{ StatusCode() int }
					if errors.As(err, &statusErr) {
						if statusErr.StatusCode() == http.StatusRequestEntityTooLarge {
							logger.Warn(ctx, "startup logs too large, discarding logs", slog.F("discarded_logs_count", len(backlog)), slog.Error(err))
							err = nil
							break
						}
					}
					logger.Error(ctx, "startup logs sender failed to upload logs, retrying later", slog.F("logs_count", len(backlog)), slog.Error(err))
				}
				if err != nil {
					return
				}
				backlog = nil

				// Anchor flush to the last log upload.
				flush.Reset(o.flushTimeout)
			}
			if done {
				return
			}
		}
	}()

	var queue []Log
	sendLog = func(callCtx context.Context, log ...Log) error {
		select {
		case <-shutdownCtx.Done():
			return xerrors.Errorf("closed: %w", shutdownCtx.Err())
		case <-callCtx.Done():
			return callCtx.Err()
		case queue = <-send:
			// Recheck to give priority to context cancellation.
			select {
			case <-shutdownCtx.Done():
				return xerrors.Errorf("closed: %w", shutdownCtx.Err())
			case <-callCtx.Done():
				return callCtx.Err()
			default:
			}
			// Queue has not been captured by sender yet, re-use.
		default:
		}

		queue = append(queue, log...)
		send <- queue // Non-blocking.
		queue = nil

		return nil
	}
	flushAndClose = func(callCtx context.Context) error {
		defer cancel()
		shutdown()
		select {
		case <-sendDone:
			return nil
		case <-callCtx.Done():
			cancel()
			<-sendDone
			return callCtx.Err()
		}
	}
	return sendLog, flushAndClose
}
