package aigatewaycoderdkey

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

// New generates an AI Gateway Coderd key. Returns InsertParams ready
// for the database query.
func New(name string) (database.InsertAIGatewayCoderdKeyParams, string, error) {
	secret, hashed, err := apikey.GenerateSecret(KeyLength)
	if err != nil {
		return database.InsertAIGatewayCoderdKeyParams{}, "", xerrors.Errorf("generate secret: %w", err)
	}
	visiblePrefix := secret[:KeyPrefixLength]

	return database.InsertAIGatewayCoderdKeyParams{
		ID:           uuid.New(),
		Name:         name,
		SecretPrefix: visiblePrefix,
		HashedSecret: hashed,
	}, secret, nil
}
