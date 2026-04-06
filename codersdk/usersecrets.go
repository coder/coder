package codersdk

import (
	"time"

	"github.com/google/uuid"
)

// UserSecret represents a user secret's metadata. The secret value
// is never included in API responses.
type UserSecret struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	EnvName     string    `json:"env_name"`
	FilePath    string    `json:"file_path"`
	CreatedAt   time.Time `json:"created_at" format:"date-time"`
	UpdatedAt   time.Time `json:"updated_at" format:"date-time"`
}

// CreateUserSecretRequest is the payload for creating a new user
// secret. Name and Value are required. All other fields are optional
// and default to empty string.
type CreateUserSecretRequest struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	EnvName     string `json:"env_name,omitempty"`
	FilePath    string `json:"file_path,omitempty"`
}

// UpdateUserSecretRequest is the payload for partially updating a
// user secret. At least one field must be non-nil. Pointer fields
// distinguish "not sent" (nil) from "set to empty string" (pointer
// to empty string).
type UpdateUserSecretRequest struct {
	Value       *string `json:"value,omitempty"`
	Description *string `json:"description,omitempty"`
	EnvName     *string `json:"env_name,omitempty"`
	FilePath    *string `json:"file_path,omitempty"`
}
