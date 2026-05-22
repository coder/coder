package aigatewaycoderdkey

import (
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
)

const (
	visiblePrefixLength = 7
	privateSuffixLength = 32

	// KeyTypePrefix marks a key as belonging to the Coder AI Gateway.
	KeyTypePrefix = "cgw_"

	// KeyPrefixLength is the total length of the visible key prefix.
	KeyPrefixLength = len(KeyTypePrefix) + visiblePrefixLength

	// KeyLength is the total length of the plaintext key returned to
	// the user on Create.
	KeyLength = KeyPrefixLength + privateSuffixLength
)

// New generates an AI Gateway Coderd key. Returns InsertParams ready
// for the database query.
//
// Key shape: "cgw_" + 7 random chars + 32 random chars = 43 chars total.
func New(name string) (database.InsertAIGatewayCoderdKeyParams, string, error) {
	secret, hashed, err := apikey.GenerateSecret(visiblePrefixLength + privateSuffixLength)
	if err != nil {
		return database.InsertAIGatewayCoderdKeyParams{}, "", xerrors.Errorf("generate secret: %w", err)
	}

	secret = KeyTypePrefix + secret
	visiblePrefix := secret[:KeyPrefixLength]

	return database.InsertAIGatewayCoderdKeyParams{
		ID:           uuid.New(),
		Name:         name,
		SecretPrefix: visiblePrefix,
		HashedSecret: hashed,
	}, secret, nil
}
