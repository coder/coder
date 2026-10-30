package mcpbuiltin

import (
	"context"
	"strings"

	"golang.org/x/xerrors"
)

// SlackThread identifies a Slack thread a chat came from.
type SlackThread struct {
	TeamID   string
	Channel  string
	ThreadTS string
}

// ConversationResolver maps a calling chat to the external conversation
// that started it, such as the Slack thread of a trigger event.
type ConversationResolver interface {
	// SlackThread returns the Slack thread of the verified calling chat.
	// ok is false when the chat did not come from Slack.
	SlackThread(ctx context.Context, identity Identity) (thread SlackThread, ok bool, err error)
}

// ParseSlackConversationKey parses a Slack trigger conversation key of the
// form "team_id/channel/thread_ts".
func ParseSlackConversationKey(key string) (SlackThread, error) {
	parts := strings.Split(key, "/")
	if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
		return SlackThread{}, xerrors.Errorf("slack conversation key %q is not team_id/channel/thread_ts", key)
	}
	return SlackThread{TeamID: parts[0], Channel: parts[1], ThreadTS: parts[2]}, nil
}

// originSlackThread returns the Slack thread of the verified calling chat.
func (h *Handler) originSlackThread(ctx context.Context, info requestInfo) (SlackThread, bool, error) {
	if !info.verified || h.opts.Conversations == nil {
		return SlackThread{}, false, nil
	}
	thread, ok, err := h.opts.Conversations.SlackThread(ctx, info.identity)
	if err != nil {
		return SlackThread{}, false, xerrors.Errorf("resolve the chat's Slack thread: %w", err)
	}
	return thread, ok, nil
}
