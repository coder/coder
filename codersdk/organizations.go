package codersdk

import (
	"context"

	"github.com/coder/coder/coderd"
	"github.com/google/uuid"
)

// OrganizationsByUser returns organizations for the provided user.
func (c *Client) OrganizationsByUser(ctx context.Context, user uuid.UUID) ([]coderd.Organization, error) {
	return nil, nil
}

// OrganizationByName returns an organization by case-insensitive name.
func (c *Client) OrganizationByName(ctx context.Context, user uuid.UUID, name string) (coderd.Organization, error) {
	return coderd.Organization{}, nil
}
