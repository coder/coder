package keys

import (
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
)

const (
	privateSuffixLength = 32

	// KeyPrefixLength is the total length of the visible key prefix.
	KeyPrefixLength = 11

	// KeyLength is the total length of the plaintext key returned to
	// the user on Create.
	KeyLength = KeyPrefixLength + privateSuffixLength
)

// New generates an AI Gateway key used for authenticating standalone replicas.
// Returns InsertParams ready for the database query.
func New(name string) (database.InsertAIGatewayKeyParams, string, error) {
	secret, hashed, err := apikey.GenerateSecret(KeyLength)
	if err != nil {
		return database.InsertAIGatewayKeyParams{}, "", xerrors.Errorf("generate secret: %w", err)
	}
	if len(secret) != KeyLength {
		return database.InsertAIGatewayKeyParams{}, "", xerrors.Errorf("generated secret has unexpected length: got %d, want %d", len(secret), KeyLength)
	}
	if KeyLength < KeyPrefixLength {
		return database.InsertAIGatewayKeyParams{}, "", xerrors.Errorf("KeyLength (%d) must be >= KeyPrefixLength (%d)", KeyLength, KeyPrefixLength)
	}
	visiblePrefix := secret[:KeyPrefixLength]

	return database.InsertAIGatewayKeyParams{
		ID:           uuid.New(),
		Name:         name,
		SecretPrefix: visiblePrefix,
		HashedSecret: hashed,
	}, secret, nil
}
