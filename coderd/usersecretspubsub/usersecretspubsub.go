package usersecretspubsub

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk"
)

type EventKind = codersdk.UserSecretEventKind

const (
	EventKindCreated EventKind = codersdk.UserSecretEventKindCreated
	EventKindUpdated EventKind = codersdk.UserSecretEventKindUpdated
	EventKindDeleted EventKind = codersdk.UserSecretEventKindDeleted
)

type Event = codersdk.UserSecretEvent

func Channel(userID uuid.UUID) string {
	return fmt.Sprintf("user_secrets:%s", userID)
}

func Publish(ps pubsub.Publisher, event Event) error {
	msg, err := json.Marshal(event)
	if err != nil {
		return xerrors.Errorf("marshal user secret event: %w", err)
	}
	if err := ps.Publish(Channel(event.UserID), msg); err != nil {
		return xerrors.Errorf("publish user secret event: %w", err)
	}
	return nil
}
