package codersdk

import (
	"time"

	"github.com/google/uuid"
)

type OrganizationMember struct {
	UserID         uuid.UUID `db:"user_id" json:"user_id"`
	OrganizationID uuid.UUID `db:"organization_id" json:"organization_id"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
	Roles          []Role    `db:"roles" json:"roles"`
}
