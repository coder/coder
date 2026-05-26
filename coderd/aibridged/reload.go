package aibridged

import (
	"context"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	coderpubsub "github.com/coder/coder/v2/coderd/pubsub"
)

// ProviderReloader applies a fresh provider snapshot from the
// authoritative source (the database). Each reloader owns its own DB
// read so the subscriber stays agnostic of provider-construction
// details (circuit-breaker config, key plumbing, etc.) that the
// consumer needs but the subscriber does not.
type ProviderReloader interface {
	Reload(ctx context.Context) error
}

// SubscribeProviderReload subscribes to the AI provider change channel
// and invokes reloader.Reload(ctx) each time a notification arrives.
//
// It performs an initial Reload synchronously so the reloader's
// snapshot is always consistent with the database immediately after
// this call returns, regardless of whether any pubsub message was
// missed between the boot-time snapshot construction and subscription
// setup. The returned unsubscribe function tears down the
// subscription; reloader ownership is the caller's responsibility.
//
// Subscribers are deliberately tolerant of dropped messages: the
// database is authoritative, the pubsub payload is only an
// invalidation hint, and any subsequent successful notification picks
// up missed changes.
func SubscribeProviderReload(
	ctx context.Context,
	ps pubsub.Pubsub,
	reloader ProviderReloader,
	logger slog.Logger,
) (func(), error) {
	if ps == nil {
		return nil, xerrors.New("pubsub is required")
	}
	if reloader == nil {
		return nil, xerrors.New("reloader is required")
	}

	// Sync the reloader with the database before listening so any change
	// committed between the boot-time snapshot construction and now is
	// applied even if its pubsub message was emitted while we were not
	// subscribed.
	if err := reloader.Reload(ctx); err != nil {
		logger.Warn(ctx, "initial ai provider reload", slog.Error(err))
	}

	unsubscribe, err := ps.SubscribeWithErr(coderpubsub.AIProvidersChangedChannel, func(cbCtx context.Context, _ []byte, err error) {
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
		return nil, xerrors.Errorf("subscribe to %s: %w", coderpubsub.AIProvidersChangedChannel, err)
	}
	return unsubscribe, nil
}
