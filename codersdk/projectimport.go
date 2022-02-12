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

// CreateProjectImportJob creates a new import job in the organization provided.
// ProjectImportJob is not associated with a project by default. Projects
// are created from import.
func (c *Client) CreateProjectImportJob(ctx context.Context, organization string, req coderd.CreateProjectImportJobRequest) (coderd.ProvisionerJob, error) {
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/projectimport/%s", organization), req)
	if err != nil {
		return coderd.ProvisionerJob{}, err
	}
	if res.StatusCode != http.StatusCreated {
		defer res.Body.Close()
		return coderd.ProvisionerJob{}, readBodyAsError(res)
	}
	var job coderd.ProvisionerJob
	return job, json.NewDecoder(res.Body).Decode(&job)
}

// ProjectImportJob returns an import job by ID.
func (c *Client) ProjectImportJob(ctx context.Context, organization string, job uuid.UUID) (coderd.ProvisionerJob, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projectimport/%s/%s", organization, job), nil)
	if err != nil {
		return coderd.ProvisionerJob{}, nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.ProvisionerJob{}, readBodyAsError(res)
	}
	var resp coderd.ProvisionerJob
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// ProjectImportJobLogsBefore returns logs that occurred before a specific time.
func (c *Client) ProjectImportJobLogsBefore(ctx context.Context, organization string, job uuid.UUID, before time.Time) ([]coderd.ProvisionerJobLog, error) {
	return c.provisionerJobLogsBefore(ctx, "projectimport", organization, job, before)
}

// ProjectImportJobLogsAfter streams logs for a project import operation that occurred after a specific time.
func (c *Client) ProjectImportJobLogsAfter(ctx context.Context, organization string, job uuid.UUID, after time.Time) (<-chan coderd.ProvisionerJobLog, error) {
	return c.provisionerJobLogsAfter(ctx, "projectimport", organization, job, after)
}

// ProjectImportJobSchemas returns schemas for an import job by ID.
func (c *Client) ProjectImportJobSchemas(ctx context.Context, organization string, job uuid.UUID) ([]coderd.ParameterSchema, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projectimport/%s/%s/schemas", organization, job), nil)
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

// ProjectImportJobParameters returns computed parameters for a project import job.
func (c *Client) ProjectImportJobParameters(ctx context.Context, organization string, job uuid.UUID) ([]coderd.ComputedParameterValue, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projectimport/%s/%s/parameters", organization, job), nil)
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

// ProjectImportJobResources returns resources for a project import job.
func (c *Client) ProjectImportJobResources(ctx context.Context, organization string, job uuid.UUID) ([]coderd.ProjectImportJobResource, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projectimport/%s/%s/resources", organization, job), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var resources []coderd.ProjectImportJobResource
	return resources, json.NewDecoder(res.Body).Decode(&resources)
}
