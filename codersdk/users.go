package codersdk

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/coder/coder/coderd"
)

// CreateInitialUser attempts to create the first user on a Coder deployment.
// This initial user has superadmin privileges. If >0 users exist, this request
// will fail.
func (c *Client) CreateInitialUser(ctx context.Context, req coderd.CreateUserRequest) (coderd.User, error) {
	res, err := c.request(ctx, http.MethodPost, "/api/v2/user", req)
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

// User returns a user for the ID provided.
// If the ID string is empty, the current user will be returned.
func (c *Client) User(ctx context.Context, _ string) (coderd.User, error) {
	res, err := c.request(ctx, http.MethodGet, "/api/v2/user", nil)
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
