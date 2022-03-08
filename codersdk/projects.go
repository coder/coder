package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd"
)

// Project returns a single project.
func (c *Client) Project(ctx context.Context, project uuid.UUID) (coderd.Project, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projects/%s", project), nil)
	if err != nil {
		return coderd.Project{}, nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.Project{}, readBodyAsError(res)
	}
	var resp coderd.Project
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// ProjectVersionsByProject lists versions associated with a project.
func (c *Client) ProjectVersionsByProject(ctx context.Context, project uuid.UUID) ([]coderd.ProjectVersion, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projects/%s/versions", project), nil)
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

// ProjectVersionByName returns a project version by it's friendly name.
// This is used for path-based routing. Like: /projects/example/versions/helloworld
func (c *Client) ProjectVersionByName(ctx context.Context, project uuid.UUID, name string) (coderd.ProjectVersion, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projects/%s/versions/%s", project, name), nil)
	if err != nil {
		return coderd.ProjectVersion{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.ProjectVersion{}, readBodyAsError(res)
	}
	var projectVersion coderd.ProjectVersion
	return projectVersion, json.NewDecoder(res.Body).Decode(&projectVersion)
}
