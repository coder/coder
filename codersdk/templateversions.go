package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// TemplateVersion represents a single version of a template.
type TemplateVersion struct {
	ID         uuid.UUID      `json:"id"`
	TemplateID *uuid.UUID     `json:"template_id,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Name       string         `json:"name"`
	Job        ProvisionerJob `json:"job"`
	Readme     string         `json:"readme"`
}

// TemplateVersionParameter represents a computed parameter value.
type TemplateVersionParameter struct {
	ID                 uuid.UUID                  `json:"id"`
	CreatedAt          time.Time                  `json:"created_at"`
	UpdatedAt          time.Time                  `json:"updated_at"`
	Scope              ParameterScope             `json:"scope"`
	ScopeID            uuid.UUID                  `json:"scope_id"`
	Name               string                     `json:"name"`
	SourceScheme       ParameterSourceScheme      `json:"source_scheme"`
	SourceValue        string                     `json:"source_value"`
	DestinationScheme  ParameterDestinationScheme `json:"destination_scheme"`
	SchemaID           uuid.UUID                  `json:"schema_id"`
	DefaultSourceValue bool                       `json:"default_source_value"`
}

// TemplateVersion returns a template version by ID.
func (c *Client) TemplateVersion(ctx context.Context, id uuid.UUID) (TemplateVersion, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templateversions/%s", id), nil)
	if err != nil {
		return TemplateVersion{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return TemplateVersion{}, readBodyAsError(res)
	}
	var version TemplateVersion
	return version, json.NewDecoder(res.Body).Decode(&version)
}

// CancelTemplateVersion marks a template version job as canceled.
func (c *Client) CancelTemplateVersion(ctx context.Context, version uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/templateversions/%s/cancel", version), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return readBodyAsError(res)
	}
	return nil
}

// TemplateVersionSchema returns schemas for a template version by ID.
func (c *Client) TemplateVersionSchema(ctx context.Context, version uuid.UUID) ([]ParameterSchema, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templateversions/%s/schema", version), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var params []ParameterSchema
	return params, json.NewDecoder(res.Body).Decode(&params)
}

// TemplateVersionParameters returns computed parameters for a template version.
func (c *Client) TemplateVersionParameters(ctx context.Context, version uuid.UUID) ([]TemplateVersionParameter, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templateversions/%s/parameters", version), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var params []TemplateVersionParameter
	return params, json.NewDecoder(res.Body).Decode(&params)
}

// TemplateVersionResources returns resources a template version declares.
func (c *Client) TemplateVersionResources(ctx context.Context, version uuid.UUID) ([]WorkspaceResource, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templateversions/%s/resources", version), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var resources []WorkspaceResource
	return resources, json.NewDecoder(res.Body).Decode(&resources)
}

// TemplateVersionLogsBefore returns logs that occurred before a specific time.
func (c *Client) TemplateVersionLogsBefore(ctx context.Context, version uuid.UUID, before time.Time) ([]ProvisionerJobLog, error) {
	return c.provisionerJobLogsBefore(ctx, fmt.Sprintf("/api/v2/templateversions/%s/logs", version), before)
}

// TemplateVersionLogsAfter streams logs for a template version that occurred after a specific time.
func (c *Client) TemplateVersionLogsAfter(ctx context.Context, version uuid.UUID, after time.Time) (<-chan ProvisionerJobLog, error) {
	return c.provisionerJobLogsAfter(ctx, fmt.Sprintf("/api/v2/templateversions/%s/logs", version), after)
}
