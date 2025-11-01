package aibridged

import "github.com/google/uuid"

type Request struct {
	SessionKey  string
	APIKeyID    string
	InitiatorID uuid.UUID
}
