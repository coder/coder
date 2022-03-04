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

// ImportProjectVersion processes source-code and optionally associates the version with a project.
// Executing without a project is useful for validating source-code.
func (c *Client) ImportProjectVersion(ctx context.Context, organization uuid.UUID, req coderd.CreateProjectVersion) (coderd.ProjectVersion, error) {
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/projectversion", organization), req)
	if err != nil {
		return coderd.ProjectVersion{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return coderd.ProjectVersion{}, readBodyAsError(res)
	}
	var projectVersion coderd.ProjectVersion
	return projectVersion, json.NewDecoder(res.Body).Decode(&projectVersion)
}

// ProjectVersions lists versions associated with a project.
func (c *Client) ProjectVersions(ctx context.Context, project uuid.UUID) ([]coderd.ProjectVersion, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/project/%s/versions", project), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var projectVersion []coderd.ProjectVersion
	return projectVersion, json.NewDecoder(res.Body).Decode(&projectVersion)
}

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
	return c.provisionerJobLogsBefore(ctx, "projectimport", organization, job, before)
}

// ProjectVersionLogsAfter streams logs for a project version that occurred after a specific time.
func (c *Client) ProjectVersionLogsAfter(ctx context.Context, organization string, job uuid.UUID, after time.Time) (<-chan coderd.ProvisionerJobLog, error) {
	return c.provisionerJobLogsAfter(ctx, "projectimport", organization, job, after)
}
