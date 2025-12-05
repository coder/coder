package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

func InboxAlertForOwnerEventChannel(ownerID uuid.UUID) string {
	return fmt.Sprintf("inbox_notification:owner:%s", ownerID)
}

func HandleInboxAlertEvent(cb func(ctx context.Context, payload InboxAlertEvent, err error)) func(ctx context.Context, message []byte, err error) {
	return func(ctx context.Context, message []byte, err error) {
		if err != nil {
			cb(ctx, InboxAlertEvent{}, xerrors.Errorf("inbox notification event pubsub: %w", err))
			return
		}
		var payload InboxAlertEvent
		if err := json.Unmarshal(message, &payload); err != nil {
			cb(ctx, InboxAlertEvent{}, xerrors.Errorf("unmarshal inbox notification event"))
			return
		}

		cb(ctx, payload, err)
	}
}

type InboxAlertEvent struct {
	Kind       InboxAlertEventKind `json:"kind"`
	InboxAlert codersdk.InboxAlert `json:"inbox_notification"`
}

type InboxAlertEventKind string

const (
	InboxAlertEventKindNew InboxAlertEventKind = "new"
)
