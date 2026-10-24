package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// GitHubRunnerPool manages ephemeral GitHub Actions runners in workspaces.
type GitHubRunnerPool struct {
	ID                    uuid.UUID `json:"id" format:"uuid"`
	OrganizationID        uuid.UUID `json:"organization_id" format:"uuid"`
	OwnerID               uuid.UUID `json:"owner_id" format:"uuid"`
	TemplateID            uuid.UUID `json:"template_id" format:"uuid"`
	Name                  string    `json:"name"`
	GitHubConfigURL       string    `json:"github_config_url"`
	RunnerGroupID         int64     `json:"runner_group_id"`
	ScaleSetID            int64     `json:"scale_set_id"`
	MaxRunners            int32     `json:"max_runners"`
	State                 string    `json:"state" enums:"active,paused,draining,deleted"`
	StartupTimeoutSeconds int32     `json:"startup_timeout_seconds"`
	JobTimeoutSeconds     int32     `json:"job_timeout_seconds"`
	AssignedJobs          int32     `json:"assigned_jobs"`
	LastError             string    `json:"last_error"`
	CreatedAt             time.Time `json:"created_at" format:"date-time"`
	UpdatedAt             time.Time `json:"updated_at" format:"date-time"`
}

// GitHubRunner associates one disposable workspace with a GitHub runner.
type GitHubRunner struct {
	ID             uuid.UUID `json:"id" format:"uuid"`
	PoolID         uuid.UUID `json:"pool_id" format:"uuid"`
	OrganizationID uuid.UUID `json:"organization_id" format:"uuid"`
	WorkspaceID    uuid.UUID `json:"workspace_id" format:"uuid"`
	Name           string    `json:"name"`
	GitHubRunnerID int64     `json:"github_runner_id"`
	State          string    `json:"state"`
	JobRequestID   int64     `json:"job_request_id"`
	JobResult      string    `json:"job_result"`
	LastError      string    `json:"last_error"`
	CreatedAt      time.Time `json:"created_at" format:"date-time"`
	UpdatedAt      time.Time `json:"updated_at" format:"date-time"`
}

// CreateGitHubRunnerPoolRequest configures a polling-based runner pool.
// GitHubToken is write-only and is never delivered to a workspace.
type CreateGitHubRunnerPoolRequest struct {
	Name                  string    `json:"name" validate:"required,workspace_name"`
	TemplateID            uuid.UUID `json:"template_id" format:"uuid"`
	OwnerID               uuid.UUID `json:"owner_id" format:"uuid"`
	GitHubConfigURL       string    `json:"github_config_url"`
	GitHubToken           string    `json:"github_token"`
	RunnerGroupID         int64     `json:"runner_group_id"`
	MaxRunners            int32     `json:"max_runners"`
	StartupTimeoutSeconds int32     `json:"startup_timeout_seconds"`
	JobTimeoutSeconds     int32     `json:"job_timeout_seconds"`
}

// Validate checks the supported scope, capacity, and timeout bounds.
func (r CreateGitHubRunnerPoolRequest) Validate() []ValidationError {
	var errs []ValidationError
	add := func(field, detail string) { errs = append(errs, ValidationError{Field: field, Detail: detail}) }
	u, err := url.Parse(r.GitHubConfigURL)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		add("github_config_url", "Must be an HTTPS github.com organization or repository URL.")
	} else {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) > 2 || parts[0] == "" || parts[0] == "enterprises" || strings.Contains(u.Path, "//") || strings.Contains(u.EscapedPath(), "%") {
			add("github_config_url", "Must identify an organization or repository.")
		}
		for _, part := range parts {
			if part == "." || part == ".." || !regexp.MustCompile(`^[A-Za-z0-9_.-]+$`).MatchString(part) {
				add("github_config_url", "Invalid GitHub scope name.")
				break
			}
		}
	}
	if r.GitHubToken == "" {
		add("github_token", "A GitHub token with runner administration permission is required.")
	}
	if r.TemplateID == uuid.Nil {
		add("template_id", "A runner template is required.")
	}
	if r.RunnerGroupID < 0 {
		add("runner_group_id", "Must be a positive runner group ID.")
	}
	if r.MaxRunners < 1 || r.MaxRunners > 1000 {
		add("max_runners", "Must be between 1 and 1000.")
	}
	if r.StartupTimeoutSeconds != 0 && (r.StartupTimeoutSeconds < 60 || r.StartupTimeoutSeconds > 3600) {
		add("startup_timeout_seconds", "Must be between 60 and 3600.")
	}
	if r.JobTimeoutSeconds != 0 && (r.JobTimeoutSeconds < 60 || r.JobTimeoutSeconds > 86400) {
		add("job_timeout_seconds", "Must be between 60 and 86400.")
	}
	return errs
}

// UpdateGitHubRunnerPoolRequest changes capacity or pauses/drains a pool.
type UpdateGitHubRunnerPoolRequest struct {
	State      string `json:"state,omitempty" enums:"active,paused,draining"`
	MaxRunners int32  `json:"max_runners,omitempty"`
}

func githubRunnerPoolsPath(organization uuid.UUID) string {
	return fmt.Sprintf("/api/v2/organizations/%s/github-runner-pools", organization)
}

// CreateGitHubRunnerPool creates a pool within an organization.
func (c *Client) CreateGitHubRunnerPool(ctx context.Context, organization uuid.UUID, req CreateGitHubRunnerPoolRequest) (GitHubRunnerPool, error) {
	res, err := c.Request(ctx, http.MethodPost, githubRunnerPoolsPath(organization), req)
	if err != nil {
		return GitHubRunnerPool{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return GitHubRunnerPool{}, ReadBodyAsError(res)
	}
	var pool GitHubRunnerPool
	err = ReadBodyAsJSON(res, &pool)
	return pool, err
}

// GitHubRunnerPools lists an organization's runner pools.
func (c *Client) GitHubRunnerPools(ctx context.Context, organization uuid.UUID) ([]GitHubRunnerPool, error) {
	res, err := c.Request(ctx, http.MethodGet, githubRunnerPoolsPath(organization), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var pools []GitHubRunnerPool
	err = ReadBodyAsJSON(res, &pools)
	return pools, err
}

// GitHubRunnerPool fetches a pool and its latest controller status.
func (c *Client) GitHubRunnerPool(ctx context.Context, organization, id uuid.UUID) (GitHubRunnerPool, error) {
	res, err := c.Request(ctx, http.MethodGet, githubRunnerPoolsPath(organization)+"/"+id.String(), nil)
	if err != nil {
		return GitHubRunnerPool{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return GitHubRunnerPool{}, ReadBodyAsError(res)
	}
	var pool GitHubRunnerPool
	err = ReadBodyAsJSON(res, &pool)
	return pool, err
}

// UpdateGitHubRunnerPool changes a pool's capacity or lifecycle state.
func (c *Client) UpdateGitHubRunnerPool(ctx context.Context, organization, id uuid.UUID, req UpdateGitHubRunnerPoolRequest) (GitHubRunnerPool, error) {
	res, err := c.Request(ctx, http.MethodPatch, githubRunnerPoolsPath(organization)+"/"+id.String(), req)
	if err != nil {
		return GitHubRunnerPool{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return GitHubRunnerPool{}, ReadBodyAsError(res)
	}
	var pool GitHubRunnerPool
	err = ReadBodyAsJSON(res, &pool)
	return pool, err
}

// DeleteGitHubRunnerPool drains a pool before removing its scale set.
func (c *Client) DeleteGitHubRunnerPool(ctx context.Context, organization, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, githubRunnerPoolsPath(organization)+"/"+id.String(), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		return ReadBodyAsError(res)
	}
	return nil
}

// GitHubRunners lists the workspace allocations belonging to a pool.
func (c *Client) GitHubRunners(ctx context.Context, organization, id uuid.UUID) ([]GitHubRunner, error) {
	res, err := c.Request(ctx, http.MethodGet, githubRunnerPoolsPath(organization)+"/"+id.String()+"/runners", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var runners []GitHubRunner
	err = ReadBodyAsJSON(res, &runners)
	return runners, err
}
