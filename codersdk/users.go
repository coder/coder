package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/coder/coder/coderd"
)

// HasFirstUser returns whether the first user has been created.
func (c *Client) HasFirstUser(ctx context.Context) (bool, error) {
	res, err := c.request(ctx, http.MethodGet, "/api/v2/users/first", nil)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if res.StatusCode != http.StatusOK {
		return false, readBodyAsError(res)
	}
	return true, nil
}

// CreateFirstUser attempts to create the first user on a Coder deployment.
// This initial user has superadmin privileges. If >0 users exist, this request will fail.
func (c *Client) CreateFirstUser(ctx context.Context, req coderd.CreateFirstUserRequest) (coderd.CreateFirstUserResponse, error) {
	res, err := c.request(ctx, http.MethodPost, "/api/v2/users/first", req)
	if err != nil {
		return coderd.CreateFirstUserResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return coderd.CreateFirstUserResponse{}, readBodyAsError(res)
	}
	var resp coderd.CreateFirstUserResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// CreateUser creates a new user.
func (c *Client) CreateUser(ctx context.Context, req coderd.CreateUserRequest) (coderd.User, error) {
	res, err := c.request(ctx, http.MethodPost, "/api/v2/users", req)
	if err != nil {
		return coderd.User{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return coderd.User{}, readBodyAsError(res)
	}
	var user coderd.User
	return user, json.NewDecoder(res.Body).Decode(&user)
}

// CreateAPIKey generates an API key for the user ID provided.
func (c *Client) CreateAPIKey(ctx context.Context, id string) (*coderd.GenerateAPIKeyResponse, error) {
	if id == "" {
		id = "me"
	}
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/keys", id), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusCreated {
		return nil, readBodyAsError(res)
	}
	apiKey := &coderd.GenerateAPIKeyResponse{}
	return apiKey, json.NewDecoder(res.Body).Decode(apiKey)
}

// LoginWithPassword creates a session token authenticating with an email and password.
// Call `SetSessionToken()` to apply the newly acquired token to the client.
func (c *Client) LoginWithPassword(ctx context.Context, req coderd.LoginWithPasswordRequest) (coderd.LoginWithPasswordResponse, error) {
	res, err := c.request(ctx, http.MethodPost, "/api/v2/users/login", req)
	if err != nil {
		return coderd.LoginWithPasswordResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return coderd.LoginWithPasswordResponse{}, readBodyAsError(res)
	}
	var resp coderd.LoginWithPasswordResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return coderd.LoginWithPasswordResponse{}, err
	}
	return resp, nil
}

// Logout calls the /logout API
// Call `ClearSessionToken()` to clear the session token of the client.
func (c *Client) Logout(ctx context.Context) error {
	// Since `LoginWithPassword` doesn't actually set a SessionToken
	// (it requires a call to SetSessionToken), this is essentially a no-op
	res, err := c.request(ctx, http.MethodPost, "/api/v2/users/logout", nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return nil
}

// User returns a user for the ID provided.
// If the ID string is empty, the current user will be returned.
func (c *Client) User(ctx context.Context, id string) (coderd.User, error) {
	if id == "" {
		id = "me"
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s", id), nil)
	if err != nil {
		return coderd.User{}, err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusOK {
		return coderd.User{}, readBodyAsError(res)
	}
	var user coderd.User
	return user, json.NewDecoder(res.Body).Decode(&user)
}

// OrganizationsByUser returns all organizations the user is a member of.
func (c *Client) OrganizationsByUser(ctx context.Context, id string) ([]coderd.Organization, error) {
	if id == "" {
		id = "me"
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/organizations", id), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var orgs []coderd.Organization
	return orgs, json.NewDecoder(res.Body).Decode(&orgs)
}

func (c *Client) OrganizationByName(ctx context.Context, user, name string) (coderd.Organization, error) {
	if user == "" {
		user = "me"
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/organizations/%s", user, name), nil)
	if err != nil {
		return coderd.Organization{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.Organization{}, readBodyAsError(res)
	}
	var org coderd.Organization
	return org, json.NewDecoder(res.Body).Decode(&org)
}

// CreateOrganization creates an organization and adds the provided user as an admin.
func (c *Client) CreateOrganization(ctx context.Context, user string, req coderd.CreateOrganizationRequest) (coderd.Organization, error) {
	if user == "" {
		user = "me"
	}
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/organizations", user), req)
	if err != nil {
		return coderd.Organization{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return coderd.Organization{}, readBodyAsError(res)
	}
	var org coderd.Organization
	return org, json.NewDecoder(res.Body).Decode(&org)
}

// CreateWorkspace creates a new workspace for the project specified.
func (c *Client) CreateWorkspace(ctx context.Context, user string, request coderd.CreateWorkspaceRequest) (coderd.Workspace, error) {
	if user == "" {
		user = "me"
	}
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/workspaces", user), request)
	if err != nil {
		return coderd.Workspace{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return coderd.Workspace{}, readBodyAsError(res)
	}
	var workspace coderd.Workspace
	return workspace, json.NewDecoder(res.Body).Decode(&workspace)
}

// WorkspacesByUser returns all workspaces the specified user has access to.
func (c *Client) WorkspacesByUser(ctx context.Context, user string) ([]coderd.Workspace, error) {
	if user == "" {
		user = "me"
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/workspaces", user), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var workspaces []coderd.Workspace
	return workspaces, json.NewDecoder(res.Body).Decode(&workspaces)
}

func (c *Client) WorkspaceByName(ctx context.Context, user, name string) (coderd.Workspace, error) {
	if user == "" {
		user = "me"
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/workspaces/%s", user, name), nil)
	if err != nil {
		return coderd.Workspace{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.Workspace{}, readBodyAsError(res)
	}
	var workspace coderd.Workspace
	return workspace, json.NewDecoder(res.Body).Decode(&workspace)
}
