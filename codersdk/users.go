package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/coder/coder/coderd"
)

// CreateInitialUser attempts to create the first user on a Coder deployment.
// This initial user has superadmin privileges. If >0 users exist, this request
// will fail.
func (c *Client) CreateInitialUser(ctx context.Context, req coderd.CreateInitialUserRequest) (coderd.User, error) {
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

// LoginWithPassword creates a session token authenticating with an email and password.
// Call `SetSessionToken()` to apply the newly acquired token to the client.
func (c *Client) LoginWithPassword(ctx context.Context, req coderd.LoginWithPasswordRequest) (coderd.LoginWithPasswordResponse, error) {
	res, err := c.request(ctx, http.MethodPost, "/api/v2/login", req)
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

// UserOrganizations fetches organizations a user is part of.
func (c *Client) UserOrganizations(ctx context.Context, id string) ([]coderd.Organization, error) {
	if id == "" {
		id = "me"
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/organizations", id), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var orgs []coderd.Organization
	return orgs, json.NewDecoder(res.Body).Decode(&orgs)
}
