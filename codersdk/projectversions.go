package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd"
)

// ProjectVersion returns a project version by ID.
func (c *Client) ProjectVersion(ctx context.Context, id uuid.UUID) (coderd.ProjectVersion, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projectversions/%s", id), nil)
	if err != nil {
		return coderd.ProjectVersion{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.ProjectVersion{}, readBodyAsError(res)
	}
	var version coderd.ProjectVersion
	return version, json.NewDecoder(res.Body).Decode(&version)
}

// ProjectVersionSchema returns schemas for a project version by ID.
func (c *Client) ProjectVersionSchema(ctx context.Context, version uuid.UUID) ([]coderd.ProjectVersionParameterSchema, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projectversions/%s/schema", version), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var params []coderd.ProjectVersionParameterSchema
	return params, json.NewDecoder(res.Body).Decode(&params)
}

// ProjectVersionParameters returns computed parameters for a project version.
func (c *Client) ProjectVersionParameters(ctx context.Context, version uuid.UUID) ([]coderd.ProjectVersionParameter, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projectversions/%s/parameters", version), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var params []coderd.ProjectVersionParameter
	return params, json.NewDecoder(res.Body).Decode(&params)
}

// ProjectVersionResources returns resources a project version declares.
func (c *Client) ProjectVersionResources(ctx context.Context, version uuid.UUID) ([]coderd.WorkspaceResource, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projectversions/%s/resources", version), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var resources []coderd.WorkspaceResource
	return resources, json.NewDecoder(res.Body).Decode(&resources)
}

// ProjectVersionLogsBefore returns logs that occurred before a specific time.
func (c *Client) ProjectVersionLogsBefore(ctx context.Context, version uuid.UUID, before time.Time) ([]coderd.ProvisionerJobLog, error) {
	return c.provisionerJobLogsBefore(ctx, fmt.Sprintf("/api/v2/projectversions/%s/logs", version), before)
}

// ProjectVersionLogsAfter streams logs for a project version that occurred after a specific time.
func (c *Client) ProjectVersionLogsAfter(ctx context.Context, version uuid.UUID, after time.Time) (<-chan coderd.ProvisionerJobLog, error) {
	return c.provisionerJobLogsAfter(ctx, fmt.Sprintf("/api/v2/projectversions/%s/logs", version), after)
}
