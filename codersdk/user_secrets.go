package codersdk

import (
	"github.com/google/uuid"
	"time"
)

// TODO: add and register custom validator functions. check codersdk/name.go for examples.
// TODO: reuse NameValid func for Name?
type CreateUserSecretRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description,omitempty" validate:"lt=1000"`
	Value       string `json:"value" validate:"required"`
}

type UpdateUserSecretRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description,omitempty" validate:"lt=1000"`
	Value       string `json:"value" validate:"required"`
}

// Response types
type UserSecret struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	UserID      uuid.UUID `json:"user_id" format:"uuid"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at" format:"date-time"`
	UpdatedAt   time.Time `json:"updated_at" format:"date-time"`
}

type UserSecretValue struct {
	Value string `json:"value"`
}

type ListUserSecretsResponse struct {
	Secrets []UserSecret `json:"secrets"`
}
