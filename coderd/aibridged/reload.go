package aibridged

import (
	"context"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/pubsub"
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
// reload is best-effort: a reload failure is logged and not returned.
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
