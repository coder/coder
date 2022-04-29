package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Me is used as a replacement for your own ID.
var Me = uuid.Nil

type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusSuspended UserStatus = "suspended"
)

type UsersRequest struct {
	AfterUser uuid.UUID `json:"after_user"`
	Search    string    `json:"search"`
	// Limit sets the maximum number of users to be returned
	// in a single page. If the limit is <= 0, there is no limit
	// and all users are returned.
	Limit int `json:"limit"`
	// Offset is used to indicate which page to return. An offset of 0
	// returns the first 'limit' number of users.
	// To get the next page, use offset=<limit>*<page_number>.
	// Offset is 0 indexed, so the first record sits at offset 0.
	Offset int `json:"offset"`
	// Filter users by status
	Status string `json:"status"`
}

// User represents a user in Coder.
type User struct {
	ID              uuid.UUID   `json:"id" validate:"required"`
	Email           string      `json:"email" validate:"required"`
	CreatedAt       time.Time   `json:"created_at" validate:"required"`
	Username        string      `json:"username" validate:"required"`
	Status          UserStatus  `json:"status"`
	OrganizationIDs []uuid.UUID `json:"organization_ids"`
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
	Email    string `json:"email" validate:"required,email"`
	Username string `json:"username" validate:"required,username"`
}

type UpdateRoles struct {
	Roles []string `json:"roles" validate:"required"`
}

type UserRoles struct {
	Roles             []string               `json:"roles"`
	OrganizationRoles map[uuid.UUID][]string `json:"organization_roles"`
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

// AuthMethods contains whether authentication types are enabled or not.
type AuthMethods struct {
	Password bool `json:"password"`
	Github   bool `json:"github"`
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

// SuspendUser enables callers to suspend a user
func (c *Client) SuspendUser(ctx context.Context, userID uuid.UUID) (User, error) {
	res, err := c.request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/users/%s/suspend", uuidOrMe(userID)), nil)
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

// UpdateUserRoles grants the userID the specified roles.
// Include ALL roles the user has.
func (c *Client) UpdateUserRoles(ctx context.Context, userID uuid.UUID, req UpdateRoles) (User, error) {
	res, err := c.request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/users/%s/roles", uuidOrMe(userID)), req)
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

// UpdateOrganizationMemberRoles grants the userID the specified roles in an org.
// Include ALL roles the user has.
func (c *Client) UpdateOrganizationMemberRoles(ctx context.Context, organizationID, userID uuid.UUID, req UpdateRoles) (User, error) {
	res, err := c.request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/organizations/%s/members/%s/roles", organizationID, uuidOrMe(userID)), req)
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

// GetUserRoles returns all roles the user has
func (c *Client) GetUserRoles(ctx context.Context, userID uuid.UUID) (UserRoles, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/roles", uuidOrMe(userID)), nil)
	if err != nil {
		return UserRoles{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return UserRoles{}, readBodyAsError(res)
	}
	var roles UserRoles
	return roles, json.NewDecoder(res.Body).Decode(&roles)
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
	return c.userByIdentifier(ctx, uuidOrMe(id))
}

// UserByUsername returns a user for the username provided.
func (c *Client) UserByUsername(ctx context.Context, username string) (User, error) {
	return c.userByIdentifier(ctx, username)
}

func (c *Client) userByIdentifier(ctx context.Context, ident string) (User, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s", ident), nil)
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

// Users returns all users according to the request parameters. If no parameters are set,
// the default behavior is to return all users in a single page.
func (c *Client) Users(ctx context.Context, req UsersRequest) ([]User, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users"), nil, func(r *http.Request) {
		q := r.URL.Query()
		if req.AfterUser != uuid.Nil {
			q.Set("after_user", req.AfterUser.String())
		}
		if req.Limit > 0 {
			q.Set("limit", strconv.Itoa(req.Limit))
		}
		q.Set("offset", strconv.Itoa(req.Offset))
		q.Set("search", req.Search)
		q.Set("status", req.Status)
		r.URL.RawQuery = q.Encode()
	})
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

// AuthMethods returns types of authentication available to the user.
func (c *Client) AuthMethods(ctx context.Context) (AuthMethods, error) {
	res, err := c.request(ctx, http.MethodGet, "/api/v2/users/authmethods", nil)
	if err != nil {
		return AuthMethods{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return AuthMethods{}, readBodyAsError(res)
	}

	var userAuth AuthMethods
	return userAuth, json.NewDecoder(res.Body).Decode(&userAuth)
}

// uuidOrMe returns the provided uuid as a string if it's valid, ortherwise
// `me`.
func uuidOrMe(id uuid.UUID) string {
	if id == Me {
		return "me"
	}

	return id.String()
}
