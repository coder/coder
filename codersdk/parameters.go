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
	SchemaID           uuid.UUID `json:"schema_id" format:"uuid"`
	DefaultSourceValue bool      `json:"default_source_value"`
}

// Parameter represents a set value for the scope.
//
// @Description Parameter represents a set value for the scope.
type Parameter struct {
	ID                uuid.UUID                  `json:"id" table:"id" format:"uuid"`
	Scope             ParameterScope             `json:"scope" table:"scope" enums:"template,workspace,import_job"`
	ScopeID           uuid.UUID                  `json:"scope_id" table:"scope id" format:"uuid"`
	Name              string                     `json:"name" table:"name,default_sort"`
	SourceScheme      ParameterSourceScheme      `json:"source_scheme" table:"source scheme" validate:"ne=none" enums:"none,data"`
	DestinationScheme ParameterDestinationScheme `json:"destination_scheme" table:"destination scheme" validate:"ne=none" enums:"none,environment_variable,provisioner_variable"`
	CreatedAt         time.Time                  `json:"created_at" table:"created at" format:"date-time"`
	UpdatedAt         time.Time                  `json:"updated_at" table:"updated at" format:"date-time"`
}

type ParameterSchema struct {
	ID                       uuid.UUID                  `json:"id" format:"uuid"`
	CreatedAt                time.Time                  `json:"created_at" format:"date-time"`
	JobID                    uuid.UUID                  `json:"job_id" format:"uuid"`
	Name                     string                     `json:"name"`
	Description              string                     `json:"description"`
	DefaultSourceScheme      ParameterSourceScheme      `json:"default_source_scheme" enums:"none,data"`
	DefaultSourceValue       string                     `json:"default_source_value"`
	AllowOverrideSource      bool                       `json:"allow_override_source"`
	DefaultDestinationScheme ParameterDestinationScheme `json:"default_destination_scheme" enums:"none,environment_variable,provisioner_variable"`
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

// CreateParameterRequest is a structure used to create a new parameter value for a scope.
//
// @Description CreateParameterRequest is a structure used to create a new parameter value for a scope.
type CreateParameterRequest struct {
	// CloneID allows copying the value of another parameter.
	// The other param must be related to the same template_id for this to
	// succeed.
	// No other fields are required if using this, as all fields will be copied
	// from the other parameter.
	CloneID uuid.UUID `json:"copy_from_parameter,omitempty" validate:"" format:"uuid"`

	Name              string                     `json:"name" validate:"required"`
	SourceValue       string                     `json:"source_value" validate:"required"`
	SourceScheme      ParameterSourceScheme      `json:"source_scheme" validate:"oneof=data,required" enums:"none,data"`
	DestinationScheme ParameterDestinationScheme `json:"destination_scheme" validate:"oneof=environment_variable provisioner_variable,required" enums:"none,environment_variable,provisioner_variable"`
}

func (c *Client) CreateParameter(ctx context.Context, scope ParameterScope, id uuid.UUID, req CreateParameterRequest) (Parameter, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/parameters/%s/%s", scope, id.String()), req)
	if err != nil {
		return Parameter{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return Parameter{}, ReadBodyAsError(res)
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
		return ReadBodyAsError(res)
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
		return nil, ReadBodyAsError(res)
	}

	var parameters []Parameter
	return parameters, json.NewDecoder(res.Body).Decode(&parameters)
}
