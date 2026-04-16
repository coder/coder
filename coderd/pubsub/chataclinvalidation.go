package pubsub

import (
	"fmt"

	"github.com/google/uuid"
)

// ChatACLInvalidationChannel returns the per-chat pubsub channel that
// carries ACL-change notifications. Open chat live streams subscribe to
// this channel so they can re-run authz exactly when the ACL changes,
// rather than on a timer. Keeping the channel per-chat bounds the
// subscription count at N viewers per chat (not N across the
// deployment) and matches the granularity at which ACLs are actually
// stored.
//
// Publishers MUST publish after the database transaction that modified
// the ACL commits; otherwise a subscriber can re-authz on stale data.
// The payload is unused today (a single byte is typical) — subscribers
// re-read the chat via dbauthz on every message.
func ChatACLInvalidationChannel(chatID uuid.UUID) string {
	return fmt.Sprintf("chat:acl:%s", chatID)
}

// ChatACLBroadcastChannel is a deployment-wide broadcast channel used
// when the kill-switch flips (DisableChatSharing=true at runtime) and
// every open live stream must re-authz. Individual per-chat
// invalidations are preferred for normal ACL writes; the broadcast
// channel is reserved for deployment-level changes that affect all
// chats.
const ChatACLBroadcastChannel = "chat:acl:all"
