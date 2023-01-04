package codersdk

import (
	"time"

	"github.com/google/uuid"
)

type OrganizationMember struct {
	UserID         uuid.UUID `db:"user_id" json:"user_id" format:"uuid"`
	OrganizationID uuid.UUID `db:"organization_id" json:"organization_id" format:"uuid"`
	CreatedAt      time.Time `db:"created_at" json:"created_at" format:"date-time"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at" format:"date-time"`
	Roles          []Role    `db:"roles" json:"roles"`
}
