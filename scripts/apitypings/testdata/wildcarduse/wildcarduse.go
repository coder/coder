package codersdk

import (
	"github.com/coder/coder/v2/x/wildcard"
	"github.com/google/uuid"
)

// WildUser ensures wildcard.Value[T] renders as T | "*" in TS
type WildUser struct {
	Type wildcard.Value[string]    `json:"type"`
	ID   wildcard.Value[uuid.UUID] `json:"id"`
}
