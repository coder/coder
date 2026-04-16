package pubsub

import (
	"fmt"

	"github.com/google/uuid"
)

// ChatWatchlistChannel returns the per-user pubsub channel that
// carries lifecycle events for chats a viewer can see via ACL sharing
// (but does not own). Publishers dual-publish lifecycle events: once to
// the owner's ChatWatchEventChannel and once to each shared viewer's
// watchlist channel. The subscriber at /chats/watch merges both
// channels.
//
// Keeping the channel per-user avoids per-chat subscription growth for
// viewers with many shared chats. It does cost O(viewers) publishes per
// lifecycle event, which is acceptable because lifecycle events are low
// rate (archive/unarchive/rename/status-change/create/delete).
func ChatWatchlistChannel(viewerID uuid.UUID) string {
	return fmt.Sprintf("chat:watchlist:%s", viewerID)
}
