package codersdk

import "time"

type GitSSHKey struct {
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	PublicKey string    `json:"public_key"`
}
