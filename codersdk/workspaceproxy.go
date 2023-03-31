package codersdk

import (
	"time"

	"github.com/google/uuid"
)

type CreateWorkspaceProxyRequest struct {
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	URL         string `json:"url"`
	WildcardURL string `json:"wildcard_url"`
}

type WorkspaceProxy struct {
	ID             uuid.UUID `db:"id" json:"id"`
	OrganizationID uuid.UUID `db:"organization_id" json:"organization_id"`
	Name           string    `db:"name" json:"name"`
	Icon           string    `db:"icon" json:"icon"`
	// Full url including scheme of the proxy api url: https://us.example.com
	Url string `db:"url" json:"url"`
	// URL with the wildcard for subdomain based app hosting: https://*.us.example.com
	WildcardUrl string    `db:"wildcard_url" json:"wildcard_url"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
	Deleted     bool      `db:"deleted" json:"deleted"`
}
