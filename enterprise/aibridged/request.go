package aibridged

import "github.com/google/uuid"

type Request struct {
	APIKeyID    string
	InitiatorID uuid.UUID
	SessionKey  string
	UserAgent   string
}
