package agentsdk

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/retry"
)

const (
	flushInterval    = time.Second
	maxBytesPerBatch = 1 << 20 // 1MiB
	overheadPerLog   = 21      // found by testing

	// maxBytesQueued is the maximum length of logs we will queue in memory.  The number is taken
	// from dump.sql `max_logs_length` constraint, as there is no point queuing more logs than we'll
	// accept in the database.
	maxBytesQueued = 1048576
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
//
// Deprecated: Use NewLogSender instead, based on the v2 Agent API.
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

type logQueue struct {
	logs           []*proto.Log
	flushRequested bool
	lastFlush      time.Time
}

// LogSender is a component that handles enqueuing logs and then sending them over the agent API.
// Things that need to log call Enqueue and Flush.  When the agent API becomes available, call
// SendLoop to send pending logs.
type LogSender struct {
	*sync.Cond
	queues           map[uuid.UUID]*logQueue
	logger           slog.Logger
	exceededLogLimit bool
	outputLen        int
}

type logDest interface {
	BatchCreateLogs(ctx context.Context, request *proto.BatchCreateLogsRequest) (*proto.BatchCreateLogsResponse, error)
}

func NewLogSender(logger slog.Logger) *LogSender {
	return &LogSender{
		Cond:   sync.NewCond(&sync.Mutex{}),
		logger: logger,
		queues: make(map[uuid.UUID]*logQueue),
	}
}

func (l *LogSender) Enqueue(src uuid.UUID, logs ...Log) {
	logger := l.logger.With(slog.F("log_source_id", src))
	if len(logs) == 0 {
		logger.Debug(context.Background(), "enqueue called with no logs")
		return
	}
	l.L.Lock()
	defer l.L.Unlock()
	if l.exceededLogLimit {
		logger.Warn(context.Background(), "dropping enqueued logs because we have reached the server limit")
		// don't error, as we also write to file and don't want the overall write to fail
		return
	}
	defer l.Broadcast()
	q, ok := l.queues[src]
	if !ok {
		q = &logQueue{}
		l.queues[src] = q
	}
	for k, log := range logs {
		// Here we check the queue size before adding a log because we want to queue up slightly
		// more logs than the database would store to ensure we trigger "logs truncated" at the
		// database layer.  Otherwise, the end user wouldn't know logs are truncated unless they
		// examined the Coder agent logs.
		if l.outputLen > maxBytesQueued {
			logger.Warn(context.Background(), "log queue full; truncating new logs", slog.F("new_logs", k), slog.F("queued_logs", len(q.logs)))
			return
		}
		pl, err := ProtoFromLog(log)
		if err != nil {
			logger.Critical(context.Background(), "failed to convert log", slog.Error(err))
			pl = &proto.Log{
				CreatedAt: timestamppb.Now(),
				Level:     proto.Log_ERROR,
				Output:    "**Coder Internal Error**: Failed to convert log",
			}
		}
		if len(pl.Output)+overheadPerLog > maxBytesPerBatch {
			logger.Warn(context.Background(), "dropping log line that exceeds our limit", slog.F("len", len(pl.Output)))
			continue
		}
		q.logs = append(q.logs, pl)
		l.outputLen += len(pl.Output)
	}
	logger.Debug(context.Background(), "enqueued agent logs", slog.F("new_logs", len(logs)), slog.F("queued_logs", len(q.logs)))
}

func (l *LogSender) Flush(src uuid.UUID) {
	l.L.Lock()
	defer l.L.Unlock()
	defer l.Broadcast()
	q, ok := l.queues[src]
	if ok {
		q.flushRequested = true
	}
	// queue might not exist because it's already been flushed and removed from
	// the map.
}

var LogLimitExceededError = xerrors.New("Log limit exceeded")

// SendLoop sends any pending logs until it hits an error or the context is canceled.  It does not
// retry as it is expected that a higher layer retries establishing connection to the agent API and
// calls SendLoop again.
func (l *LogSender) SendLoop(ctx context.Context, dest logDest) error {
	l.L.Lock()
	defer l.L.Unlock()
	if l.exceededLogLimit {
		l.logger.Debug(ctx, "aborting SendLoop because log limit is already exceeded")
		return LogLimitExceededError
	}

	ctxDone := false
	defer l.logger.Debug(ctx, "log sender send loop exiting")

	// wake 4 times per Flush interval to check if anything needs to be flushed
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		tkr := time.NewTicker(flushInterval / 4)
		defer tkr.Stop()
		for {
			select {
			// also monitor the context here, so we notice immediately, rather
			// than waiting for the next tick or logs
			case <-ctx.Done():
				l.L.Lock()
				ctxDone = true
				l.L.Unlock()
				l.Broadcast()
				return
			case <-tkr.C:
				l.Broadcast()
			}
		}
	}()

	for {
		for !ctxDone && !l.hasPendingWorkLocked() {
			l.Wait()
		}
		if ctxDone {
			return ctx.Err()
		}

		src, q := l.getPendingWorkLocked()
		logger := l.logger.With(slog.F("log_source_id", src))
		q.flushRequested = false // clear flag since we're now flushing
		req := &proto.BatchCreateLogsRequest{
			LogSourceId: src[:],
		}

		// outputToSend keeps track of the size of the protobuf message we send, while
		// outputToRemove keeps track of the size of the output we'll remove from the queues on
		// success.  They are different because outputToSend also counts protocol message overheads.
		outputToSend := 0
		outputToRemove := 0
		n := 0
		for n < len(q.logs) {
			log := q.logs[n]
			outputToSend += len(log.Output) + overheadPerLog
			if outputToSend > maxBytesPerBatch {
				break
			}
			req.Logs = append(req.Logs, log)
			n++
			outputToRemove += len(log.Output)
		}

		l.L.Unlock()
		logger.Debug(ctx, "sending logs to agent API", slog.F("num_logs", len(req.Logs)))
		resp, err := dest.BatchCreateLogs(ctx, req)
		l.L.Lock()
		if err != nil {
			return xerrors.Errorf("failed to upload logs: %w", err)
		}
		if resp.LogLimitExceeded {
			l.logger.Warn(ctx, "server log limit exceeded; logs truncated")
			l.exceededLogLimit = true
			// no point in keeping anything we have queued around, server will not accept them
			l.queues = make(map[uuid.UUID]*logQueue)
			return LogLimitExceededError
		}

		// Since elsewhere we only append to the logs, here we can remove them
		// since we successfully sent them.  First we nil the pointers though,
		// so that they can be gc'd.
		for i := 0; i < n; i++ {
			q.logs[i] = nil
		}
		q.logs = q.logs[n:]
		l.outputLen -= outputToRemove
		if len(q.logs) == 0 {
			// no empty queues
			delete(l.queues, src)
			continue
		}
		q.lastFlush = time.Now()
	}
}

func (l *LogSender) hasPendingWorkLocked() bool {
	for _, q := range l.queues {
		if time.Since(q.lastFlush) > flushInterval {
			return true
		}
		if q.flushRequested {
			return true
		}
	}
	return false
}

func (l *LogSender) getPendingWorkLocked() (src uuid.UUID, q *logQueue) {
	// take the one it's been the longest since we've flushed, so that we have some sense of
	// fairness across sources
	var earliestFlush time.Time
	for is, iq := range l.queues {
		if q == nil || iq.lastFlush.Before(earliestFlush) {
			src = is
			q = iq
			earliestFlush = iq.lastFlush
		}
	}
	return src, q
}

func (l *LogSender) GetScriptLogger(logSourceID uuid.UUID) ScriptLogger {
	return ScriptLogger{srcID: logSourceID, sender: l}
}

type ScriptLogger struct {
	sender *LogSender
	srcID  uuid.UUID
}

func (s ScriptLogger) Send(ctx context.Context, log ...Log) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		s.sender.Enqueue(s.srcID, log...)
		return nil
	}
}

func (s ScriptLogger) Flush(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		s.sender.Flush(s.srcID)
		return nil
	}
}
