package provisionerkey
import (
	"fmt"
	"errors"
	"crypto/sha256"
	"crypto/subtle"
	"github.com/google/uuid"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/cryptorand"
)
const (
	secretLength = 43
)
func New(organizationID uuid.UUID, name string, tags map[string]string) (database.InsertProvisionerKeyParams, string, error) {
	secret, err := cryptorand.String(secretLength)
	if err != nil {
		return database.InsertProvisionerKeyParams{}, "", fmt.Errorf("generate secret: %w", err)
	}
	if tags == nil {
		tags = map[string]string{}
	}
	return database.InsertProvisionerKeyParams{
		ID:             uuid.New(),
		CreatedAt:      dbtime.Now(),
		OrganizationID: organizationID,
		Name:           name,
		HashedSecret:   HashSecret(secret),
		Tags:           tags,
	}, secret, nil
}
func Validate(token string) error {
	if len(token) != secretLength {
		return fmt.Errorf("must be %d characters", secretLength)
	}
	return nil
}
func HashSecret(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}
func Compare(a []byte, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) != 1
}
