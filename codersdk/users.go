package codersdk

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/coder/coder/coderd"
)

func (c *Client) User(ctx context.Context, id string) (coderd.User, error) {
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
