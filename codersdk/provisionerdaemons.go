package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk/drpc"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionerd/runner"
	"github.com/coder/websocket"
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

// ProvisionerDaemonStatus represents the status of a provisioner daemon.
type ProvisionerDaemonStatus string

// ProvisionerDaemonStatus enums.
const (
	ProvisionerDaemonOffline ProvisionerDaemonStatus = "offline"
	ProvisionerDaemonIdle    ProvisionerDaemonStatus = "idle"
	ProvisionerDaemonBusy    ProvisionerDaemonStatus = "busy"
)

type ProvisionerDaemon struct {
	ID             uuid.UUID         `json:"id" format:"uuid" table:"id"`
	OrganizationID uuid.UUID         `json:"organization_id" format:"uuid" table:"organization id"`
	KeyID          uuid.UUID         `json:"key_id" format:"uuid" table:"-"`
	CreatedAt      time.Time         `json:"created_at" format:"date-time" table:"created at"`
	LastSeenAt     NullTime          `json:"last_seen_at,omitempty" format:"date-time" table:"last seen at"`
	Name           string            `json:"name" table:"name,default_sort"`
	Version        string            `json:"version" table:"version"`
	APIVersion     string            `json:"api_version" table:"api version"`
	Provisioners   []ProvisionerType `json:"provisioners" table:"-"`
	Tags           map[string]string `json:"tags" table:"tags"`

	// Optional fields.
	KeyName     *string                  `json:"key_name" table:"key name"`
	Status      *ProvisionerDaemonStatus `json:"status" enums:"offline,idle,busy" table:"status"`
	CurrentJob  *ProvisionerDaemonJob    `json:"current_job" table:"current job,recursive"`
	PreviousJob *ProvisionerDaemonJob    `json:"previous_job" table:"previous job,recursive"`
}

type ProvisionerDaemonJob struct {
	ID     uuid.UUID            `json:"id" format:"uuid" table:"id"`
	Status ProvisionerJobStatus `json:"status" enums:"pending,running,succeeded,canceling,canceled,failed" table:"status"`
}

// MatchedProvisioners represents the number of provisioner daemons
// available to take a job at a specific point in time.
// Introduced in Coder version 2.18.0.
type MatchedProvisioners struct {
	// Count is the number of provisioner daemons that matched the given
	// tags. If the count is 0, it means no provisioner daemons matched the
	// requested tags.
	Count int `json:"count"`
	// Available is the number of provisioner daemons that are available to
	// take jobs. This may be less than the count if some provisioners are
	// busy or have been stopped.
	Available int `json:"available"`
	// MostRecentlySeen is the most recently seen time of the set of matched
	// provisioners. If no provisioners matched, this field will be null.
	MostRecentlySeen NullTime `json:"most_recently_seen,omitempty" format:"date-time"`
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

func ProvisionerJobStatusEnums() []ProvisionerJobStatus {
	return []ProvisionerJobStatus{
		ProvisionerJobPending,
		ProvisionerJobRunning,
		ProvisionerJobSucceeded,
		ProvisionerJobCanceling,
		ProvisionerJobCanceled,
		ProvisionerJobFailed,
		ProvisionerJobUnknown,
	}
}

// ProvisionerJobInput represents the input for the job.
type ProvisionerJobInput struct {
	TemplateVersionID *uuid.UUID `json:"template_version_id,omitempty" format:"uuid" table:"template version id"`
	WorkspaceBuildID  *uuid.UUID `json:"workspace_build_id,omitempty" format:"uuid" table:"workspace build id"`
	Error             string     `json:"error,omitempty" table:"-"`
}

// ProvisionerJobMetadata contains metadata for the job.
type ProvisionerJobMetadata struct {
	TemplateVersionName string     `json:"template_version_name" table:"template version name"`
	TemplateID          uuid.UUID  `json:"template_id" format:"uuid" table:"template id"`
	TemplateName        string     `json:"template_name" table:"template name"`
	TemplateDisplayName string     `json:"template_display_name" table:"template display name"`
	WorkspaceID         *uuid.UUID `json:"workspace_id,omitempty" format:"uuid" table:"workspace id"`
	WorkspaceName       string     `json:"workspace_name,omitempty" table:"workspace name"`
}

// ProvisionerJobType represents the type of job.
type ProvisionerJobType string

const (
	ProvisionerJobTypeTemplateVersionImport ProvisionerJobType = "template_version_import"
	ProvisionerJobTypeWorkspaceBuild        ProvisionerJobType = "workspace_build"
	ProvisionerJobTypeTemplateVersionDryRun ProvisionerJobType = "template_version_dry_run"
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
	ID               uuid.UUID               `json:"id" format:"uuid" table:"id"`
	CreatedAt        time.Time               `json:"created_at" format:"date-time" table:"created at"`
	StartedAt        *time.Time              `json:"started_at,omitempty" format:"date-time" table:"started at"`
	CompletedAt      *time.Time              `json:"completed_at,omitempty" format:"date-time" table:"completed at"`
	CanceledAt       *time.Time              `json:"canceled_at,omitempty" format:"date-time" table:"canceled at"`
	Error            string                  `json:"error,omitempty" table:"error"`
	ErrorCode        JobErrorCode            `json:"error_code,omitempty" enums:"REQUIRED_TEMPLATE_VARIABLES" table:"error code"`
	Status           ProvisionerJobStatus    `json:"status" enums:"pending,running,succeeded,canceling,canceled,failed" table:"status"`
	WorkerID         *uuid.UUID              `json:"worker_id,omitempty" format:"uuid" table:"worker id"`
	FileID           uuid.UUID               `json:"file_id" format:"uuid" table:"file id"`
	Tags             map[string]string       `json:"tags" table:"tags"`
	QueuePosition    int                     `json:"queue_position" table:"queue position"`
	QueueSize        int                     `json:"queue_size" table:"queue size"`
	OrganizationID   uuid.UUID               `json:"organization_id" format:"uuid" table:"organization id"`
	Input            ProvisionerJobInput     `json:"input" table:"input,recursive_inline"`
	Type             ProvisionerJobType      `json:"type" table:"type"`
	AvailableWorkers []uuid.UUID             `json:"available_workers,omitempty" format:"uuid" table:"available workers"`
	Metadata         *ProvisionerJobMetadata `json:"metadata,omitempty" table:"metadata,recursive_inline"`
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
	d := wsjson.NewDecoder[ProvisionerJobLog](conn, websocket.MessageText, c.logger)
	return d.Chan(), d, nil
}

// ServeProvisionerDaemonRequest are the parameters to call ServeProvisionerDaemon with
// @typescript-ignore ServeProvisionerDaemonRequest
type ServeProvisionerDaemonRequest struct {
	// ID is a unique ID for a provisioner daemon.
	ID uuid.UUID `json:"id" format:"uuid"`
	// Name is the human-readable unique identifier for the daemon.
	Name string `json:"name" example:"my-cool-provisioner-daemon"`
	// Organization is the organization for the URL. If no orgID is provided,
	// then it is assumed to use the default organization.
	Organization uuid.UUID `json:"organization" format:"uuid"`
	// Provisioners is a list of provisioner types hosted by the provisioner daemon
	Provisioners []ProvisionerType `json:"provisioners"`
	// Tags is a map of key-value pairs that tag the jobs this provisioner daemon can handle
	Tags map[string]string `json:"tags"`
	// PreSharedKey is an authentication key to use on the API instead of the normal session token from the client.
	PreSharedKey string `json:"pre_shared_key"`
	// ProvisionerKey is an authentication key to use on the API instead of the normal session token from the client.
	ProvisionerKey string `json:"provisioner_key"`
}

// ServeProvisionerDaemon returns the gRPC service for a provisioner daemon
// implementation. The context is during dial, not during the lifetime of the
// client. Client should be closed after use.
func (c *Client) ServeProvisionerDaemon(ctx context.Context, req ServeProvisionerDaemonRequest) (proto.DRPCProvisionerDaemonClient, error) {
	orgParam := req.Organization.String()
	if req.Organization == uuid.Nil {
		orgParam = DefaultOrganization
	}

	serverURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/organizations/%s/provisionerdaemons/serve", orgParam))
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	query := serverURL.Query()
	query.Add("version", proto.CurrentVersion.String())
	query.Add("id", req.ID.String())
	query.Add("name", req.Name)
	query.Add("version", proto.CurrentVersion.String())

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

	headers.Set(BuildVersionHeader, buildinfo.Version())

	if req.ProvisionerKey != "" {
		headers.Set(ProvisionerDaemonKey, req.ProvisionerKey)
	}
	if req.PreSharedKey != "" {
		headers.Set(ProvisionerDaemonPSK, req.PreSharedKey)
	}
	if req.ProvisionerKey == "" && req.PreSharedKey == "" {
		// use session token if we don't have a PSK or provisioner key.
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, xerrors.Errorf("create cookie jar: %w", err)
		}
		jar.SetCookies(serverURL, []*http.Cookie{{
			Name:  SessionTokenCookie,
			Value: c.SessionToken(),
		}})
		httpClient.Jar = jar
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
	_, wsNetConn := WebsocketNetConn(context.Background(), conn, websocket.MessageBinary)
	session, err := yamux.Client(wsNetConn, config)
	if err != nil {
		_ = conn.Close(websocket.StatusGoingAway, "")
		_ = wsNetConn.Close()
		return nil, xerrors.Errorf("multiplex client: %w", err)
	}
	return proto.NewDRPCProvisionerDaemonClient(drpc.MultiplexedConn(session)), nil
}

type ProvisionerKeyTags map[string]string

func (p ProvisionerKeyTags) String() string {
	keys := maps.Keys(p)
	slices.Sort(keys)
	tags := []string{}
	for _, key := range keys {
		tags = append(tags, fmt.Sprintf("%s=%s", key, p[key]))
	}
	return strings.Join(tags, " ")
}

type ProvisionerKey struct {
	ID             uuid.UUID          `json:"id" table:"-" format:"uuid"`
	CreatedAt      time.Time          `json:"created_at" table:"created at" format:"date-time"`
	OrganizationID uuid.UUID          `json:"organization" table:"-" format:"uuid"`
	Name           string             `json:"name" table:"name,default_sort"`
	Tags           ProvisionerKeyTags `json:"tags" table:"tags"`
	// HashedSecret - never include the access token in the API response
}

type ProvisionerKeyDaemons struct {
	Key     ProvisionerKey      `json:"key"`
	Daemons []ProvisionerDaemon `json:"daemons"`
}

const (
	ProvisionerKeyIDBuiltIn  = "00000000-0000-0000-0000-000000000001"
	ProvisionerKeyIDUserAuth = "00000000-0000-0000-0000-000000000002"
	ProvisionerKeyIDPSK      = "00000000-0000-0000-0000-000000000003"
)

var (
	ProvisionerKeyUUIDBuiltIn  = uuid.MustParse(ProvisionerKeyIDBuiltIn)
	ProvisionerKeyUUIDUserAuth = uuid.MustParse(ProvisionerKeyIDUserAuth)
	ProvisionerKeyUUIDPSK      = uuid.MustParse(ProvisionerKeyIDPSK)
)

const (
	ProvisionerKeyNameBuiltIn  = "built-in"
	ProvisionerKeyNameUserAuth = "user-auth"
	ProvisionerKeyNamePSK      = "psk"
)

func ReservedProvisionerKeyNames() []string {
	return []string{
		ProvisionerKeyNameBuiltIn,
		ProvisionerKeyNameUserAuth,
		ProvisionerKeyNamePSK,
	}
}

type CreateProvisionerKeyRequest struct {
	Name string            `json:"name"`
	Tags map[string]string `json:"tags"`
}

type CreateProvisionerKeyResponse struct {
	Key string `json:"key"`
}

// CreateProvisionerKey creates a new provisioner key for an organization.
func (c *Client) CreateProvisionerKey(ctx context.Context, organizationID uuid.UUID, req CreateProvisionerKeyRequest) (CreateProvisionerKeyResponse, error) {
	res, err := c.Request(ctx, http.MethodPost,
		fmt.Sprintf("/api/v2/organizations/%s/provisionerkeys", organizationID.String()),
		req,
	)
	if err != nil {
		return CreateProvisionerKeyResponse{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return CreateProvisionerKeyResponse{}, ReadBodyAsError(res)
	}
	var resp CreateProvisionerKeyResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// ListProvisionerKeys lists all provisioner keys for an organization.
func (c *Client) ListProvisionerKeys(ctx context.Context, organizationID uuid.UUID) ([]ProvisionerKey, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/provisionerkeys", organizationID.String()),
		nil,
	)
	if err != nil {
		return nil, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var resp []ProvisionerKey
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// GetProvisionerKey returns the provisioner key.
func (c *Client) GetProvisionerKey(ctx context.Context, pk string) (ProvisionerKey, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/provisionerkeys/%s", pk), nil,
		func(req *http.Request) {
			req.Header.Add(ProvisionerDaemonKey, pk)
		},
	)
	if err != nil {
		return ProvisionerKey{}, xerrors.Errorf("request to fetch provisioner key failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ProvisionerKey{}, ReadBodyAsError(res)
	}
	var resp ProvisionerKey
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// ListProvisionerKeyDaemons lists all provisioner keys with their associated daemons for an organization.
func (c *Client) ListProvisionerKeyDaemons(ctx context.Context, organizationID uuid.UUID) ([]ProvisionerKeyDaemons, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/provisionerkeys/daemons", organizationID.String()),
		nil,
	)
	if err != nil {
		return nil, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var resp []ProvisionerKeyDaemons
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// DeleteProvisionerKey deletes a provisioner key.
func (c *Client) DeleteProvisionerKey(ctx context.Context, organizationID uuid.UUID, name string) error {
	res, err := c.Request(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v2/organizations/%s/provisionerkeys/%s", organizationID.String(), name),
		nil,
	)
	if err != nil {
		return xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

func ConvertWorkspaceStatus(jobStatus ProvisionerJobStatus, transition WorkspaceTransition) WorkspaceStatus {
	switch jobStatus {
	case ProvisionerJobPending:
		return WorkspaceStatusPending
	case ProvisionerJobRunning:
		switch transition {
		case WorkspaceTransitionStart:
			return WorkspaceStatusStarting
		case WorkspaceTransitionStop:
			return WorkspaceStatusStopping
		case WorkspaceTransitionDelete:
			return WorkspaceStatusDeleting
		}
	case ProvisionerJobSucceeded:
		switch transition {
		case WorkspaceTransitionStart:
			return WorkspaceStatusRunning
		case WorkspaceTransitionStop:
			return WorkspaceStatusStopped
		case WorkspaceTransitionDelete:
			return WorkspaceStatusDeleted
		}
	case ProvisionerJobCanceling:
		return WorkspaceStatusCanceling
	case ProvisionerJobCanceled:
		return WorkspaceStatusCanceled
	case ProvisionerJobFailed:
		return WorkspaceStatusFailed
	}

	// return error status since we should never get here
	return WorkspaceStatusFailed
}
