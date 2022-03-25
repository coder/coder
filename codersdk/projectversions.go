package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/parameter"
)

// ProjectVersion represents a single version of a project.
type ProjectVersion struct {
	ID        uuid.UUID      `json:"id"`
	ProjectID *uuid.UUID     `json:"project_id,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Name      string         `json:"name"`
	Job       ProvisionerJob `json:"job"`
}

// ProjectVersionParameterSchema represents a parameter parsed from project version source.
type ProjectVersionParameterSchema database.ParameterSchema

// ProjectVersionParameter represents a computed parameter value.
type ProjectVersionParameter parameter.ComputedValue

// ProjectVersion returns a project version by ID.
func (c *Client) ProjectVersion(ctx context.Context, id uuid.UUID) (ProjectVersion, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projectversions/%s", id), nil)
	if err != nil {
		return ProjectVersion{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ProjectVersion{}, readBodyAsError(res)
	}
	var version ProjectVersion
	return version, json.NewDecoder(res.Body).Decode(&version)
}

// CancelProjectVersion marks a project version job as canceled.
func (c *Client) CancelProjectVersion(ctx context.Context, version uuid.UUID) error {
	res, err := c.request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/projectversions/%s/cancel", version), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return readBodyAsError(res)
	}
	return nil
}

// ProjectVersionSchema returns schemas for a project version by ID.
func (c *Client) ProjectVersionSchema(ctx context.Context, version uuid.UUID) ([]ProjectVersionParameterSchema, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projectversions/%s/schema", version), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var params []ProjectVersionParameterSchema
	return params, json.NewDecoder(res.Body).Decode(&params)
}

// ProjectVersionParameters returns computed parameters for a project version.
func (c *Client) ProjectVersionParameters(ctx context.Context, version uuid.UUID) ([]ProjectVersionParameter, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projectversions/%s/parameters", version), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var params []ProjectVersionParameter
	return params, json.NewDecoder(res.Body).Decode(&params)
}

// ProjectVersionResources returns resources a project version declares.
func (c *Client) ProjectVersionResources(ctx context.Context, version uuid.UUID) ([]WorkspaceResource, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projectversions/%s/resources", version), nil)
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

// ProjectVersionLogsBefore returns logs that occurred before a specific time.
func (c *Client) ProjectVersionLogsBefore(ctx context.Context, version uuid.UUID, before time.Time) ([]ProvisionerJobLog, error) {
	return c.provisionerJobLogsBefore(ctx, fmt.Sprintf("/api/v2/projectversions/%s/logs", version), before)
}

// ProjectVersionLogsAfter streams logs for a project version that occurred after a specific time.
func (c *Client) ProjectVersionLogsAfter(ctx context.Context, version uuid.UUID, after time.Time) (<-chan ProvisionerJobLog, error) {
	return c.provisionerJobLogsAfter(ctx, fmt.Sprintf("/api/v2/projectversions/%s/logs", version), after)
}
