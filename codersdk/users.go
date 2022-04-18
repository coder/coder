package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Me is used as a replacement for your own ID.
var Me = uuid.Nil

// User represents a user in Coder.
type User struct {
	ID        uuid.UUID `json:"id" validate:"required"`
	Email     string    `json:"email" validate:"required"`
	CreatedAt time.Time `json:"created_at" validate:"required"`
	Username  string    `json:"username" validate:"required"`
	Name      string    `json:"name"`
}

type CreateFirstUserRequest struct {
	Email            string `json:"email" validate:"required,email"`
	Username         string `json:"username" validate:"required,username"`
	Password         string `json:"password" validate:"required"`
	OrganizationName string `json:"organization" validate:"required,username"`
}

// CreateFirstUserResponse contains IDs for newly created user info.
type CreateFirstUserResponse struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
}

type CreateUserRequest struct {
	Email          string    `json:"email" validate:"required,email"`
	Username       string    `json:"username" validate:"required,username"`
	Password       string    `json:"password" validate:"required"`
	OrganizationID uuid.UUID `json:"organization_id" validate:"required"`
}

type UpdateUserProfileRequest struct {
	Email    string  `json:"email" validate:"required,email"`
	Username string  `json:"username" validate:"required,username"`
	Name     *string `json:"name"`
}

// LoginWithPasswordRequest enables callers to authenticate with email and password.
type LoginWithPasswordRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// LoginWithPasswordResponse contains a session token for the newly authenticated user.
type LoginWithPasswordResponse struct {
	SessionToken string `json:"session_token" validate:"required"`
}

// GenerateAPIKeyResponse contains an API key for a user.
type GenerateAPIKeyResponse struct {
	Key string `json:"key"`
}

type CreateOrganizationRequest struct {
	Name string `json:"name" validate:"required,username"`
}

// CreateWorkspaceRequest provides options for creating a new workspace.
type CreateWorkspaceRequest struct {
	TemplateID uuid.UUID `json:"template_id" validate:"required"`
	Name       string    `json:"name" validate:"username,required"`
	// ParameterValues allows for additional parameters to be provided
	// during the initial provision.
	ParameterValues []CreateParameterRequest `json:"parameter_values"`
}

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
func (c *Client) CreateFirstUser(ctx context.Context, req CreateFirstUserRequest) (CreateFirstUserResponse, error) {
	res, err := c.request(ctx, http.MethodPost, "/api/v2/users/first", req)
	if err != nil {
		return CreateFirstUserResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return CreateFirstUserResponse{}, readBodyAsError(res)
	}
	var resp CreateFirstUserResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// CreateUser creates a new user.
func (c *Client) CreateUser(ctx context.Context, req CreateUserRequest) (User, error) {
	res, err := c.request(ctx, http.MethodPost, "/api/v2/users", req)
	if err != nil {
		return User{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return User{}, readBodyAsError(res)
	}
	var user User
	return user, json.NewDecoder(res.Body).Decode(&user)
}

// UpdateUserProfile enables callers to update profile information
func (c *Client) UpdateUserProfile(ctx context.Context, userID uuid.UUID, req UpdateUserProfileRequest) (User, error) {
	res, err := c.request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/users/%s/profile", uuidOrMe(userID)), req)
	if err != nil {
		return User{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return User{}, readBodyAsError(res)
	}
	var user User
	return user, json.NewDecoder(res.Body).Decode(&user)
}

func (c *Client) GetUsers(ctx context.Context) ([]User, error) {
	res, err := c.request(ctx, http.MethodGet, "/api/v2/users", nil)
	if err != nil {
		return []User{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return []User{}, readBodyAsError(res)
	}
	var users []User
	return users, json.NewDecoder(res.Body).Decode(&users)
}

// CreateAPIKey generates an API key for the user ID provided.
func (c *Client) CreateAPIKey(ctx context.Context, userID uuid.UUID) (*GenerateAPIKeyResponse, error) {
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/keys", uuidOrMe(userID)), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusCreated {
		return nil, readBodyAsError(res)
	}
	apiKey := &GenerateAPIKeyResponse{}
	return apiKey, json.NewDecoder(res.Body).Decode(apiKey)
}

// LoginWithPassword creates a session token authenticating with an email and password.
// Call `SetSessionToken()` to apply the newly acquired token to the client.
func (c *Client) LoginWithPassword(ctx context.Context, req LoginWithPasswordRequest) (LoginWithPasswordResponse, error) {
	res, err := c.request(ctx, http.MethodPost, "/api/v2/users/login", req)
	if err != nil {
		return LoginWithPasswordResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return LoginWithPasswordResponse{}, readBodyAsError(res)
	}
	var resp LoginWithPasswordResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return LoginWithPasswordResponse{}, err
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
// If the uuid is nil, the current user will be returned.
func (c *Client) User(ctx context.Context, id uuid.UUID) (User, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s", uuidOrMe(id)), nil)
	if err != nil {
		return User{}, err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusOK {
		return User{}, readBodyAsError(res)
	}
	var user User
	return user, json.NewDecoder(res.Body).Decode(&user)
}

// OrganizationsByUser returns all organizations the user is a member of.
func (c *Client) OrganizationsByUser(ctx context.Context, userID uuid.UUID) ([]Organization, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/organizations", uuidOrMe(userID)), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var orgs []Organization
	return orgs, json.NewDecoder(res.Body).Decode(&orgs)
}

func (c *Client) OrganizationByName(ctx context.Context, userID uuid.UUID, name string) (Organization, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/organizations/%s", uuidOrMe(userID), name), nil)
	if err != nil {
		return Organization{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Organization{}, readBodyAsError(res)
	}
	var org Organization
	return org, json.NewDecoder(res.Body).Decode(&org)
}

// CreateOrganization creates an organization and adds the provided user as an admin.
func (c *Client) CreateOrganization(ctx context.Context, userID uuid.UUID, req CreateOrganizationRequest) (Organization, error) {
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/organizations", uuidOrMe(userID)), req)
	if err != nil {
		return Organization{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return Organization{}, readBodyAsError(res)
	}

	var org Organization
	return org, json.NewDecoder(res.Body).Decode(&org)
}

// CreateWorkspace creates a new workspace for the template specified.
func (c *Client) CreateWorkspace(ctx context.Context, userID uuid.UUID, request CreateWorkspaceRequest) (Workspace, error) {
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/workspaces", uuidOrMe(userID)), request)
	if err != nil {
		return Workspace{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return Workspace{}, readBodyAsError(res)
	}

	var workspace Workspace
	return workspace, json.NewDecoder(res.Body).Decode(&workspace)
}

// WorkspacesByUser returns all workspaces the specified user has access to.
func (c *Client) WorkspacesByUser(ctx context.Context, userID uuid.UUID) ([]Workspace, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/workspaces", uuidOrMe(userID)), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}

	var workspaces []Workspace
	return workspaces, json.NewDecoder(res.Body).Decode(&workspaces)
}

func (c *Client) WorkspaceByName(ctx context.Context, userID uuid.UUID, name string) (Workspace, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/workspaces/%s", uuidOrMe(userID), name), nil)
	if err != nil {
		return Workspace{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return Workspace{}, readBodyAsError(res)
	}

	var workspace Workspace
	return workspace, json.NewDecoder(res.Body).Decode(&workspace)
}

// uuidOrMe returns the provided uuid as a string if it's valid, ortherwise
// `me`.
func uuidOrMe(id uuid.UUID) string {
	if id == Me {
		return "me"
	}

	return id.String()
}
