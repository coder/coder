package coderd

import (
	"time"

	"github.com/coder/coder/database"
)

// Organization is the JSON representation of a Coder organization.
type Organization struct {
	ID        string    `json:"id" validate:"required"`
	Username  string    `json:"username" validate:"required"`
	CreatedAt time.Time `json:"created_at" validate:"required"`
	UpdatedAt time.Time `json:"updated_at" validate:"required"`
}

// convertOrganization consumes the database representation and outputs API friendly.
func convertOrganization(organization database.Organization) Organization {
	return Organization{
		ID:        organization.ID,
		Username:  organization.Name,
		CreatedAt: organization.CreatedAt,
		UpdatedAt: organization.UpdatedAt,
	}
}
