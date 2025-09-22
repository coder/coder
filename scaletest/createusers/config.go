package createusers

import (
	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type Config struct {
	// OrganizationID is the ID of the organization to add the user to.
	OrganizationID uuid.UUID `json:"organization_id"`
	// Username is the username of the new user. Generated if empty.
	Username string `json:"username"`
	// Email is the email of the new user. Generated if empty.
	Email string `json:"email"`
}

func (c Config) Validate() error {
	if c.OrganizationID == uuid.Nil {
		return xerrors.New("organization_id must not be a nil UUID")
	}

	return nil
}
