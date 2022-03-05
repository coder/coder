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
func (c *Client) ProjectVersion(ctx context.Context, version uuid.UUID) (coderd.ProjectVersion, error) {
	return coderd.ProjectVersion{}, nil
}

// ProjectVersionByName returns a project version by it's friendly name.
// This is used for path-based routing. Like: /projects/example/versions/helloworld
func (c *Client) ProjectVersionByName(ctx context.Context, project uuid.UUID, name string) (coderd.ProjectVersion, error) {
	return coderd.ProjectVersion{}, nil
}

// ProjectVersionSchema returns schemas for a project version by ID.
func (c *Client) ProjectVersionSchema(ctx context.Context, version uuid.UUID) ([]coderd.ParameterSchema, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projectversion/%s/schema", version), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var params []coderd.ParameterSchema
	return params, json.NewDecoder(res.Body).Decode(&params)
}

// ProjectVersionParameters returns computed parameters for a project version.
func (c *Client) ProjectVersionComputedParameters(ctx context.Context, version uuid.UUID) ([]coderd.ComputedParameterValue, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projectimport/%s/parameters", version), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var params []coderd.ComputedParameterValue
	return params, json.NewDecoder(res.Body).Decode(&params)
}

// ProjectVersionResources returns resources a project version declares.
func (c *Client) ProjectVersionResources(ctx context.Context, version uuid.UUID) ([]coderd.ProvisionerJobResource, error) {
	return nil, nil
}

// ProjectVersionLogsBefore returns logs that occurred before a specific time.
func (c *Client) ProjectVersionLogsBefore(ctx context.Context, version uuid.UUID, before time.Time) ([]coderd.ProvisionerJobLog, error) {
	return c.provisionerJobLogsBefore(ctx, "projectimport", "", version, before)
}

// ProjectVersionLogsAfter streams logs for a project version that occurred after a specific time.
func (c *Client) ProjectVersionLogsAfter(ctx context.Context, organization string, job uuid.UUID, after time.Time) (<-chan coderd.ProvisionerJobLog, error) {
	return c.provisionerJobLogsAfter(ctx, "projectimport", organization, job, after)
}
