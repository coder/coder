package provisionerkey

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/cryptorand"
)

func New(organizationID uuid.UUID, name string, tags map[string]string) (database.InsertProvisionerKeyParams, string, error) {
	id := uuid.New()
	secret, err := cryptorand.HexString(64)
	if err != nil {
		return database.InsertProvisionerKeyParams{}, "", xerrors.Errorf("generate token: %w", err)
	}
	hashedSecret := HashSecret(secret)
	token := fmt.Sprintf("%s:%s", id, secret)

	if tags == nil {
		tags = map[string]string{}
	}

	return database.InsertProvisionerKeyParams{
		ID:             id,
		CreatedAt:      dbtime.Now(),
		OrganizationID: organizationID,
		Name:           name,
		HashedSecret:   hashedSecret,
		Tags:           tags,
	}, token, nil
}

func Parse(token string) (uuid.UUID, string, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 2 {
		return uuid.UUID{}, "", xerrors.Errorf("invalid token format")
	}

	id, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.UUID{}, "", xerrors.Errorf("parse id: %w", err)
	}

	return id, parts[1], nil
}

func HashSecret(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

func Compare(a []byte, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) != 1
}
