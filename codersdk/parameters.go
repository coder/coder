package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/coder/coder/coderd"
)

func (c *Client) CreateParameter(ctx context.Context, scope coderd.ParameterScope, id string, req coderd.CreateParameterRequest) (coderd.Parameter, error) {
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/parameters/%s/%s", scope, id), req)
	if err != nil {
		return coderd.Parameter{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return coderd.Parameter{}, readBodyAsError(res)
	}
	var param coderd.Parameter
	return param, json.NewDecoder(res.Body).Decode(&param)
}

func (c *Client) DeleteParameter(ctx context.Context, scope coderd.ParameterScope, id, name string) error {
	res, err := c.request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/parameters/%s/%s/%s", scope, id, name), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return readBodyAsError(res)
	}
	return nil
}

func (c *Client) Parameters(ctx context.Context, scope coderd.ParameterScope, id string) ([]coderd.Parameter, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/parameters/%s/%s", scope, id), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var parameters []coderd.Parameter
	return parameters, json.NewDecoder(res.Body).Decode(&parameters)
}
