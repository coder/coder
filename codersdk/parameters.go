package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type ParameterScope string

const (
	ParameterTemplate  ParameterScope = "template"
	ParameterWorkspace ParameterScope = "workspace"
	ParameterImportJob ParameterScope = "import_job"
)

type ParameterSourceScheme string

const (
	ParameterSourceSchemeNone ParameterSourceScheme = "none"
	ParameterSourceSchemeData ParameterSourceScheme = "data"
)

type ParameterDestinationScheme string

const (
	ParameterDestinationSchemeNone                ParameterDestinationScheme = "none"
	ParameterDestinationSchemeEnvironmentVariable ParameterDestinationScheme = "environment_variable"
	ParameterDestinationSchemeProvisionerVariable ParameterDestinationScheme = "provisioner_variable"
)

type ParameterTypeSystem string

const (
	ParameterTypeSystemNone ParameterTypeSystem = "none"
	ParameterTypeSystemHCL  ParameterTypeSystem = "hcl"
)

type ComputedParameter struct {
	Parameter
	SourceValue        string    `json:"source_value"`
	SchemaID           uuid.UUID `json:"schema_id"`
	DefaultSourceValue bool      `json:"default_source_value"`
}

// Parameter represents a set value for the scope.
type Parameter struct {
	ID                uuid.UUID                  `json:"id"`
	CreatedAt         time.Time                  `json:"created_at"`
	UpdatedAt         time.Time                  `json:"updated_at"`
	Scope             ParameterScope             `json:"scope"`
	ScopeID           uuid.UUID                  `json:"scope_id"`
	Name              string                     `json:"name"`
	SourceScheme      ParameterSourceScheme      `json:"source_scheme"`
	DestinationScheme ParameterDestinationScheme `json:"destination_scheme"`
}

type ParameterSchema struct {
	ID                       uuid.UUID                  `json:"id"`
	CreatedAt                time.Time                  `json:"created_at"`
	JobID                    uuid.UUID                  `json:"job_id"`
	Name                     string                     `json:"name"`
	Description              string                     `json:"description"`
	DefaultSourceScheme      ParameterSourceScheme      `json:"default_source_scheme"`
	DefaultSourceValue       string                     `json:"default_source_value"`
	AllowOverrideSource      bool                       `json:"allow_override_source"`
	DefaultDestinationScheme ParameterDestinationScheme `json:"default_destination_scheme"`
	AllowOverrideDestination bool                       `json:"allow_override_destination"`
	DefaultRefresh           string                     `json:"default_refresh"`
	RedisplayValue           bool                       `json:"redisplay_value"`
	ValidationError          string                     `json:"validation_error"`
	ValidationCondition      string                     `json:"validation_condition"`
	ValidationTypeSystem     string                     `json:"validation_type_system"`
	ValidationValueType      string                     `json:"validation_value_type"`

	// This is a special array of items provided if the validation condition
	// explicitly states the value must be one of a set.
	ValidationContains []string `json:"validation_contains,omitempty"`
}

// CreateParameterRequest is used to create a new parameter value for a scope.
type CreateParameterRequest struct {
	// CloneID allows copying the value of another parameter.
	// The other param must be related to the same template_id for this to
	// succeed.
	// No other fields are required if using this, as all fields will be copied
	// from the other parameter.
	CloneID uuid.UUID `json:"copy_from_parameter,omitempty" validate:""`

	Name              string                     `json:"name" validate:"required"`
	SourceValue       string                     `json:"source_value" validate:"required"`
	SourceScheme      ParameterSourceScheme      `json:"source_scheme" validate:"oneof=data,required"`
	DestinationScheme ParameterDestinationScheme `json:"destination_scheme" validate:"oneof=environment_variable provisioner_variable,required"`
}

func (c *Client) CreateParameter(ctx context.Context, scope ParameterScope, id uuid.UUID, req CreateParameterRequest) (Parameter, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/parameters/%s/%s", scope, id.String()), req)
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

func (c *Client) DeleteParameter(ctx context.Context, scope ParameterScope, id uuid.UUID, name string) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/parameters/%s/%s/%s", scope, id.String(), name), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return readBodyAsError(res)
	}

	_, _ = io.Copy(io.Discard, res.Body)
	return nil
}

func (c *Client) Parameters(ctx context.Context, scope ParameterScope, id uuid.UUID) ([]Parameter, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/parameters/%s/%s", scope, id.String()), nil)
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
