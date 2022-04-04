package codersdk

import (
	"time"

	"github.com/google/uuid"
)

type GitSSHKey struct {
	UserID    uuid.UUID `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	PublicKey string    `json:"public_key"`
}

type AgentGitSSHKey struct {
	UserID     uuid.UUID `json:"user_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	PrivateKey string    `json:"private_key"`
	PublicKey  string    `json:"public_key"`
}
