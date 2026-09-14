package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

func InboxNotificationForOwnerEventChannel(ownerID uuid.UUID) string {
	return fmt.Sprintf("inbox_notification:owner:%s", ownerID)
}

func HandleInboxNotificationEvent(cb func(ctx context.Context, payload InboxNotificationEvent, err error)) func(ctx context.Context, message []byte, err error) {
	return func(ctx context.Context, message []byte, err error) {
		if err != nil {
			cb(ctx, InboxNotificationEvent{}, xerrors.Errorf("inbox notification event pubsub: %w", err))
			return
		}
		var payload InboxNotificationEvent
		if err := json.Unmarshal(message, &payload); err != nil {
			cb(ctx, InboxNotificationEvent{}, xerrors.Errorf("unmarshal inbox notification event"))
			return
		}

		cb(ctx, payload, err)
	}
}

type InboxNotificationEvent struct {
	Kind              InboxNotificationEventKind `json:"kind"`
	InboxNotification codersdk.InboxNotification `json:"inbox_notification"`
}

type InboxNotificationEventKind string

const (
	InboxNotificationEventKindNew InboxNotificationEventKind = "new"
)
