package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// DelegatedIdentity is the identity of something that acts under a user,
// such as a coding agent. It is an account of its own: it owns the workspaces
// created on its behalf and is named in audit logs. It can never do more than
// its parent.
type DelegatedIdentity struct {
	// ID is also the ID of the account that is this identity.
	ID             uuid.UUID `json:"id" format:"uuid"`
	OrganizationID uuid.UUID `json:"organization_id" format:"uuid"`
	// Name is the username of the identity's account. Workspaces of the
	// identity are addressed as name/workspace.
	Name string `json:"name"`
	// ParentID is the user or delegated identity this identity acts under.
	ParentID   uuid.UUID `json:"parent_id" format:"uuid"`
	ParentName string    `json:"parent_name"`
	// RootUserID is the user at the top of the chain of parents. The
	// identity is judged with this user's roles and groups.
	RootUserID   uuid.UUID `json:"root_user_id" format:"uuid"`
	RootUsername string    `json:"root_username"`
	// CreatedBy differs from the parent when a trusted service created
	// the identity under a member.
	CreatedBy uuid.UUID `json:"created_by" format:"uuid"`
	// Scopes is what the identity was given. An empty list grants nothing.
	Scopes []APIKeyScope `json:"scopes"`
	// EffectiveScopes is what the identity may do now: its scopes, less
	// whatever a delegated identity above it lacks.
	EffectiveScopes []APIKeyScope `json:"effective_scopes"`
	CreatedAt       time.Time     `json:"created_at" format:"date-time"`
	UpdatedAt       time.Time     `json:"updated_at" format:"date-time"`
}

// CreateDelegatedIdentityRequest creates a delegated identity. It returns a
// record and no credential: an identity gets its credentials where it is
// used, for example in a workspace that is built on its behalf.
type CreateDelegatedIdentityRequest struct {
	// Name becomes the username of the identity's account. One is made up
	// when it is empty.
	Name string `json:"name,omitempty"`
	// ParentID is the user or delegated identity to create the identity
	// under. It defaults to the caller. Naming anyone who is not the
	// caller or below the caller needs permission to create delegated
	// identities across the organization.
	ParentID uuid.UUID `json:"parent_id,omitempty" format:"uuid"`
	// OrganizationID is required when the parent is a user in more than
	// one organization. An identity under an identity is in its parent's
	// organization.
	OrganizationID uuid.UUID `json:"organization_id,omitempty" format:"uuid"`
	// Scopes is what the identity may do with the API. It can hold no
	// more than its parent does.
	Scopes []APIKeyScope `json:"scopes,omitempty"`
}

// UpdateDelegatedIdentityRequest replaces what a delegated identity may do.
type UpdateDelegatedIdentityRequest struct {
	Scopes []APIKeyScope `json:"scopes"`
}

// DelegatedIdentityMe names the delegated identity that makes the request.
const DelegatedIdentityMe = "me"

// CreateDelegatedIdentity creates a delegated identity under the caller, or
// under req.ParentID.
func (c *Client) CreateDelegatedIdentity(ctx context.Context, req CreateDelegatedIdentityRequest) (DelegatedIdentity, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/identities", req)
	if err != nil {
		return DelegatedIdentity{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return DelegatedIdentity{}, ReadBodyAsError(res)
	}
	var identity DelegatedIdentity
	return identity, ReadBodyAsJSON(res, &identity)
}

// DelegatedIdentities lists the delegated identities below the caller, at
// any depth.
func (c *Client) DelegatedIdentities(ctx context.Context) ([]DelegatedIdentity, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/identities", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var identities []DelegatedIdentity
	return identities, ReadBodyAsJSON(res, &identities)
}

// DelegatedIdentity returns a delegated identity by its ID or name, or the
// caller's own with DelegatedIdentityMe.
func (c *Client) DelegatedIdentity(ctx context.Context, identity string) (DelegatedIdentity, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/identities/%s", identity), nil)
	if err != nil {
		return DelegatedIdentity{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return DelegatedIdentity{}, ReadBodyAsError(res)
	}
	var got DelegatedIdentity
	return got, ReadBodyAsJSON(res, &got)
}

// UpdateDelegatedIdentity replaces what a delegated identity may do.
func (c *Client) UpdateDelegatedIdentity(ctx context.Context, identity string, req UpdateDelegatedIdentityRequest) (DelegatedIdentity, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/identities/%s", identity), req)
	if err != nil {
		return DelegatedIdentity{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return DelegatedIdentity{}, ReadBodyAsError(res)
	}
	var got DelegatedIdentity
	return got, ReadBodyAsJSON(res, &got)
}

// DeleteDelegatedIdentity deletes a delegated identity that owns no
// workspaces and has no identities below it.
func (c *Client) DeleteDelegatedIdentity(ctx context.Context, identity string) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/identities/%s", identity), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
