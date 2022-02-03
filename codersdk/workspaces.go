package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/coder/coder/coderd"
)

// Workspaces returns all workspaces the authenticated session has access to.
// If owner is specified, all workspaces for an organization will be returned.
// If owner is empty, all workspaces the caller has access to will be returned.
func (c *Client) WorkspacesByUser(ctx context.Context, user string) ([]coderd.Workspace, error) {
	route := "/api/v2/workspaces"
	if user != "" {
		route += fmt.Sprintf("/%s", user)
	}
	res, err := c.request(ctx, http.MethodGet, route, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var workspaces []coderd.Workspace
	return workspaces, json.NewDecoder(res.Body).Decode(&workspaces)
}

// WorkspacesByProject lists all workspaces for a specific project.
func (c *Client) WorkspacesByProject(ctx context.Context, organization, project string) ([]coderd.Workspace, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projects/%s/%s/workspaces", organization, project), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var workspaces []coderd.Workspace
	return workspaces, json.NewDecoder(res.Body).Decode(&workspaces)
}

// Workspace returns a single workspace by owner and name.
func (c *Client) Workspace(ctx context.Context, owner, name string) (coderd.Workspace, error) {
	if owner == "" {
		owner = "me"
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/%s", owner, name), nil)
	if err != nil {
		return coderd.Workspace{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.Workspace{}, readBodyAsError(res)
	}
	var workspace coderd.Workspace
	return workspace, json.NewDecoder(res.Body).Decode(&workspace)
}

// ListWorkspaceHistory returns historical data for workspace builds.
func (c *Client) ListWorkspaceHistory(ctx context.Context, owner, workspace string) ([]coderd.WorkspaceHistory, error) {
	if owner == "" {
		owner = "me"
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/%s/history", owner, workspace), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var workspaceHistory []coderd.WorkspaceHistory
	return workspaceHistory, json.NewDecoder(res.Body).Decode(&workspaceHistory)
}

// WorkspaceHistory returns a single workspace history for a workspace.
// If history is "", the latest version is returned.
func (c *Client) WorkspaceHistory(ctx context.Context, owner, workspace, history string) (coderd.WorkspaceHistory, error) {
	if owner == "" {
		owner = "me"
	}
	if history == "" {
		history = "latest"
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/%s/history/%s", owner, workspace, history), nil)
	if err != nil {
		return coderd.WorkspaceHistory{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.WorkspaceHistory{}, readBodyAsError(res)
	}
	var workspaceHistory coderd.WorkspaceHistory
	return workspaceHistory, json.NewDecoder(res.Body).Decode(&workspaceHistory)
}

// CreateWorkspace creates a new workspace for the project specified.
func (c *Client) CreateWorkspace(ctx context.Context, user string, request coderd.CreateWorkspaceRequest) (coderd.Workspace, error) {
	if user == "" {
		user = "me"
	}
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/workspaces/%s", user), request)
	if err != nil {
		return coderd.Workspace{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return coderd.Workspace{}, readBodyAsError(res)
	}
	var workspace coderd.Workspace
	return workspace, json.NewDecoder(res.Body).Decode(&workspace)
}

// CreateWorkspaceHistory queues a new build to occur for a workspace.
func (c *Client) CreateWorkspaceHistory(ctx context.Context, owner, workspace string, request coderd.CreateWorkspaceHistoryRequest) (coderd.WorkspaceHistory, error) {
	if owner == "" {
		owner = "me"
	}
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/workspaces/%s/%s/history", owner, workspace), request)
	if err != nil {
		return coderd.WorkspaceHistory{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return coderd.WorkspaceHistory{}, readBodyAsError(res)
	}
	var workspaceHistory coderd.WorkspaceHistory
	return workspaceHistory, json.NewDecoder(res.Body).Decode(&workspaceHistory)
}

// WorkspaceHistoryLogs returns all logs for workspace history.
// To stream logs, use the FollowWorkspaceHistoryLogs function.
func (c *Client) WorkspaceHistoryLogs(ctx context.Context, owner, workspace, history string) ([]coderd.WorkspaceHistoryLog, error) {
	return c.WorkspaceHistoryLogsBetween(ctx, owner, workspace, history, time.Time{}, time.Time{})
}

// WorkspaceHistoryLogsBetween returns logs between a specific time.
func (c *Client) WorkspaceHistoryLogsBetween(ctx context.Context, owner, workspace, history string, after, before time.Time) ([]coderd.WorkspaceHistoryLog, error) {
	if owner == "" {
		owner = "me"
	}
	values := url.Values{}
	if !after.IsZero() {
		values["after"] = []string{strconv.FormatInt(after.UTC().UnixMilli(), 10)}
	}
	if !before.IsZero() {
		values["before"] = []string{strconv.FormatInt(before.UTC().UnixMilli(), 10)}
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/%s/history/%s/logs?%s", owner, workspace, history, values.Encode()), nil)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		return nil, readBodyAsError(res)
	}

	var logs []coderd.WorkspaceHistoryLog
	return logs, json.NewDecoder(res.Body).Decode(&logs)
}

// FollowWorkspaceHistoryLogsAfter returns a stream of workspace history logs.
// The channel will close when the workspace history job is no longer active.
func (c *Client) FollowWorkspaceHistoryLogsAfter(ctx context.Context, owner, workspace, history string, after time.Time) (<-chan coderd.WorkspaceHistoryLog, error) {
	afterQuery := ""
	if !after.IsZero() {
		afterQuery = fmt.Sprintf("&after=%d", after.UTC().UnixMilli())
	}
	if owner == "" {
		owner = "me"
	}

	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/%s/history/%s/logs?follow%s", owner, workspace, history, afterQuery), nil)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		return nil, readBodyAsError(res)
	}

	logs := make(chan coderd.WorkspaceHistoryLog)
	decoder := json.NewDecoder(res.Body)
	go func() {
		defer close(logs)
		var log coderd.WorkspaceHistoryLog
		for {
			err = decoder.Decode(&log)
			if err != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case logs <- log:
			}
		}
	}()
	return logs, nil
}
