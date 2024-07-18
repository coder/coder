package provisionerkey

import (
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/cryptorand"
)

func New(organizationID uuid.UUID, name string) (database.InsertProvisionerKeyParams, string, error) {
	id := uuid.New()
	secret, err := cryptorand.HexString(64)
	if err != nil {
		return database.InsertProvisionerKeyParams{}, "", xerrors.Errorf("generate token: %w", err)
	}
	hashedSecret := sha256.Sum256([]byte(secret))
	token := fmt.Sprintf("%s:%s", id, secret)

	return database.InsertProvisionerKeyParams{
		ID:             id,
		CreatedAt:      dbtime.Now(),
		OrganizationID: organizationID,
		Name:           name,
		HashedSecret:   hashedSecret[:],
	}, token, nil
}
