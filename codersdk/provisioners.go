package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/provisionerd/proto"
	"github.com/coder/coder/provisionersdk"
)

func (c *Client) ProvisionerDaemons(ctx context.Context) ([]coderd.ProvisionerDaemon, error) {
	res, err := c.request(ctx, http.MethodGet, "/api/v2/provisioners/daemons", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var daemons []coderd.ProvisionerDaemon
	return daemons, json.NewDecoder(res.Body).Decode(&daemons)
}

// ProvisionerDaemonClient returns the gRPC service for a provisioner daemon implementation.
func (c *Client) ProvisionerDaemonClient(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
	serverURL, err := c.URL.Parse("/api/v2/provisioners/daemons/serve")
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	conn, res, err := websocket.Dial(ctx, serverURL.String(), &websocket.DialOptions{
		HTTPClient: c.httpClient,
		// Need to disable compression to avoid a data-race.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, readBodyAsError(res)
	}
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Client(websocket.NetConn(ctx, conn, websocket.MessageBinary), config)
	if err != nil {
		return nil, xerrors.Errorf("multiplex client: %w", err)
	}
	return proto.NewDRPCProvisionerDaemonClient(provisionersdk.Conn(session)), nil
}

// CreateProjectVersionImportProvisionerJob creates a job for importing
// the provided project version.
func (c *Client) CreateProjectVersionImportProvisionerJob(ctx context.Context, organization string, req coderd.CreateProjectImportJobRequest) (coderd.ProvisionerJob, error) {
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/provisioners/jobs/%s/import", organization), req)
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

// ProvisionerJob returns a job by ID.
func (c *Client) ProvisionerJob(ctx context.Context, organization string, job uuid.UUID) (coderd.ProvisionerJob, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/provisioners/jobs/%s/%s", organization, job), nil)
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

// ProvisionerJobLogs returns all logs for workspace history.
// To stream logs, use the FollowProvisionerJobLogs function.
func (c *Client) ProvisionerJobLogs(ctx context.Context, organization string, jobID uuid.UUID) ([]coderd.ProvisionerJobLog, error) {
	return c.ProvisionerJobLogsBetween(ctx, organization, jobID, time.Time{}, time.Time{})
}

// ProvisionerJobLogsBetween returns logs between a specific time.
func (c *Client) ProvisionerJobLogsBetween(ctx context.Context, organization string, jobID uuid.UUID, after, before time.Time) ([]coderd.ProvisionerJobLog, error) {
	values := url.Values{}
	if !after.IsZero() {
		values["after"] = []string{strconv.FormatInt(after.UTC().UnixMilli(), 10)}
	}
	if !before.IsZero() {
		values["before"] = []string{strconv.FormatInt(before.UTC().UnixMilli(), 10)}
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/provisioners/jobs/%s/%s/logs?%s", organization, jobID, values.Encode()), nil)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		return nil, readBodyAsError(res)
	}

	var logs []coderd.ProvisionerJobLog
	return logs, json.NewDecoder(res.Body).Decode(&logs)
}

// FollowProvisionerJobLogsAfter returns a stream of workspace history logs.
// The channel will close when the workspace history job is no longer active.
func (c *Client) FollowProvisionerJobLogsAfter(ctx context.Context, organization string, jobID uuid.UUID, after time.Time) (<-chan coderd.ProvisionerJobLog, error) {
	afterQuery := ""
	if !after.IsZero() {
		afterQuery = fmt.Sprintf("&after=%d", after.UTC().UnixMilli())
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/provisioners/jobs/%s/%s/logs?follow%s", organization, jobID, afterQuery), nil)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		return nil, readBodyAsError(res)
	}

	logs := make(chan coderd.ProvisionerJobLog)
	decoder := json.NewDecoder(res.Body)
	go func() {
		defer close(logs)
		var log coderd.ProvisionerJobLog
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
