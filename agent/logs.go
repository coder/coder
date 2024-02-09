package agent

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

const flushInterval = time.Second

type logQueue struct {
	logs           []*proto.Log
	flushRequested bool
	lastFlush      time.Time
}

// logSender is a subcomponent of agent that handles enqueuing logs and then sending them over the
// agent API.  Things that need to log call enqueue and flush.  When the agent API becomes available,
// the agent calls sendLoop to send pending logs.
type logSender struct {
	*sync.Cond
	queues           map[uuid.UUID]*logQueue
	logger           slog.Logger
	exceededLogLimit bool
}

type logDest interface {
	BatchCreateLogs(ctx context.Context, request *proto.BatchCreateLogsRequest) (*proto.BatchCreateLogsResponse, error)
}

func newLogSender(logger slog.Logger) *logSender {
	return &logSender{
		Cond:   sync.NewCond(&sync.Mutex{}),
		logger: logger,
		queues: make(map[uuid.UUID]*logQueue),
	}
}

func (l *logSender) enqueue(src uuid.UUID, logs ...agentsdk.Log) error {
	logger := l.logger.With(slog.F("log_source_id", src))
	if len(logs) == 0 {
		logger.Debug(context.Background(), "enqueue called with no logs")
		return nil
	}
	l.L.Lock()
	defer l.L.Unlock()
	if l.exceededLogLimit {
		logger.Warn(context.Background(), "dropping enqueued logs because we have reached the server limit")
		// don't error, as we also write to file and don't want the overall write to fail
		return nil
	}
	defer l.Broadcast()
	q, ok := l.queues[src]
	if !ok {
		q = &logQueue{}
		l.queues[src] = q
	}
	for _, log := range logs {
		pl, err := agentsdk.ProtoFromLog(log)
		if err != nil {
			return xerrors.Errorf("failed to convert log: %w", err)
		}
		q.logs = append(q.logs, pl)
	}
	logger.Debug(context.Background(), "enqueued agent logs", slog.F("new_logs", len(logs)), slog.F("queued_logs", len(q.logs)))
	return nil
}

func (l *logSender) flush(src uuid.UUID) error {
	l.L.Lock()
	defer l.L.Unlock()
	defer l.Broadcast()
	q, ok := l.queues[src]
	if ok {
		q.flushRequested = true
	}
	// queue might not exist because it's already been flushed and removed from
	// the map.
	return nil
}

// sendLoop sends any pending logs until it hits an error or the context is canceled.  It does not
// retry as it is expected that a higher layer retries establishing connection to the agent API and
// calls sendLoop again.
func (l *logSender) sendLoop(ctx context.Context, dest logDest) error {
	ctxDone := false
	defer l.logger.Debug(ctx, "sendLoop exiting")

	// wake 4 times per flush interval to check if anything needs to be flushed
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

	l.L.Lock()
	defer l.L.Unlock()
	for {
		for !ctxDone && !l.exceededLogLimit && !l.hasPendingWorkLocked() {
			l.Wait()
		}
		if ctxDone {
			return nil
		}
		if l.exceededLogLimit {
			l.logger.Debug(ctx, "aborting sendLoop because log limit is already exceeded")
			// no point in keeping this loop going, if log limit is exceeded, but don't return an
			// error because we're already handled it
			return nil
		}
		src, q := l.getPendingWorkLocked()
		q.flushRequested = false // clear flag since we're now flushing
		req := &proto.BatchCreateLogsRequest{
			LogSourceId: src[:],
			Logs:        q.logs[:],
		}

		l.L.Unlock()
		l.logger.Debug(ctx, "sending logs to agent API", slog.F("log_source_id", src), slog.F("num_logs", len(req.Logs)))
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
			// We've handled the error as best as we can. We don't want the server limit to grind
			// other things to a halt, so this is all we can do.
			return nil
		}

		// Since elsewhere we only append to the logs, here we can remove them
		// since we successfully sent them.  First we nil the pointers though,
		// so that they can be gc'd.
		for i := 0; i < len(req.Logs); i++ {
			q.logs[i] = nil
		}
		q.logs = q.logs[len(req.Logs):]
		if len(q.logs) == 0 {
			// no empty queues
			delete(l.queues, src)
			continue
		}
		q.lastFlush = time.Now()
	}
}

func (l *logSender) hasPendingWorkLocked() bool {
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

func (l *logSender) getPendingWorkLocked() (src uuid.UUID, q *logQueue) {
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
