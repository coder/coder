package aibridged

import "github.com/google/uuid"

type Request struct {
	SessionKey  string
	InitiatorID uuid.UUID
}
