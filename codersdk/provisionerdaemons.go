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

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/provisionerd/proto"
	"github.com/coder/coder/provisionersdk"
)

type ProvisionerDaemon database.ProvisionerDaemon

// ProvisionerJobStaus represents the at-time state of a job.
type ProvisionerJobStatus string

// Active returns whether the job is still active or not.
// It returns true if canceling as well, since the job isn't
// in an entirely inactive state yet.
func (p ProvisionerJobStatus) Active() bool {
	return p == ProvisionerJobPending ||
		p == ProvisionerJobRunning ||
		p == ProvisionerJobCanceling
}

const (
	ProvisionerJobPending   ProvisionerJobStatus = "pending"
	ProvisionerJobRunning   ProvisionerJobStatus = "running"
	ProvisionerJobSucceeded ProvisionerJobStatus = "succeeded"
	ProvisionerJobCanceling ProvisionerJobStatus = "canceling"
	ProvisionerJobCanceled  ProvisionerJobStatus = "canceled"
	ProvisionerJobFailed    ProvisionerJobStatus = "failed"
)

type ProvisionerJob struct {
	ID          uuid.UUID            `json:"id"`
	CreatedAt   time.Time            `json:"created_at"`
	StartedAt   *time.Time           `json:"started_at,omitempty"`
	CompletedAt *time.Time           `json:"completed_at,omitempty"`
	Error       string               `json:"error,omitempty"`
	Status      ProvisionerJobStatus `json:"status"`
	WorkerID    *uuid.UUID           `json:"worker_id,omitempty"`
}

type ProvisionerJobLog struct {
	ID        uuid.UUID          `json:"id"`
	CreatedAt time.Time          `json:"created_at"`
	Source    database.LogSource `json:"log_source"`
	Level     database.LogLevel  `json:"log_level"`
	Stage     string             `json:"stage"`
	Output    string             `json:"output"`
}

// ListenProvisionerDaemon returns the gRPC service for a provisioner daemon implementation.
func (c *Client) ListenProvisionerDaemon(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
	serverURL, err := c.URL.Parse("/api/v2/provisionerdaemons/me/listen")
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	conn, res, err := websocket.Dial(ctx, serverURL.String(), &websocket.DialOptions{
		HTTPClient: c.HTTPClient,
		// Need to disable compression to avoid a data-race.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, readBodyAsError(res)
	}
	// Align with the frame size of yamux.
	conn.SetReadLimit(256 * 1024)

	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Client(websocket.NetConn(ctx, conn, websocket.MessageBinary), config)
	if err != nil {
		return nil, xerrors.Errorf("multiplex client: %w", err)
	}
	return proto.NewDRPCProvisionerDaemonClient(provisionersdk.Conn(session)), nil
}

// provisionerJobLogsBefore provides log output that occurred before a time.
// This is abstracted from a specific job type to provide consistency between
// APIs. Logs is the only shared route between jobs.
func (c *Client) provisionerJobLogsBefore(ctx context.Context, path string, before time.Time) ([]ProvisionerJobLog, error) {
	values := url.Values{}
	if !before.IsZero() {
		values["before"] = []string{strconv.FormatInt(before.UTC().UnixMilli(), 10)}
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("%s?%s", path, values.Encode()), nil)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		return nil, readBodyAsError(res)
	}

	var logs []ProvisionerJobLog
	return logs, json.NewDecoder(res.Body).Decode(&logs)
}

// provisionerJobLogsAfter streams logs that occurred after a specific time.
func (c *Client) provisionerJobLogsAfter(ctx context.Context, path string, after time.Time) (<-chan ProvisionerJobLog, error) {
	afterQuery := ""
	if !after.IsZero() {
		afterQuery = fmt.Sprintf("&after=%d", after.UTC().UnixMilli())
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("%s?follow%s", path, afterQuery), nil)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		return nil, readBodyAsError(res)
	}

	logs := make(chan ProvisionerJobLog)
	decoder := json.NewDecoder(res.Body)
	go func() {
		defer close(logs)
		var log ProvisionerJobLog
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
