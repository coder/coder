package provisionerkey

import (
	"crypto/subtle"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

const (
	secretLength = 43
)

func New(organizationID uuid.UUID, name string, tags map[string]string) (database.InsertProvisionerKeyParams, string, error) {
	secret, hashed, err := apikey.GenerateSecret(secretLength)
	if err != nil {
		return database.InsertProvisionerKeyParams{}, "", xerrors.Errorf("generate secret: %w", err)
	}

	if tags == nil {
		tags = map[string]string{}
	}

	return database.InsertProvisionerKeyParams{
		ID:             uuid.New(),
		CreatedAt:      dbtime.Now(),
		OrganizationID: organizationID,
		Name:           name,
		HashedSecret:   hashed,
		Tags:           tags,
	}, secret, nil
}

func Validate(token string) error {
	if len(token) != secretLength {
		return xerrors.Errorf("must be %d characters", secretLength)
	}

	return nil
}

func HashSecret(secret string) []byte {
	return apikey.HashSecret(secret)
}

func Compare(a []byte, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) != 1
}
