package usersecretspubsub

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/pubsub"
)

type EventKind string

const (
	EventKindCreated EventKind = "created"
	EventKindUpdated EventKind = "updated"
	EventKindDeleted EventKind = "deleted"
)

type Event struct {
	Kind     EventKind `json:"kind"`
	UserID   uuid.UUID `json:"user_id" format:"uuid"`
	Name     string    `json:"name"`
	EnvName  string    `json:"env_name,omitempty"`
	FilePath string    `json:"file_path,omitempty"`
}

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
