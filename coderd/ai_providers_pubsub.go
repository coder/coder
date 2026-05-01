package coderd

import (
	"context"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// AIProvidersChangedChannel is the pubsub channel published whenever an
// ai_providers or ai_provider_keys row is inserted, updated, or
// soft-deleted via the API. Subscribers (currently aibridged in every
// replica, but the channel is provider-generic) refresh their cached
// state by re-querying the database.
//
// Messages have no payload; receivers re-read the rows themselves.
// This keeps the channel agnostic to dbcrypt-key changes and avoids
// bus traffic carrying secrets.
const AIProvidersChangedChannel = "ai_providers_changed"

// publishAIProvidersChanged publishes a notification on the providers-
// changed channel. Errors are logged but never returned to callers; a
// missed notification only delays consumers catching up to the new
// state, and the next mutation will retry.
func publishAIProvidersChanged(ctx context.Context, ps pubsub.Pubsub, logger slog.Logger) {
	if ps == nil {
		return
	}
	if err := ps.Publish(AIProvidersChangedChannel, nil); err != nil {
		logger.Warn(ctx, "failed to publish ai_providers_changed",
			slog.Error(err))
	}
}
