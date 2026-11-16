package database

import (
	"crypto/sha256"

	"github.com/google/uuid"
)

// HashAuthToken computes the deterministic lookup hash of a workspace agent
// auth token for the workspace_agents.auth_token_hashed column. Agents are
// authenticated by this hash so the token itself can be encrypted at rest
// with a non-deterministic cipher (SEC-1491). The input is the canonical
// lowercase uuid string, matching the sha256(convert_to(auth_token, 'UTF8'))
// backfill in migration 000649.
func HashAuthToken(token uuid.UUID) []byte {
	hashed := sha256.Sum256([]byte(token.String()))
	return hashed[:]
}
