package codersdk

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/coder/coder/coderd/httpmw"
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
}

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
	ID            uuid.UUID            `json:"id"`
	CreatedAt     time.Time            `json:"created_at"`
	StartedAt     *time.Time           `json:"started_at,omitempty"`
	CompletedAt   *time.Time           `json:"completed_at,omitempty"`
	Error         string               `json:"error,omitempty"`
	Status        ProvisionerJobStatus `json:"status"`
	WorkerID      *uuid.UUID           `json:"worker_id,omitempty"`
	StorageSource string               `json:"storage_source"`
}

type ProvisionerJobLog struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Source    LogSource `json:"log_source"`
	Level     LogLevel  `json:"log_level"`
	Stage     string    `json:"stage"`
	Output    string    `json:"output"`
}

// provisionerJobLogsBefore provides log output that occurred before a time.
// This is abstracted from a specific job type to provide consistency between
// APIs. Logs is the only shared route between jobs.
func (c *Client) provisionerJobLogsBefore(ctx context.Context, path string, before time.Time) ([]ProvisionerJobLog, error) {
	values := url.Values{}
	if !before.IsZero() {
		values["before"] = []string{strconv.FormatInt(before.UTC().UnixMilli(), 10)}
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
func (c *Client) provisionerJobLogsAfter(ctx context.Context, path string, after time.Time) (<-chan ProvisionerJobLog, error) {
	afterQuery := ""
	if !after.IsZero() {
		afterQuery = fmt.Sprintf("&after=%d", after.UTC().UnixMilli())
	}
	followURL, err := c.URL.Parse(fmt.Sprintf("%s?follow%s", path, afterQuery))
	if err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(followURL, []*http.Cookie{{
		Name:  httpmw.SessionTokenKey,
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
		return nil, readBodyAsError(res)
	}
	logs := make(chan ProvisionerJobLog)
	decoder := json.NewDecoder(websocket.NetConn(ctx, conn, websocket.MessageText))
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
