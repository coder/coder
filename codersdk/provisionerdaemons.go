package codersdk

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/coder/coder/provisionerd/proto"
	"github.com/coder/coder/provisionersdk"
)

type LogSource string

const (
	LogSourceProvisionerDaemon LogSource = "provisioner_daemon"
	LogSourceProvisioner       LogSource = "provisioner"
)

type LogLevel string

const (
	LogLevelTrace LogLevel = "trace"
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

type ProvisionerDaemon struct {
	ID           uuid.UUID         `json:"id"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    sql.NullTime      `json:"updated_at"`
	Name         string            `json:"name"`
	Provisioners []ProvisionerType `json:"provisioners"`
	AuthToken    *uuid.UUID        `json:"auth_token"`
}

// ProvisionerJobStatus represents the at-time state of a job.
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
	CanceledAt  *time.Time           `json:"canceled_at,omitempty"`
	Error       string               `json:"error,omitempty"`
	Status      ProvisionerJobStatus `json:"status"`
	WorkerID    *uuid.UUID           `json:"worker_id,omitempty"`
	FileID      uuid.UUID            `json:"file_id"`
}

type ProvisionerJobLog struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Source    LogSource `json:"log_source"`
	Level     LogLevel  `json:"log_level"`
	Stage     string    `json:"stage"`
	Output    string    `json:"output"`
}

// provisionerJobLogsBefore provides log output that occurred before a time.
// This is abstracted from a specific job type to provide consistency between
// APIs. Logs is the only shared route between jobs.
func (c *Client) provisionerJobLogsBefore(ctx context.Context, path string, before int64) ([]ProvisionerJobLog, error) {
	values := url.Values{}
	if before != 0 {
		values["before"] = []string{strconv.FormatInt(before, 10)}
	}
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("%s?%s", path, values.Encode()), nil)
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
func (c *Client) provisionerJobLogsAfter(ctx context.Context, path string, after int64) (<-chan ProvisionerJobLog, io.Closer, error) {
	afterQuery := ""
	if after != 0 {
		afterQuery = fmt.Sprintf("&after=%d", after)
	}
	followURL, err := c.URL.Parse(fmt.Sprintf("%s?follow%s", path, afterQuery))
	if err != nil {
		return nil, nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(followURL, []*http.Cookie{{
		Name:  SessionTokenKey,
		Value: c.SessionToken,
	}})
	httpClient := &http.Client{
		Jar: jar,
	}
	conn, res, err := websocket.Dial(ctx, followURL.String(), &websocket.DialOptions{
		HTTPClient:      httpClient,
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		if res == nil {
			return nil, nil, err
		}
		return nil, nil, readBodyAsError(res)
	}
	logs := make(chan ProvisionerJobLog)
	decoder := json.NewDecoder(websocket.NetConn(ctx, conn, websocket.MessageText))
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		defer close(logs)
		defer conn.Close(websocket.StatusGoingAway, "")
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
	return logs, closeFunc(func() error {
		_ = conn.Close(websocket.StatusNormalClosure, "")
		<-closed
		return nil
	}), nil
}

type CreateProvisionerDaemonRequest struct {
	Name string `json:"name" validate:"required"`
}

// ListenProvisionerDaemon returns the gRPC service for a provisioner daemon implementation.
func (c *Client) ListenProvisionerDaemon(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
	serverURL, err := c.URL.Parse("/api/v2/provisionerdaemons/me/listen")
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(serverURL, []*http.Cookie{{
		Name:  SessionTokenKey,
		Value: c.SessionToken,
	}})
	httpClient := &http.Client{
		Jar: jar,
	}
	conn, res, err := websocket.Dial(ctx, serverURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
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

// CreateProvisionerDaemon creates a new standalone provisioner instance and generates an auth token.
func (c *Client) CreateProvisionerDaemon(ctx context.Context, req CreateProvisionerDaemonRequest) (ProvisionerDaemon, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/provisionerdaemons/", req)
	if err != nil {
		return ProvisionerDaemon{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return ProvisionerDaemon{}, readBodyAsError(res)
	}

	var provisionerDaemon ProvisionerDaemon
	return provisionerDaemon, json.NewDecoder(res.Body).Decode(&provisionerDaemon)
}
