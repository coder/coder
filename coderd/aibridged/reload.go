package aibridged

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/pubsub"
)

// DefaultReloadInterval is the periodic backstop reload cadence. It converges
// the snapshot after a missed pubsub notification or read-replica lag.
const DefaultReloadInterval = 5 * time.Minute

// ProviderReloader refreshes a component's provider snapshot.
type ProviderReloader interface {
	Reload(ctx context.Context) error
}

// SubscribeProviderReload refreshes once, then on AI provider changes.
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
			logger.Warn(cbCtx, "ai providers changed event delivered with error", slog.Error(err))
			return
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

// SubscribePipelineReload reloads on AI gateway policy/pipeline changes. Unlike
// SubscribeProviderReload it does not perform an initial reload, because the
// caller's provider subscription already triggers the initial (combined)
// reload. The returned function unsubscribes.
func SubscribePipelineReload(
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

	unsubscribe, err := ps.SubscribeWithErr(pubsub.AIGatewayPipelinesChangedChannel, func(cbCtx context.Context, _ []byte, err error) {
		if err != nil {
			logger.Warn(cbCtx, "ai gateway pipelines changed event delivered with error", slog.Error(err))
			return
		}
		if err := reloader.Reload(cbCtx); err != nil {
			logger.Warn(cbCtx, "reload policy pipeline snapshot from pubsub event", slog.Error(err))
			return
		}
		logger.Debug(cbCtx, "reloaded policy pipeline snapshot from pubsub event")
	})
	if err != nil {
		return nil, xerrors.Errorf("subscribe to %s: %w", pubsub.AIGatewayPipelinesChangedChannel, err)
	}
	return unsubscribe, nil
}

// StartPeriodicReload runs reloader.Reload on a fixed interval until the
// returned stop function is called. It is a backstop against missed pubsub
// notifications and read-replica lag (the snapshot read may lag a post-commit
// notification).
func StartPeriodicReload(
	reloader ProviderReloader,
	interval time.Duration,
	logger slog.Logger,
) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := reloader.Reload(ctx); err != nil {
					logger.Warn(ctx, "periodic snapshot reload", slog.Error(err))
					continue
				}
				logger.Debug(ctx, "reloaded snapshot from periodic tick")
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}
