package aibridged

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/retry"
)

// ProviderReloader refreshes a component's provider snapshot.
type ProviderReloader interface {
	Reload(ctx context.Context) error
}

// SubscribeProviderReload subscribes to AI provider change events, reloading
// the reloader's snapshot on each event, and performs one initial reload
// before returning. Subscribing happens before the initial reload so no change
// event is missed.
//
// A subscription failure returns an error without reloading. The initial
// reload is best-effort: a reload failure is logged and not returned. A
// dropped-message delivery error triggers a reload too, matching
// WatchAIProviders: a drop may have masked a change, so the snapshot must
// reconverge.
func SubscribeProviderReload(
	ctx context.Context,
	ps dbpubsub.Pubsub,
	reloader ProviderReloader,
	logger slog.Logger,
) (func(), error) {
	if ps == nil {
		return nil, xerrors.New("pubsub is required")
	}
	if reloader == nil {
		return nil, xerrors.New("reloader is required")
	}

	unsubscribe, err := ps.SubscribeWithErr(pubsub.AIProvidersChangedChannel, func(cbCtx context.Context, _ []byte, err error) {
		if err != nil {
			// A dropped message may have masked a change, so reload anyway to
			// reconverge rather than skipping.
			logger.Warn(cbCtx, "ai providers changed event delivered with error", slog.Error(err))
		}
		if err := reloader.Reload(cbCtx); err != nil {
			logger.Warn(cbCtx, "reload ai provider snapshot from pubsub event", slog.Error(err))
			return
		}
		logger.Debug(cbCtx, "reloaded ai provider snapshot from pubsub event")
	})
	if err != nil {
		return nil, xerrors.Errorf("subscribe to %s: %w", pubsub.AIProvidersChangedChannel, err)
	}

	if err := reloader.Reload(ctx); err != nil {
		logger.Warn(ctx, "initial ai provider reload", slog.Error(err))
	}
	return unsubscribe, nil
}

// WatchProviderReload opens a coderd WatchAIProviders stream via clientFn and
// calls reloader.Reload on each change signal the server emits. The stream is
// re-established with exponential backoff whenever it drops. It does not
// perform an initial load; the caller is responsible for any blocking load
// before serving.
//
// It runs until ctx is canceled, then returns ctx.Err(). clientFn receives ctx,
// so a client acquisition that blocks (e.g. Server.ClientContext waiting for
// the daemon to connect to coderd) unblocks when ctx is canceled, leaving no
// goroutine behind.
func WatchProviderReload(
	ctx context.Context,
	clientFn ClientFuncWithContext,
	reloader ProviderReloader,
	logger slog.Logger,
) error {
	if clientFn == nil {
		return xerrors.New("client is required")
	}
	if reloader == nil {
		return xerrors.New("reloader is required")
	}

	r := retry.New(50*time.Millisecond, 10*time.Second)
	for {
		received, err := watchProviderReloadOnce(ctx, clientFn, reloader, logger)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		logger.Warn(ctx, "ai provider watch stream ended; reconnecting", slog.Error(err))
		// Only reset the backoff once a signal was actually received. A stream
		// that opens but errors before any Recv (e.g. the server fails during
		// subscribe) would otherwise reset to the floor and reconnect at
		// network-RTT speed.
		if received {
			r.Reset()
		}
		if !r.Wait(ctx) {
			return ctx.Err()
		}
	}
}

// watchProviderReloadOnce opens a single WatchAIProviders stream and reloads on
// each signal until the stream fails. received reports whether at least one
// signal was received before the error.
func watchProviderReloadOnce(ctx context.Context, clientFn ClientFuncWithContext, reloader ProviderReloader, logger slog.Logger) (received bool, err error) {
	// clientFn blocks until the daemon connects to coderd or ctx is canceled.
	c, err := clientFn(ctx)
	if err != nil {
		return false, xerrors.Errorf("get ai-gateway client: %w", err)
	}
	stream, err := c.WatchAIProviders(ctx, &proto.WatchAIProvidersRequest{})
	if err != nil {
		return false, xerrors.Errorf("open ai providers watch stream: %w", err)
	}
	defer func() {
		_ = stream.Close()
	}()

	for {
		if _, err := stream.Recv(); err != nil {
			return received, xerrors.Errorf("receive ai providers change signal: %w", err)
		}
		received = true
		if err := reloader.Reload(ctx); err != nil {
			logger.Warn(ctx, "failed to reload ai provider snapshot from watch signal", slog.Error(err))
			continue
		}
		logger.Debug(ctx, "reloaded ai provider snapshot from watch signal")
	}
}
