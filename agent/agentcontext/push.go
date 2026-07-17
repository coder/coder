package agentcontext

import (
	"context"
	"errors"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
)

// PushRequest is the wire-format-independent payload the
// Manager hands to a Pusher. It mirrors the protobuf
// PushContextStateRequest message reserved in the RFC.
//
// Keeping the shape in plain Go lets this package compile
// without bumping the drpc proto version. The follow-up
// integration change can add a thin adapter that converts
// PushRequest to proto and back.
type PushRequest struct {
	Version       uint64
	AggregateHash [32]byte
	Resources     []Resource
	Initial       bool
	SnapshotError string
}

// PushResponse is the wire-format-independent return value of
// a push.
type PushResponse struct {
	Accepted bool
}

// Pusher delivers snapshots to coderd. Concrete implementations
// wrap a drpc client (Agent API v2.10 and later) or, in tests,
// a recording in-memory fake.
//
// PushContextState must respect ctx cancellation; the Manager
// retries on transient errors with backoff but stops on
// ErrPushUnimplemented.
type Pusher interface {
	PushContextState(ctx context.Context, req *PushRequest) (*PushResponse, error)
}

// ErrPushUnimplemented signals that the coderd peer does not
// implement PushContextState. RunPush stops pushing for the
// remainder of the connection.
var ErrPushUnimplemented = xerrors.New("agentcontext: PushContextState unimplemented")

// Default backoff timings for pushWithRetry. Exposed as named
// constants (rather than inline literals) so godoc shows them
// and a second push loop, if it ever appears, can reuse them.
const (
	DefaultPushInitialBackoff = 250 * time.Millisecond
	DefaultPushMaxBackoff     = 30 * time.Second
)

// PushOptions parameterizes RunPush.
type PushOptions struct {
	// Logger receives push success/failure diagnostics.
	Logger slog.Logger
	// InitialBackoff is the wait before the first retry.
	// Default 250ms.
	InitialBackoff time.Duration
	// MaxBackoff caps the retry wait. Default 30s.
	MaxBackoff time.Duration
	// Clock is the time source for retry backoffs. Optional;
	// defaults to the Manager's clock so tests can trap waits
	// with quartz instead of real sleeps.
	Clock quartz.Clock
}

// RunPush ships the current snapshot to the Pusher, then ships
// every subsequent snapshot whenever the Manager broadcasts a
// change. RunPush returns when ctx is canceled, when the
// Manager is closed, or when the Pusher signals
// ErrPushUnimplemented.
//
// The first push is always sent with Initial=true so coderd can
// distinguish a fresh boot from a drift event.
func (m *Manager) RunPush(ctx context.Context, p Pusher, opts PushOptions) error {
	if p == nil {
		return xerrors.New("agentcontext: Pusher is required")
	}
	logger := opts.Logger
	initialBackoff := opts.InitialBackoff
	if initialBackoff <= 0 {
		initialBackoff = DefaultPushInitialBackoff
	}
	maxBackoff := opts.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = DefaultPushMaxBackoff
	}
	clock := opts.Clock
	if clock == nil {
		clock = m.clock
	}

	changes, unsub := m.SubscribeChanges()
	defer unsub()

	// Until SetReady the snapshot is version 0: wait, don't push it.
	initial := true
	for {
		snap := m.Snapshot()
		if snap.Version == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-m.closedCh:
				return nil
			case <-changes:
			}
			continue
		}
		req := snapshotToPushRequest(snap, initial)

		err := pushWithRetry(ctx, p, req, initialBackoff, maxBackoff, clock, logger)
		switch {
		case err == nil:
			initial = false
		case errors.Is(err, ErrPushUnimplemented):
			logger.Warn(ctx, "coderd peer does not implement PushContextState; stopping")
			return nil
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return ctx.Err()
		default:
			// Should be unreachable: pushWithRetry only
			// returns terminal errors. Log and continue.
			logger.Warn(ctx, "push terminated with non-retried error", slog.Error(err))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-m.closedCh:
			return nil
		case <-changes:
			// Shutdown comes from closedCh or ctx; the
			// subscriber channel is never closed by
			// SubscribeChanges.
		}
	}
}

// pushWithRetry retries transient errors with exponential
// backoff capped at maxBackoff. The retry loop exits when:
//
//   - ctx is canceled (returns ctx.Err()).
//   - The Pusher returns nil (success).
//   - The Pusher returns ErrPushUnimplemented (propagated).
func pushWithRetry(
	ctx context.Context,
	p Pusher,
	req *PushRequest,
	initialBackoff, maxBackoff time.Duration,
	clock quartz.Clock,
	logger slog.Logger,
) error {
	backoff := initialBackoff
	for {
		resp, err := p.PushContextState(ctx, req)
		if err == nil {
			if resp != nil && !resp.Accepted {
				// Out-of-order or replayed push. Do not
				// retry; the next change will redeliver
				// the snapshot with a higher version.
				logger.Debug(ctx, "push rejected, awaiting next change",
					slog.F("version", req.Version))
			}
			return nil
		}
		if errors.Is(err, ErrPushUnimplemented) {
			return err
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		logger.Warn(ctx, "push failed, retrying",
			slog.F("version", req.Version),
			slog.F("backoff", backoff),
			slog.Error(err))
		timer := clock.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// snapshotToPushRequest copies the Snapshot into the wire
// representation. The Resources slice is reused; callers must
// not mutate it.
func snapshotToPushRequest(s Snapshot, initial bool) *PushRequest {
	return &PushRequest{
		Version:       s.Version,
		AggregateHash: s.AggregateHash,
		Resources:     s.Resources,
		Initial:       initial,
		SnapshotError: s.SnapshotError,
	}
}
