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

type DeprecatedParameterScope string

const (
	DeprecatedParameterTemplate  DeprecatedParameterScope = "template"
	DeprecatedParameterWorkspace DeprecatedParameterScope = "workspace"
	DeprecatedParameterImportJob DeprecatedParameterScope = "import_job"
)

type DeprecatedParameterSourceScheme string

const (
	DeprecatedParameterSourceSchemeNone DeprecatedParameterSourceScheme = "none"
	DeprecatedParameterSourceSchemeData DeprecatedParameterSourceScheme = "data"
)

type DeprecatedParameterDestinationScheme string

const (
	DeprecatedParameterDestinationSchemeNone                DeprecatedParameterDestinationScheme = "none"
	DeprecatedParameterDestinationSchemeEnvironmentVariable DeprecatedParameterDestinationScheme = "environment_variable"
	DeprecatedParameterDestinationSchemeProvisionerVariable DeprecatedParameterDestinationScheme = "provisioner_variable"
)

type DeprecatedParameterTypeSystem string

const (
	DeprecatedParameterTypeSystemNone DeprecatedParameterTypeSystem = "none"
	DeprecatedParameterTypeSystemHCL  DeprecatedParameterTypeSystem = "hcl"
)

type DeprecatedComputedParameter struct {
	DeprecatedParameter
	SourceValue        string    `json:"source_value"`
	SchemaID           uuid.UUID `json:"schema_id"`
	DefaultSourceValue bool      `json:"default_source_value"`
}

// DeprecatedParameter represents a set value for the scope.
type DeprecatedParameter struct {
	ID                uuid.UUID                            `json:"id" table:"id"`
	Scope             DeprecatedParameterScope             `json:"scope" table:"scope"`
	ScopeID           uuid.UUID                            `json:"scope_id" table:"scope id"`
	Name              string                               `json:"name" table:"name"`
	SourceScheme      DeprecatedParameterSourceScheme      `json:"source_scheme" table:"source scheme" validate:"ne=none"`
	DestinationScheme DeprecatedParameterDestinationScheme `json:"destination_scheme" table:"destination scheme" validate:"ne=none"`
	CreatedAt         time.Time                            `json:"created_at" table:"created at"`
	UpdatedAt         time.Time                            `json:"updated_at" table:"updated at"`
}

type DeprecatedParameterSchema struct {
	ID                       uuid.UUID                            `json:"id"`
	CreatedAt                time.Time                            `json:"created_at"`
	JobID                    uuid.UUID                            `json:"job_id"`
	Name                     string                               `json:"name"`
	Description              string                               `json:"description"`
	DefaultSourceScheme      DeprecatedParameterSourceScheme      `json:"default_source_scheme"`
	DefaultSourceValue       string                               `json:"default_source_value"`
	AllowOverrideSource      bool                                 `json:"allow_override_source"`
	DefaultDestinationScheme DeprecatedParameterDestinationScheme `json:"default_destination_scheme"`
	AllowOverrideDestination bool                                 `json:"allow_override_destination"`
	DefaultRefresh           string                               `json:"default_refresh"`
	RedisplayValue           bool                                 `json:"redisplay_value"`
	ValidationError          string                               `json:"validation_error"`
	ValidationCondition      string                               `json:"validation_condition"`
	ValidationTypeSystem     string                               `json:"validation_type_system"`
	ValidationValueType      string                               `json:"validation_value_type"`

	// This is a special array of items provided if the validation condition
	// explicitly states the value must be one of a set.
	ValidationContains []string `json:"validation_contains,omitempty"`
}

// DeprecatedCreateParameterRequest is used to create a new parameter value for a scope.
type DeprecatedCreateParameterRequest struct {
	// CloneID allows copying the value of another parameter.
	// The other param must be related to the same template_id for this to
	// succeed.
	// No other fields are required if using this, as all fields will be copied
	// from the other parameter.
	CloneID uuid.UUID `json:"copy_from_parameter,omitempty" validate:""`

	Name              string                               `json:"name" validate:"required"`
	SourceValue       string                               `json:"source_value" validate:"required"`
	SourceScheme      DeprecatedParameterSourceScheme      `json:"source_scheme" validate:"oneof=data,required"`
	DestinationScheme DeprecatedParameterDestinationScheme `json:"destination_scheme" validate:"oneof=environment_variable provisioner_variable,required"`
}

func (c *Client) DeprecatedCreateParameter(ctx context.Context, scope DeprecatedParameterScope, id uuid.UUID, req DeprecatedCreateParameterRequest) (DeprecatedParameter, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/deprecated-parameters/%s/%s", scope, id.String()), req)
	if err != nil {
		return DeprecatedParameter{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return DeprecatedParameter{}, readBodyAsError(res)
	}

	var param DeprecatedParameter
	return param, json.NewDecoder(res.Body).Decode(&param)
}

func (c *Client) DeprecatedDeleteParameter(ctx context.Context, scope DeprecatedParameterScope, id uuid.UUID, name string) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/deprecated-parameters/%s/%s/%s", scope, id.String(), name), nil)
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

func (c *Client) DeprecatedParameters(ctx context.Context, scope DeprecatedParameterScope, id uuid.UUID) ([]DeprecatedParameter, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/deprecated-parameters/%s/%s", scope, id.String()), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}

	var parameters []DeprecatedParameter
	return parameters, json.NewDecoder(res.Body).Decode(&parameters)
}
