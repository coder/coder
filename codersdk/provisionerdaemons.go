package codersdk

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionerd/runner"
	"github.com/coder/coder/v2/provisionersdk"
)

type LogSource string

type LogLevel string

const (
	LogSourceProvisionerDaemon LogSource = "provisioner_daemon"
	LogSourceProvisioner       LogSource = "provisioner"

	LogLevelTrace LogLevel = "trace"
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

type ProvisionerDaemon struct {
	ID           uuid.UUID         `json:"id" format:"uuid"`
	CreatedAt    time.Time         `json:"created_at" format:"date-time"`
	UpdatedAt    sql.NullTime      `json:"updated_at" format:"date-time"`
	LastSeenAt   NullTime          `json:"last_seen_at,omitempty" format:"date-time"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Provisioners []ProvisionerType `json:"provisioners"`
	Tags         map[string]string `json:"tags"`
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
	ProvisionerJobUnknown   ProvisionerJobStatus = "unknown"
)

// JobErrorCode defines the error code returned by job runner.
type JobErrorCode string

const (
	RequiredTemplateVariables JobErrorCode = "REQUIRED_TEMPLATE_VARIABLES"
)

// JobIsMissingParameterErrorCode returns whether the error is a missing parameter error.
// This can indicate to consumers that they should check parameters.
func JobIsMissingParameterErrorCode(code JobErrorCode) bool {
	return string(code) == runner.MissingParameterErrorCode
}

// ProvisionerJob describes the job executed by the provisioning daemon.
type ProvisionerJob struct {
	ID            uuid.UUID            `json:"id" format:"uuid"`
	CreatedAt     time.Time            `json:"created_at" format:"date-time"`
	StartedAt     *time.Time           `json:"started_at,omitempty" format:"date-time"`
	CompletedAt   *time.Time           `json:"completed_at,omitempty" format:"date-time"`
	CanceledAt    *time.Time           `json:"canceled_at,omitempty" format:"date-time"`
	Error         string               `json:"error,omitempty"`
	ErrorCode     JobErrorCode         `json:"error_code,omitempty" enums:"REQUIRED_TEMPLATE_VARIABLES"`
	Status        ProvisionerJobStatus `json:"status" enums:"pending,running,succeeded,canceling,canceled,failed"`
	WorkerID      *uuid.UUID           `json:"worker_id,omitempty" format:"uuid"`
	FileID        uuid.UUID            `json:"file_id" format:"uuid"`
	Tags          map[string]string    `json:"tags"`
	QueuePosition int                  `json:"queue_position"`
	QueueSize     int                  `json:"queue_size"`
}

// ProvisionerJobLog represents the provisioner log entry annotated with source and level.
type ProvisionerJobLog struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at" format:"date-time"`
	Source    LogSource `json:"log_source"`
	Level     LogLevel  `json:"log_level" enums:"trace,debug,info,warn,error"`
	Stage     string    `json:"stage"`
	Output    string    `json:"output"`
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
		Name:  SessionTokenCookie,
		Value: c.SessionToken(),
	}})
	httpClient := &http.Client{
		Jar:       jar,
		Transport: c.HTTPClient.Transport,
	}
	conn, res, err := websocket.Dial(ctx, followURL.String(), &websocket.DialOptions{
		HTTPClient:      httpClient,
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		if res == nil {
			return nil, nil, err
		}
		return nil, nil, ReadBodyAsError(res)
	}
	logs := make(chan ProvisionerJobLog)
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		defer close(logs)
		defer conn.Close(websocket.StatusGoingAway, "")
		var log ProvisionerJobLog
		for {
			msgType, msg, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if msgType != websocket.MessageText {
				return
			}
			err = json.Unmarshal(msg, &log)
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
		<-closed
		return nil
	}), nil
}

// ServeProvisionerDaemonRequest are the parameters to call ServeProvisionerDaemon with
// @typescript-ignore ServeProvisionerDaemonRequest
type ServeProvisionerDaemonRequest struct {
	// ID is a unique ID for a provisioner daemon.
	ID uuid.UUID `json:"id" format:"uuid"`
	// Organization is the organization for the URL.  At present provisioner daemons ARE NOT scoped to organizations
	// and so the organization ID is optional.
	Organization uuid.UUID `json:"organization" format:"uuid"`
	// Provisioners is a list of provisioner types hosted by the provisioner daemon
	Provisioners []ProvisionerType `json:"provisioners"`
	// Tags is a map of key-value pairs that tag the jobs this provisioner daemon can handle
	Tags map[string]string `json:"tags"`
	// PreSharedKey is an authentication key to use on the API instead of the normal session token from the client.
	PreSharedKey string `json:"pre_shared_key"`
}

// ServeProvisionerDaemon returns the gRPC service for a provisioner daemon
// implementation. The context is during dial, not during the lifetime of the
// client. Client should be closed after use.
func (c *Client) ServeProvisionerDaemon(ctx context.Context, req ServeProvisionerDaemonRequest) (proto.DRPCProvisionerDaemonClient, error) {
	serverURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/organizations/%s/provisionerdaemons/serve", req.Organization))
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	query := serverURL.Query()
	query.Add("id", req.ID.String())
	for _, provisioner := range req.Provisioners {
		query.Add("provisioner", string(provisioner))
	}
	for key, value := range req.Tags {
		query.Add("tag", fmt.Sprintf("%s=%s", key, value))
	}
	serverURL.RawQuery = query.Encode()
	httpClient := &http.Client{
		Transport: c.HTTPClient.Transport,
	}
	headers := http.Header{}

	if req.PreSharedKey == "" {
		// use session token if we don't have a PSK.
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, xerrors.Errorf("create cookie jar: %w", err)
		}
		jar.SetCookies(serverURL, []*http.Cookie{{
			Name:  SessionTokenCookie,
			Value: c.SessionToken(),
		}})
		httpClient.Jar = jar
	} else {
		headers.Set(ProvisionerDaemonPSK, req.PreSharedKey)
	}

	conn, res, err := websocket.Dial(ctx, serverURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
		// Need to disable compression to avoid a data-race.
		CompressionMode: websocket.CompressionDisabled,
		HTTPHeader:      headers,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, ReadBodyAsError(res)
	}
	// Align with the frame size of yamux.
	conn.SetReadLimit(256 * 1024)

	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	// Use background context because caller should close the client.
	_, wsNetConn := websocketNetConn(context.Background(), conn, websocket.MessageBinary)
	session, err := yamux.Client(wsNetConn, config)
	if err != nil {
		_ = conn.Close(websocket.StatusGoingAway, "")
		_ = wsNetConn.Close()
		return nil, xerrors.Errorf("multiplex client: %w", err)
	}
	return proto.NewDRPCProvisionerDaemonClient(provisionersdk.MultiplexedConn(session)), nil
}

// wsNetConn wraps net.Conn created by websocket.NetConn(). Cancel func
// is called if a read or write error is encountered.
// @typescript-ignore wsNetConn
type wsNetConn struct {
	cancel context.CancelFunc
	net.Conn
}

func (c *wsNetConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if err != nil {
		c.cancel()
	}
	return n, err
}

func (c *wsNetConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	if err != nil {
		c.cancel()
	}
	return n, err
}

func (c *wsNetConn) Close() error {
	defer c.cancel()
	return c.Conn.Close()
}

// websocketNetConn wraps websocket.NetConn and returns a context that
// is tied to the parent context and the lifetime of the conn. Any error
// during read or write will cancel the context, but not close the
// conn. Close should be called to release context resources.
func websocketNetConn(ctx context.Context, conn *websocket.Conn, msgType websocket.MessageType) (context.Context, net.Conn) {
	ctx, cancel := context.WithCancel(ctx)
	nc := websocket.NetConn(ctx, conn, msgType)
	return ctx, &wsNetConn{
		cancel: cancel,
		Conn:   nc,
	}
}
