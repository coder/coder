package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
)

// Project is the JSON representation of a Coder project. This type matches the
// database object for now, but is abstracted for ease of change later on.
type Project struct {
	ID                  uuid.UUID                `json:"id"`
	CreatedAt           time.Time                `json:"created_at"`
	UpdatedAt           time.Time                `json:"updated_at"`
	OrganizationID      uuid.UUID                `json:"organization_id"`
	Name                string                   `json:"name"`
	Provisioner         database.ProvisionerType `json:"provisioner"`
	ActiveVersionID     uuid.UUID                `json:"active_version_id"`
	WorkspaceOwnerCount uint32                   `json:"workspace_owner_count"`
}

type UpdateActiveProjectVersion struct {
	ID uuid.UUID `json:"id" validate:"required"`
}

// Project returns a single project.
func (c *Client) Project(ctx context.Context, project uuid.UUID) (Project, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projects/%s", project), nil)
	if err != nil {
		return Project{}, nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Project{}, readBodyAsError(res)
	}
	var resp Project
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) DeleteProject(ctx context.Context, project uuid.UUID) error {
	res, err := c.request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/projects/%s", project), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return readBodyAsError(res)
	}
	return nil
}

// UpdateActiveProjectVersion updates the active project version to the ID provided.
// The project version must be attached to the project.
func (c *Client) UpdateActiveProjectVersion(ctx context.Context, project uuid.UUID, req UpdateActiveProjectVersion) error {
	res, err := c.request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/projects/%s/versions", project), req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return readBodyAsError(res)
	}
	return nil
}

// ProjectVersionsByProject lists versions associated with a project.
func (c *Client) ProjectVersionsByProject(ctx context.Context, project uuid.UUID) ([]ProjectVersion, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projects/%s/versions", project), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var projectVersion []ProjectVersion
	return projectVersion, json.NewDecoder(res.Body).Decode(&projectVersion)
}

// ProjectVersionByName returns a project version by it's friendly name.
// This is used for path-based routing. Like: /projects/example/versions/helloworld
func (c *Client) ProjectVersionByName(ctx context.Context, project uuid.UUID, name string) (ProjectVersion, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projects/%s/versions/%s", project, name), nil)
	if err != nil {
		return ProjectVersion{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ProjectVersion{}, readBodyAsError(res)
	}
	var projectVersion ProjectVersion
	return projectVersion, json.NewDecoder(res.Body).Decode(&projectVersion)
}
