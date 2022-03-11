package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/database"
)

type ParameterScope string

const (
	ParameterOrganization ParameterScope = "organization"
	ParameterProject      ParameterScope = "project"
	ParameterUser         ParameterScope = "user"
	ParameterWorkspace    ParameterScope = "workspace"
)

// Parameter represents a set value for the scope.
type Parameter struct {
	ID                uuid.UUID                           `db:"id" json:"id"`
	CreatedAt         time.Time                           `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time                           `db:"updated_at" json:"updated_at"`
	Scope             ParameterScope                      `db:"scope" json:"scope"`
	ScopeID           string                              `db:"scope_id" json:"scope_id"`
	Name              string                              `db:"name" json:"name"`
	SourceScheme      database.ParameterSourceScheme      `db:"source_scheme" json:"source_scheme"`
	DestinationScheme database.ParameterDestinationScheme `db:"destination_scheme" json:"destination_scheme"`
}

// CreateParameterRequest is used to create a new parameter value for a scope.
type CreateParameterRequest struct {
	Name              string                              `json:"name" validate:"required"`
	SourceValue       string                              `json:"source_value" validate:"required"`
	SourceScheme      database.ParameterSourceScheme      `json:"source_scheme" validate:"oneof=data,required"`
	DestinationScheme database.ParameterDestinationScheme `json:"destination_scheme" validate:"oneof=environment_variable provisioner_variable,required"`
}

func (c *Client) CreateParameter(ctx context.Context, scope ParameterScope, id string, req CreateParameterRequest) (Parameter, error) {
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/parameters/%s/%s", scope, id), req)
	if err != nil {
		return Parameter{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return Parameter{}, readBodyAsError(res)
	}
	var param Parameter
	return param, json.NewDecoder(res.Body).Decode(&param)
}

func (c *Client) DeleteParameter(ctx context.Context, scope ParameterScope, id, name string) error {
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

func (c *Client) Parameters(ctx context.Context, scope ParameterScope, id string) ([]Parameter, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/parameters/%s/%s", scope, id), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var parameters []Parameter
	return parameters, json.NewDecoder(res.Body).Decode(&parameters)
}
