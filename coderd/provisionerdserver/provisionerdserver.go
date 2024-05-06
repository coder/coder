package provisionerdserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	semconv "go.opentelemetry.io/otel/semconv/v1.14.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"
	protobuf "google.golang.org/protobuf/proto"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/drpc"
	"github.com/coder/coder/v2/provisioner"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

const (
	// DefaultAcquireJobLongPollDur is the time the (deprecated) AcquireJob rpc waits to try to obtain a job before
	// canceling and returning an empty job.
	DefaultAcquireJobLongPollDur = time.Second * 5

	// DefaultHeartbeatInterval is the interval at which the provisioner daemon
	// will update its last seen at timestamp in the database.
	DefaultHeartbeatInterval = time.Minute
)

type Options struct {
	OIDCConfig          promoauth.OAuth2Config
	ExternalAuthConfigs []*externalauth.Config
	// TimeNowFn is only used in tests
	TimeNowFn func() time.Time

	// AcquireJobLongPollDur is used in tests
	AcquireJobLongPollDur time.Duration

	// HeartbeatInterval is the interval at which the provisioner daemon
	// will update its last seen at timestamp in the database.
	HeartbeatInterval time.Duration

	// HeartbeatFn is the function that will be called at the interval
	// specified by HeartbeatInterval.
	// The default function just calls UpdateProvisionerDaemonLastSeenAt.
	// This is mainly used for testing.
	HeartbeatFn func(context.Context) error
}

type server struct {
	// lifecycleCtx must be tied to the API server's lifecycle
	// as when the API server shuts down, we want to cancel any
	// long-running operations.
	lifecycleCtx                context.Context
	AccessURL                   *url.URL
	ID                          uuid.UUID
	OrganizationID              uuid.UUID
	Logger                      slog.Logger
	Provisioners                []database.ProvisionerType
	ExternalAuthConfigs         []*externalauth.Config
	Tags                        Tags
	Database                    database.Store
	Pubsub                      pubsub.Pubsub
	Acquirer                    *Acquirer
	Telemetry                   telemetry.Reporter
	Tracer                      trace.Tracer
	QuotaCommitter              *atomic.Pointer[proto.QuotaCommitter]
	Auditor                     *atomic.Pointer[audit.Auditor]
	TemplateScheduleStore       *atomic.Pointer[schedule.TemplateScheduleStore]
	UserQuietHoursScheduleStore *atomic.Pointer[schedule.UserQuietHoursScheduleStore]
	DeploymentValues            *codersdk.DeploymentValues

	OIDCConfig promoauth.OAuth2Config

	TimeNowFn func() time.Time

	acquireJobLongPollDur time.Duration

	heartbeatInterval time.Duration
	heartbeatFn       func(ctx context.Context) error
}

// We use the null byte (0x00) in generating a canonical map key for tags, so
// it cannot be used in the tag keys or values.

var ErrorTagsContainNullByte = xerrors.New("tags cannot contain the null byte (0x00)")

type Tags map[string]string

func (t Tags) ToJSON() (json.RawMessage, error) {
	r, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}
	return r, err
}

func (t Tags) Valid() error {
	for k, v := range t {
		if slices.Contains([]byte(k), 0x00) || slices.Contains([]byte(v), 0x00) {
			return ErrorTagsContainNullByte
		}
	}
	return nil
}

func NewServer(
	lifecycleCtx context.Context,
	accessURL *url.URL,
	id uuid.UUID,
	organizationID uuid.UUID,
	logger slog.Logger,
	provisioners []database.ProvisionerType,
	tags Tags,
	db database.Store,
	ps pubsub.Pubsub,
	acquirer *Acquirer,
	tel telemetry.Reporter,
	tracer trace.Tracer,
	quotaCommitter *atomic.Pointer[proto.QuotaCommitter],
	auditor *atomic.Pointer[audit.Auditor],
	templateScheduleStore *atomic.Pointer[schedule.TemplateScheduleStore],
	userQuietHoursScheduleStore *atomic.Pointer[schedule.UserQuietHoursScheduleStore],
	deploymentValues *codersdk.DeploymentValues,
	options Options,
) (proto.DRPCProvisionerDaemonServer, error) {
	// Fail-fast if pointers are nil
	if lifecycleCtx == nil {
		return nil, xerrors.New("ctx is nil")
	}
	if quotaCommitter == nil {
		return nil, xerrors.New("quotaCommitter is nil")
	}
	if auditor == nil {
		return nil, xerrors.New("auditor is nil")
	}
	if templateScheduleStore == nil {
		return nil, xerrors.New("templateScheduleStore is nil")
	}
	if userQuietHoursScheduleStore == nil {
		return nil, xerrors.New("userQuietHoursScheduleStore is nil")
	}
	if deploymentValues == nil {
		return nil, xerrors.New("deploymentValues is nil")
	}
	if acquirer == nil {
		return nil, xerrors.New("acquirer is nil")
	}
	if tags == nil {
		return nil, xerrors.Errorf("tags is nil")
	}
	if err := tags.Valid(); err != nil {
		return nil, xerrors.Errorf("invalid tags: %w", err)
	}
	if options.AcquireJobLongPollDur == 0 {
		options.AcquireJobLongPollDur = DefaultAcquireJobLongPollDur
	}
	if options.HeartbeatInterval == 0 {
		options.HeartbeatInterval = DefaultHeartbeatInterval
	}

	s := &server{
		lifecycleCtx:                lifecycleCtx,
		AccessURL:                   accessURL,
		ID:                          id,
		OrganizationID:              organizationID,
		Logger:                      logger,
		Provisioners:                provisioners,
		ExternalAuthConfigs:         options.ExternalAuthConfigs,
		Tags:                        tags,
		Database:                    db,
		Pubsub:                      ps,
		Acquirer:                    acquirer,
		Telemetry:                   tel,
		Tracer:                      tracer,
		QuotaCommitter:              quotaCommitter,
		Auditor:                     auditor,
		TemplateScheduleStore:       templateScheduleStore,
		UserQuietHoursScheduleStore: userQuietHoursScheduleStore,
		DeploymentValues:            deploymentValues,
		OIDCConfig:                  options.OIDCConfig,
		TimeNowFn:                   options.TimeNowFn,
		acquireJobLongPollDur:       options.AcquireJobLongPollDur,
		heartbeatInterval:           options.HeartbeatInterval,
		heartbeatFn:                 options.HeartbeatFn,
	}

	if s.heartbeatFn == nil {
		s.heartbeatFn = s.defaultHeartbeat
	}

	go s.heartbeatLoop()
	return s, nil
}

// timeNow should be used when trying to get the current time for math
// calculations regarding workspace start and stop time.
func (s *server) timeNow() time.Time {
	if s.TimeNowFn != nil {
		return dbtime.Time(s.TimeNowFn())
	}
	return dbtime.Now()
}

// heartbeatLoop runs heartbeatOnce at the interval specified by HeartbeatInterval
// until the lifecycle context is canceled.
func (s *server) heartbeatLoop() {
	tick := time.NewTicker(time.Nanosecond)
	defer tick.Stop()
	for {
		select {
		case <-s.lifecycleCtx.Done():
			s.Logger.Debug(s.lifecycleCtx, "heartbeat loop canceled")
			return
		case <-tick.C:
			if s.lifecycleCtx.Err() != nil {
				return
			}
			start := s.timeNow()
			hbCtx, hbCancel := context.WithTimeout(s.lifecycleCtx, s.heartbeatInterval)
			if err := s.heartbeat(hbCtx); err != nil && !database.IsQueryCanceledError(err) {
				s.Logger.Warn(hbCtx, "heartbeat failed", slog.Error(err))
			}
			hbCancel()
			elapsed := s.timeNow().Sub(start)
			nextBeat := s.heartbeatInterval - elapsed
			// avoid negative interval
			if nextBeat <= 0 {
				nextBeat = time.Nanosecond
			}
			tick.Reset(nextBeat)
		}
	}
}

// heartbeat updates the last seen at timestamp in the database.
// If HeartbeatFn is set, it will be called instead.
func (s *server) heartbeat(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	default:
		return s.heartbeatFn(ctx)
	}
}

func (s *server) defaultHeartbeat(ctx context.Context) error {
	//nolint:gocritic // This is specifically for updating the last seen at timestamp.
	return s.Database.UpdateProvisionerDaemonLastSeenAt(dbauthz.AsSystemRestricted(ctx), database.UpdateProvisionerDaemonLastSeenAtParams{
		ID:         s.ID,
		LastSeenAt: sql.NullTime{Time: s.timeNow(), Valid: true},
	})
}

// AcquireJob queries the database to lock a job.
//
// Deprecated: This method is only available for back-level provisioner daemons.
func (s *server) AcquireJob(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
	//nolint:gocritic // Provisionerd has specific authz rules.
	ctx = dbauthz.AsProvisionerd(ctx)
	// Since AcquireJob blocks until a job is available, we set a long (5s by default) timeout.  This allows back-level
	// provisioner daemons to gracefully shut down within a few seconds, but keeps them from rapidly polling the
	// database.
	acqCtx, acqCancel := context.WithTimeout(ctx, s.acquireJobLongPollDur)
	defer acqCancel()
	job, err := s.Acquirer.AcquireJob(acqCtx, s.OrganizationID, s.ID, s.Provisioners, s.Tags)
	if xerrors.Is(err, context.DeadlineExceeded) {
		s.Logger.Debug(ctx, "successful cancel")
		return &proto.AcquiredJob{}, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("acquire job: %w", err)
	}
	s.Logger.Debug(ctx, "locked job from database", slog.F("job_id", job.ID))
	return s.acquireProtoJob(ctx, job)
}

type jobAndErr struct {
	job database.ProvisionerJob
	err error
}

// AcquireJobWithCancel queries the database to lock a job.
func (s *server) AcquireJobWithCancel(stream proto.DRPCProvisionerDaemon_AcquireJobWithCancelStream) (retErr error) {
	//nolint:gocritic // Provisionerd has specific authz rules.
	streamCtx := dbauthz.AsProvisionerd(stream.Context())
	defer func() {
		closeErr := stream.Close()
		s.Logger.Debug(streamCtx, "closed stream", slog.Error(closeErr))
		if retErr == nil {
			retErr = closeErr
		}
	}()
	acqCtx, acqCancel := context.WithCancel(streamCtx)
	defer acqCancel()
	recvCh := make(chan error, 1)
	go func() {
		_, err := stream.Recv() // cancel is the only message
		recvCh <- err
	}()
	jec := make(chan jobAndErr, 1)
	go func() {
		job, err := s.Acquirer.AcquireJob(acqCtx, s.OrganizationID, s.ID, s.Provisioners, s.Tags)
		jec <- jobAndErr{job: job, err: err}
	}()
	var recvErr error
	var je jobAndErr
	select {
	case recvErr = <-recvCh:
		acqCancel()
		je = <-jec
	case je = <-jec:
	}
	if xerrors.Is(je.err, context.Canceled) {
		s.Logger.Debug(streamCtx, "successful cancel")
		err := stream.Send(&proto.AcquiredJob{})
		if err != nil {
			// often this is just because the other side hangs up and doesn't wait for the cancel, so log at INFO
			s.Logger.Info(streamCtx, "failed to send empty job", slog.Error(err))
			return err
		}
		return nil
	}
	if je.err != nil {
		return xerrors.Errorf("acquire job: %w", je.err)
	}
	logger := s.Logger.With(slog.F("job_id", je.job.ID))
	logger.Debug(streamCtx, "locked job from database")

	if recvErr != nil {
		logger.Error(streamCtx, "recv error and failed to cancel acquire job", slog.Error(recvErr))
		// Well, this is awkward.  We hit an error receiving from the stream, but didn't cancel before we locked a job
		// in the database.  We need to mark this job as failed so the end user can retry if they want to.
		now := dbtime.Now()
		err := s.Database.UpdateProvisionerJobWithCompleteByID(
			//nolint:gocritic // Provisionerd has specific authz rules.
			dbauthz.AsProvisionerd(context.Background()),
			database.UpdateProvisionerJobWithCompleteByIDParams{
				ID: je.job.ID,
				CompletedAt: sql.NullTime{
					Time:  now,
					Valid: true,
				},
				UpdatedAt: now,
				Error: sql.NullString{
					String: "connection to provisioner daemon broken",
					Valid:  true,
				},
				ErrorCode: sql.NullString{},
			})
		if err != nil {
			logger.Error(streamCtx, "error updating failed job", slog.Error(err))
		}
		return recvErr
	}

	pj, err := s.acquireProtoJob(streamCtx, je.job)
	if err != nil {
		return err
	}
	err = stream.Send(pj)
	if err != nil {
		s.Logger.Error(streamCtx, "failed to send job", slog.Error(err))
		return err
	}
	return nil
}

func (s *server) acquireProtoJob(ctx context.Context, job database.ProvisionerJob) (*proto.AcquiredJob, error) {
	// Marks the acquired job as failed with the error message provided.
	failJob := func(errorMessage string) error {
		err := s.Database.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: job.ID,
			CompletedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
			Error: sql.NullString{
				String: errorMessage,
				Valid:  true,
			},
			ErrorCode: job.ErrorCode,
			UpdatedAt: dbtime.Now(),
		})
		if err != nil {
			return xerrors.Errorf("update provisioner job: %w", err)
		}
		return xerrors.Errorf("request job was invalidated: %s", errorMessage)
	}

	user, err := s.Database.GetUserByID(ctx, job.InitiatorID)
	if err != nil {
		return nil, failJob(fmt.Sprintf("get user: %s", err))
	}

	jobTraceMetadata := map[string]string{}
	if job.TraceMetadata.Valid {
		err := json.Unmarshal(job.TraceMetadata.RawMessage, &jobTraceMetadata)
		if err != nil {
			return nil, failJob(fmt.Sprintf("unmarshal metadata: %s", err))
		}
	}

	protoJob := &proto.AcquiredJob{
		JobId:         job.ID.String(),
		CreatedAt:     job.CreatedAt.UnixMilli(),
		Provisioner:   string(job.Provisioner),
		UserName:      user.Username,
		TraceMetadata: jobTraceMetadata,
	}

	switch job.Type {
	case database.ProvisionerJobTypeWorkspaceBuild:
		var input WorkspaceProvisionJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, failJob(fmt.Sprintf("unmarshal job input %q: %s", job.Input, err))
		}
		workspaceBuild, err := s.Database.GetWorkspaceBuildByID(ctx, input.WorkspaceBuildID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get workspace build: %s", err))
		}
		workspace, err := s.Database.GetWorkspaceByID(ctx, workspaceBuild.WorkspaceID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get workspace: %s", err))
		}
		templateVersion, err := s.Database.GetTemplateVersionByID(ctx, workspaceBuild.TemplateVersionID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get template version: %s", err))
		}
		templateVariables, err := s.Database.GetTemplateVersionVariables(ctx, templateVersion.ID)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return nil, failJob(fmt.Sprintf("get template version variables: %s", err))
		}
		template, err := s.Database.GetTemplateByID(ctx, templateVersion.TemplateID.UUID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get template: %s", err))
		}
		owner, err := s.Database.GetUserByID(ctx, workspace.OwnerID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get owner: %s", err))
		}
		ownerGroups, err := s.Database.GetGroupsByOrganizationAndUserID(ctx, database.GetGroupsByOrganizationAndUserIDParams{
			UserID:         owner.ID,
			OrganizationID: s.OrganizationID,
		})
		if err != nil {
			return nil, failJob(fmt.Sprintf("get owner group names: %s", err))
		}
		ownerGroupNames := []string{}
		for _, group := range ownerGroups {
			ownerGroupNames = append(ownerGroupNames, group.Name)
		}
		err = s.Pubsub.Publish(codersdk.WorkspaceNotifyChannel(workspace.ID), []byte{})
		if err != nil {
			return nil, failJob(fmt.Sprintf("publish workspace update: %s", err))
		}

		var workspaceOwnerOIDCAccessToken string
		if s.OIDCConfig != nil {
			workspaceOwnerOIDCAccessToken, err = obtainOIDCAccessToken(ctx, s.Database, s.OIDCConfig, owner.ID)
			if err != nil {
				return nil, failJob(fmt.Sprintf("obtain OIDC access token: %s", err))
			}
		}

		var sessionToken string
		switch workspaceBuild.Transition {
		case database.WorkspaceTransitionStart:
			sessionToken, err = s.regenerateSessionToken(ctx, owner, workspace)
			if err != nil {
				return nil, failJob(fmt.Sprintf("regenerate session token: %s", err))
			}
		case database.WorkspaceTransitionStop, database.WorkspaceTransitionDelete:
			err = deleteSessionToken(ctx, s.Database, workspace)
			if err != nil {
				return nil, failJob(fmt.Sprintf("delete session token: %s", err))
			}
		}

		transition, err := convertWorkspaceTransition(workspaceBuild.Transition)
		if err != nil {
			return nil, failJob(fmt.Sprintf("convert workspace transition: %s", err))
		}

		workspaceBuildParameters, err := s.Database.GetWorkspaceBuildParameters(ctx, workspaceBuild.ID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get workspace build parameters: %s", err))
		}

		dbExternalAuthProviders := []database.ExternalAuthProvider{}
		err = json.Unmarshal(templateVersion.ExternalAuthProviders, &dbExternalAuthProviders)
		if err != nil {
			return nil, xerrors.Errorf("failed to deserialize external_auth_providers value: %w", err)
		}

		externalAuthProviders := make([]*sdkproto.ExternalAuthProvider, 0, len(dbExternalAuthProviders))
		for _, p := range dbExternalAuthProviders {
			link, err := s.Database.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
				ProviderID: p.ID,
				UserID:     owner.ID,
			})
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, failJob(fmt.Sprintf("acquire external auth link: %s", err))
			}
			var config *externalauth.Config
			for _, c := range s.ExternalAuthConfigs {
				if c.ID != p.ID {
					continue
				}
				config = c
				break
			}
			// We weren't able to find a matching config for the ID!
			if config == nil {
				s.Logger.Warn(ctx, "workspace build job is missing external auth provider",
					slog.F("provider_id", p.ID),
					slog.F("template_version_id", templateVersion.ID),
					slog.F("workspace_id", workspaceBuild.WorkspaceID))
				continue
			}

			link, valid, err := config.RefreshToken(ctx, s.Database, link)
			if err != nil {
				return nil, failJob(fmt.Sprintf("refresh external auth link %q: %s", p.ID, err))
			}
			if !valid {
				continue
			}
			externalAuthProviders = append(externalAuthProviders, &sdkproto.ExternalAuthProvider{
				Id:          p.ID,
				AccessToken: link.OAuthAccessToken,
			})
		}

		protoJob.Type = &proto.AcquiredJob_WorkspaceBuild_{
			WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
				WorkspaceBuildId:      workspaceBuild.ID.String(),
				WorkspaceName:         workspace.Name,
				State:                 workspaceBuild.ProvisionerState,
				RichParameterValues:   convertRichParameterValues(workspaceBuildParameters),
				VariableValues:        asVariableValues(templateVariables),
				ExternalAuthProviders: externalAuthProviders,
				Metadata: &sdkproto.Metadata{
					CoderUrl:                      s.AccessURL.String(),
					WorkspaceTransition:           transition,
					WorkspaceName:                 workspace.Name,
					WorkspaceOwner:                owner.Username,
					WorkspaceOwnerEmail:           owner.Email,
					WorkspaceOwnerName:            owner.Name,
					WorkspaceOwnerGroups:          ownerGroupNames,
					WorkspaceOwnerOidcAccessToken: workspaceOwnerOIDCAccessToken,
					WorkspaceId:                   workspace.ID.String(),
					WorkspaceOwnerId:              owner.ID.String(),
					TemplateId:                    template.ID.String(),
					TemplateName:                  template.Name,
					TemplateVersion:               templateVersion.Name,
					WorkspaceOwnerSessionToken:    sessionToken,
				},
				LogLevel: input.LogLevel,
			},
		}
	case database.ProvisionerJobTypeTemplateVersionDryRun:
		var input TemplateVersionDryRunJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, failJob(fmt.Sprintf("unmarshal job input %q: %s", job.Input, err))
		}

		templateVersion, err := s.Database.GetTemplateVersionByID(ctx, input.TemplateVersionID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get template version: %s", err))
		}
		templateVariables, err := s.Database.GetTemplateVersionVariables(ctx, templateVersion.ID)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return nil, failJob(fmt.Sprintf("get template version variables: %s", err))
		}

		protoJob.Type = &proto.AcquiredJob_TemplateDryRun_{
			TemplateDryRun: &proto.AcquiredJob_TemplateDryRun{
				RichParameterValues: convertRichParameterValues(input.RichParameterValues),
				VariableValues:      asVariableValues(templateVariables),
				Metadata: &sdkproto.Metadata{
					CoderUrl:      s.AccessURL.String(),
					WorkspaceName: input.WorkspaceName,
				},
			},
		}
	case database.ProvisionerJobTypeTemplateVersionImport:
		var input TemplateVersionImportJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, failJob(fmt.Sprintf("unmarshal job input %q: %s", job.Input, err))
		}

		userVariableValues, err := s.includeLastVariableValues(ctx, input.TemplateVersionID, input.UserVariableValues)
		if err != nil {
			return nil, failJob(err.Error())
		}

		protoJob.Type = &proto.AcquiredJob_TemplateImport_{
			TemplateImport: &proto.AcquiredJob_TemplateImport{
				UserVariableValues: convertVariableValues(userVariableValues),
				Metadata: &sdkproto.Metadata{
					CoderUrl: s.AccessURL.String(),
				},
			},
		}
	}
	switch job.StorageMethod {
	case database.ProvisionerStorageMethodFile:
		file, err := s.Database.GetFileByID(ctx, job.FileID)
		if err != nil {
			return nil, failJob(fmt.Sprintf("get file by hash: %s", err))
		}
		protoJob.TemplateSourceArchive = file.Data
	default:
		return nil, failJob(fmt.Sprintf("unsupported storage method: %s", job.StorageMethod))
	}
	if protobuf.Size(protoJob) > drpc.MaxMessageSize {
		return nil, failJob(fmt.Sprintf("payload was too big: %d > %d", protobuf.Size(protoJob), drpc.MaxMessageSize))
	}

	return protoJob, err
}

func (s *server) includeLastVariableValues(ctx context.Context, templateVersionID uuid.UUID, userVariableValues []codersdk.VariableValue) ([]codersdk.VariableValue, error) {
	var values []codersdk.VariableValue
	values = append(values, userVariableValues...)

	if templateVersionID == uuid.Nil {
		return values, nil
	}

	templateVersion, err := s.Database.GetTemplateVersionByID(ctx, templateVersionID)
	if err != nil {
		return nil, xerrors.Errorf("get template version: %w", err)
	}

	if templateVersion.TemplateID.UUID == uuid.Nil {
		return values, nil
	}

	template, err := s.Database.GetTemplateByID(ctx, templateVersion.TemplateID.UUID)
	if err != nil {
		return nil, xerrors.Errorf("get template: %w", err)
	}

	if template.ActiveVersionID == uuid.Nil {
		return values, nil
	}

	templateVariables, err := s.Database.GetTemplateVersionVariables(ctx, template.ActiveVersionID)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("get template version variables: %w", err)
	}

	for _, templateVariable := range templateVariables {
		var alreadyAdded bool
		for _, uvv := range userVariableValues {
			if uvv.Name == templateVariable.Name {
				alreadyAdded = true
				break
			}
		}

		if alreadyAdded {
			continue
		}

		values = append(values, codersdk.VariableValue{
			Name:  templateVariable.Name,
			Value: templateVariable.Value,
		})
	}
	return values, nil
}

func (s *server) CommitQuota(ctx context.Context, request *proto.CommitQuotaRequest) (*proto.CommitQuotaResponse, error) {
	ctx, span := s.startTrace(ctx, tracing.FuncName())
	defer span.End()

	//nolint:gocritic // Provisionerd has specific authz rules.
	ctx = dbauthz.AsProvisionerd(ctx)
	jobID, err := uuid.Parse(request.JobId)
	if err != nil {
		return nil, xerrors.Errorf("parse job id: %w", err)
	}

	job, err := s.Database.GetProvisionerJobByID(ctx, jobID)
	if err != nil {
		return nil, xerrors.Errorf("get job: %w", err)
	}
	if !job.WorkerID.Valid {
		return nil, xerrors.New("job isn't running yet")
	}

	if job.WorkerID.UUID.String() != s.ID.String() {
		return nil, xerrors.New("you don't own this job")
	}

	q := s.QuotaCommitter.Load()
	if q == nil {
		// We're probably in community edition or a test.
		return &proto.CommitQuotaResponse{
			Budget: -1,
			Ok:     true,
		}, nil
	}
	return (*q).CommitQuota(ctx, request)
}

func (s *server) UpdateJob(ctx context.Context, request *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
	ctx, span := s.startTrace(ctx, tracing.FuncName())
	defer span.End()

	//nolint:gocritic // Provisionerd has specific authz rules.
	ctx = dbauthz.AsProvisionerd(ctx)
	parsedID, err := uuid.Parse(request.JobId)
	if err != nil {
		return nil, xerrors.Errorf("parse job id: %w", err)
	}
	s.Logger.Debug(ctx, "stage UpdateJob starting", slog.F("job_id", parsedID))
	job, err := s.Database.GetProvisionerJobByID(ctx, parsedID)
	if err != nil {
		return nil, xerrors.Errorf("get job: %w", err)
	}
	if !job.WorkerID.Valid {
		return nil, xerrors.New("job isn't running yet")
	}
	if job.WorkerID.UUID.String() != s.ID.String() {
		return nil, xerrors.New("you don't own this job")
	}
	err = s.Database.UpdateProvisionerJobByID(ctx, database.UpdateProvisionerJobByIDParams{
		ID:        parsedID,
		UpdatedAt: dbtime.Now(),
	})
	if err != nil {
		return nil, xerrors.Errorf("update job: %w", err)
	}

	if len(request.Logs) > 0 {
		//nolint:exhaustruct // We append to the additional fields below.
		insertParams := database.InsertProvisionerJobLogsParams{
			JobID: parsedID,
		}
		for _, log := range request.Logs {
			logLevel, err := convertLogLevel(log.Level)
			if err != nil {
				return nil, xerrors.Errorf("convert log level: %w", err)
			}
			logSource, err := convertLogSource(log.Source)
			if err != nil {
				return nil, xerrors.Errorf("convert log source: %w", err)
			}
			insertParams.CreatedAt = append(insertParams.CreatedAt, time.UnixMilli(log.CreatedAt))
			insertParams.Level = append(insertParams.Level, logLevel)
			insertParams.Stage = append(insertParams.Stage, log.Stage)
			insertParams.Source = append(insertParams.Source, logSource)
			insertParams.Output = append(insertParams.Output, log.Output)
			s.Logger.Debug(ctx, "job log",
				slog.F("job_id", parsedID),
				slog.F("stage", log.Stage),
				slog.F("output", log.Output))
		}

		logs, err := s.Database.InsertProvisionerJobLogs(ctx, insertParams)
		if err != nil {
			s.Logger.Error(ctx, "failed to insert job logs", slog.F("job_id", parsedID), slog.Error(err))
			return nil, xerrors.Errorf("insert job logs: %w", err)
		}
		// Publish by the lowest log ID inserted so the log stream will fetch
		// everything from that point.
		lowestID := logs[0].ID
		s.Logger.Debug(ctx, "inserted job logs", slog.F("job_id", parsedID))
		data, err := json.Marshal(provisionersdk.ProvisionerJobLogsNotifyMessage{
			CreatedAfter: lowestID - 1,
		})
		if err != nil {
			return nil, xerrors.Errorf("marshal: %w", err)
		}
		err = s.Pubsub.Publish(provisionersdk.ProvisionerJobLogsNotifyChannel(parsedID), data)
		if err != nil {
			s.Logger.Error(ctx, "failed to publish job logs", slog.F("job_id", parsedID), slog.Error(err))
			return nil, xerrors.Errorf("publish job logs: %w", err)
		}
		s.Logger.Debug(ctx, "published job logs", slog.F("job_id", parsedID))
	}

	if len(request.Readme) > 0 {
		err := s.Database.UpdateTemplateVersionDescriptionByJobID(ctx, database.UpdateTemplateVersionDescriptionByJobIDParams{
			JobID:     job.ID,
			Readme:    string(request.Readme),
			UpdatedAt: dbtime.Now(),
		})
		if err != nil {
			return nil, xerrors.Errorf("update template version description: %w", err)
		}
	}

	if len(request.TemplateVariables) > 0 {
		templateVersion, err := s.Database.GetTemplateVersionByJobID(ctx, job.ID)
		if err != nil {
			s.Logger.Error(ctx, "failed to get the template version", slog.F("job_id", parsedID), slog.Error(err))
			return nil, xerrors.Errorf("get template version by job id: %w", err)
		}

		var variableValues []*sdkproto.VariableValue
		var variablesWithMissingValues []string
		for _, templateVariable := range request.TemplateVariables {
			s.Logger.Debug(ctx, "insert template variable", slog.F("template_version_id", templateVersion.ID), slog.F("template_variable", redactTemplateVariable(templateVariable)))

			value := templateVariable.DefaultValue
			for _, v := range request.UserVariableValues {
				if v.Name == templateVariable.Name {
					value = v.Value
					break
				}
			}

			if templateVariable.Required && value == "" {
				variablesWithMissingValues = append(variablesWithMissingValues, templateVariable.Name)
			}

			variableValues = append(variableValues, &sdkproto.VariableValue{
				Name:      templateVariable.Name,
				Value:     value,
				Sensitive: templateVariable.Sensitive,
			})

			_, err = s.Database.InsertTemplateVersionVariable(ctx, database.InsertTemplateVersionVariableParams{
				TemplateVersionID: templateVersion.ID,
				Name:              templateVariable.Name,
				Description:       templateVariable.Description,
				Type:              templateVariable.Type,
				DefaultValue:      templateVariable.DefaultValue,
				Required:          templateVariable.Required,
				Sensitive:         templateVariable.Sensitive,
				Value:             value,
			})
			if err != nil {
				return nil, xerrors.Errorf("insert parameter schema: %w", err)
			}
		}

		if len(variablesWithMissingValues) > 0 {
			return nil, xerrors.Errorf("required template variables need values: %s", strings.Join(variablesWithMissingValues, ", "))
		}

		return &proto.UpdateJobResponse{
			Canceled:       job.CanceledAt.Valid,
			VariableValues: variableValues,
		}, nil
	}

	return &proto.UpdateJobResponse{
		Canceled: job.CanceledAt.Valid,
	}, nil
}

func (s *server) FailJob(ctx context.Context, failJob *proto.FailedJob) (*proto.Empty, error) {
	ctx, span := s.startTrace(ctx, tracing.FuncName())
	defer span.End()

	//nolint:gocritic // Provisionerd has specific authz rules.
	ctx = dbauthz.AsProvisionerd(ctx)
	jobID, err := uuid.Parse(failJob.JobId)
	if err != nil {
		return nil, xerrors.Errorf("parse job id: %w", err)
	}
	s.Logger.Debug(ctx, "stage FailJob starting", slog.F("job_id", jobID))
	job, err := s.Database.GetProvisionerJobByID(ctx, jobID)
	if err != nil {
		return nil, xerrors.Errorf("get provisioner job: %w", err)
	}
	if job.WorkerID.UUID.String() != s.ID.String() {
		return nil, xerrors.New("you don't own this job")
	}
	if job.CompletedAt.Valid {
		return nil, xerrors.Errorf("job already completed")
	}
	job.CompletedAt = sql.NullTime{
		Time:  dbtime.Now(),
		Valid: true,
	}
	job.Error = sql.NullString{
		String: failJob.Error,
		Valid:  failJob.Error != "",
	}
	job.ErrorCode = sql.NullString{
		String: failJob.ErrorCode,
		Valid:  failJob.ErrorCode != "",
	}

	err = s.Database.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
		ID:          jobID,
		CompletedAt: job.CompletedAt,
		UpdatedAt:   dbtime.Now(),
		Error:       job.Error,
		ErrorCode:   job.ErrorCode,
	})
	if err != nil {
		return nil, xerrors.Errorf("update provisioner job: %w", err)
	}
	s.Telemetry.Report(&telemetry.Snapshot{
		ProvisionerJobs: []telemetry.ProvisionerJob{telemetry.ConvertProvisionerJob(job)},
	})

	switch jobType := failJob.Type.(type) {
	case *proto.FailedJob_WorkspaceBuild_:
		var input WorkspaceProvisionJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, xerrors.Errorf("unmarshal workspace provision input: %w", err)
		}

		var build database.WorkspaceBuild
		err = s.Database.InTx(func(db database.Store) error {
			build, err = db.GetWorkspaceBuildByID(ctx, input.WorkspaceBuildID)
			if err != nil {
				return xerrors.Errorf("get workspace build: %w", err)
			}

			if jobType.WorkspaceBuild.State != nil {
				err = db.UpdateWorkspaceBuildProvisionerStateByID(ctx, database.UpdateWorkspaceBuildProvisionerStateByIDParams{
					ID:               input.WorkspaceBuildID,
					UpdatedAt:        dbtime.Now(),
					ProvisionerState: jobType.WorkspaceBuild.State,
				})
				if err != nil {
					return xerrors.Errorf("update workspace build state: %w", err)
				}
				err = db.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
					ID:          input.WorkspaceBuildID,
					UpdatedAt:   dbtime.Now(),
					Deadline:    build.Deadline,
					MaxDeadline: build.MaxDeadline,
				})
				if err != nil {
					return xerrors.Errorf("update workspace build deadline: %w", err)
				}
			}

			return nil
		}, nil)
		if err != nil {
			return nil, err
		}

		err = s.Pubsub.Publish(codersdk.WorkspaceNotifyChannel(build.WorkspaceID), []byte{})
		if err != nil {
			return nil, xerrors.Errorf("update workspace: %w", err)
		}
	case *proto.FailedJob_TemplateImport_:
	}

	// if failed job is a workspace build, audit the outcome
	if job.Type == database.ProvisionerJobTypeWorkspaceBuild {
		auditor := s.Auditor.Load()
		build, err := s.Database.GetWorkspaceBuildByJobID(ctx, job.ID)
		if err != nil {
			s.Logger.Error(ctx, "audit log - get build", slog.Error(err))
		} else {
			auditAction := auditActionFromTransition(build.Transition)
			workspace, err := s.Database.GetWorkspaceByID(ctx, build.WorkspaceID)
			if err != nil {
				s.Logger.Error(ctx, "audit log - get workspace", slog.Error(err))
			} else {
				previousBuildNumber := build.BuildNumber - 1
				previousBuild, prevBuildErr := s.Database.GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx, database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{
					WorkspaceID: workspace.ID,
					BuildNumber: previousBuildNumber,
				})
				if prevBuildErr != nil {
					previousBuild = database.WorkspaceBuild{}
				}

				// We pass the below information to the Auditor so that it
				// can form a friendly string for the user to view in the UI.
				buildResourceInfo := audit.AdditionalFields{
					WorkspaceName: workspace.Name,
					BuildNumber:   strconv.FormatInt(int64(build.BuildNumber), 10),
					BuildReason:   database.BuildReason(string(build.Reason)),
					WorkspaceID:   workspace.ID,
				}

				wriBytes, err := json.Marshal(buildResourceInfo)
				if err != nil {
					s.Logger.Error(ctx, "marshal workspace resource info for failed job", slog.Error(err))
				}

				bag := audit.BaggageFromContext(ctx)

				audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.WorkspaceBuild]{
					Audit:            *auditor,
					Log:              s.Logger,
					UserID:           job.InitiatorID,
					OrganizationID:   workspace.OrganizationID,
					RequestID:        job.ID,
					IP:               bag.IP,
					Action:           auditAction,
					Old:              previousBuild,
					New:              build,
					Status:           http.StatusInternalServerError,
					AdditionalFields: wriBytes,
				})
			}
		}
	}

	data, err := json.Marshal(provisionersdk.ProvisionerJobLogsNotifyMessage{EndOfLogs: true})
	if err != nil {
		return nil, xerrors.Errorf("marshal job log: %w", err)
	}
	err = s.Pubsub.Publish(provisionersdk.ProvisionerJobLogsNotifyChannel(jobID), data)
	if err != nil {
		s.Logger.Error(ctx, "failed to publish end of job logs", slog.F("job_id", jobID), slog.Error(err))
		return nil, xerrors.Errorf("publish end of job logs: %w", err)
	}
	return &proto.Empty{}, nil
}

// CompleteJob is triggered by a provision daemon to mark a provisioner job as completed.
func (s *server) CompleteJob(ctx context.Context, completed *proto.CompletedJob) (*proto.Empty, error) {
	ctx, span := s.startTrace(ctx, tracing.FuncName())
	defer span.End()

	//nolint:gocritic // Provisionerd has specific authz rules.
	ctx = dbauthz.AsProvisionerd(ctx)
	jobID, err := uuid.Parse(completed.JobId)
	if err != nil {
		return nil, xerrors.Errorf("parse job id: %w", err)
	}
	s.Logger.Debug(ctx, "stage CompleteJob starting", slog.F("job_id", jobID))
	job, err := s.Database.GetProvisionerJobByID(ctx, jobID)
	if err != nil {
		return nil, xerrors.Errorf("get job by id: %w", err)
	}
	if job.WorkerID.UUID.String() != s.ID.String() {
		return nil, xerrors.Errorf("you don't own this job")
	}

	telemetrySnapshot := &telemetry.Snapshot{}
	// Items are added to this snapshot as they complete!
	defer s.Telemetry.Report(telemetrySnapshot)

	switch jobType := completed.Type.(type) {
	case *proto.CompletedJob_TemplateImport_:
		var input TemplateVersionImportJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, xerrors.Errorf("template version ID is expected: %w", err)
		}

		for transition, resources := range map[database.WorkspaceTransition][]*sdkproto.Resource{
			database.WorkspaceTransitionStart: jobType.TemplateImport.StartResources,
			database.WorkspaceTransitionStop:  jobType.TemplateImport.StopResources,
		} {
			for _, resource := range resources {
				s.Logger.Info(ctx, "inserting template import job resource",
					slog.F("job_id", job.ID.String()),
					slog.F("resource_name", resource.Name),
					slog.F("resource_type", resource.Type),
					slog.F("transition", transition))

				err = InsertWorkspaceResource(ctx, s.Database, jobID, transition, resource, telemetrySnapshot)
				if err != nil {
					return nil, xerrors.Errorf("insert resource: %w", err)
				}
			}
		}

		for _, richParameter := range jobType.TemplateImport.RichParameters {
			s.Logger.Info(ctx, "inserting template import job parameter",
				slog.F("job_id", job.ID.String()),
				slog.F("parameter_name", richParameter.Name),
				slog.F("type", richParameter.Type),
				slog.F("ephemeral", richParameter.Ephemeral),
			)
			options, err := json.Marshal(richParameter.Options)
			if err != nil {
				return nil, xerrors.Errorf("marshal parameter options: %w", err)
			}

			var validationMin, validationMax sql.NullInt32
			if richParameter.ValidationMin != nil {
				validationMin = sql.NullInt32{
					Int32: *richParameter.ValidationMin,
					Valid: true,
				}
			}
			if richParameter.ValidationMax != nil {
				validationMax = sql.NullInt32{
					Int32: *richParameter.ValidationMax,
					Valid: true,
				}
			}

			_, err = s.Database.InsertTemplateVersionParameter(ctx, database.InsertTemplateVersionParameterParams{
				TemplateVersionID:   input.TemplateVersionID,
				Name:                richParameter.Name,
				DisplayName:         richParameter.DisplayName,
				Description:         richParameter.Description,
				Type:                richParameter.Type,
				Mutable:             richParameter.Mutable,
				DefaultValue:        richParameter.DefaultValue,
				Icon:                richParameter.Icon,
				Options:             options,
				ValidationRegex:     richParameter.ValidationRegex,
				ValidationError:     richParameter.ValidationError,
				ValidationMin:       validationMin,
				ValidationMax:       validationMax,
				ValidationMonotonic: richParameter.ValidationMonotonic,
				Required:            richParameter.Required,
				DisplayOrder:        richParameter.Order,
				Ephemeral:           richParameter.Ephemeral,
			})
			if err != nil {
				return nil, xerrors.Errorf("insert parameter: %w", err)
			}
		}

		var completedError sql.NullString

		for _, externalAuthProvider := range jobType.TemplateImport.ExternalAuthProviders {
			contains := false
			for _, configuredProvider := range s.ExternalAuthConfigs {
				if configuredProvider.ID == externalAuthProvider.Id {
					contains = true
					break
				}
			}
			if !contains {
				completedError = sql.NullString{
					String: fmt.Sprintf("external auth provider %q is not configured", externalAuthProvider.Id),
					Valid:  true,
				}
				break
			}
		}

		// Fallback to `ExternalAuthProvidersNames` if it was specified and `ExternalAuthProviders`
		// was not. Gives us backwards compatibility with custom provisioners that haven't been
		// updated to use the new field yet.
		var externalAuthProviders []database.ExternalAuthProvider
		if providersLen := len(jobType.TemplateImport.ExternalAuthProviders); providersLen > 0 {
			externalAuthProviders = make([]database.ExternalAuthProvider, 0, providersLen)
			for _, provider := range jobType.TemplateImport.ExternalAuthProviders {
				externalAuthProviders = append(externalAuthProviders, database.ExternalAuthProvider{
					ID:       provider.Id,
					Optional: provider.Optional,
				})
			}
		} else if namesLen := len(jobType.TemplateImport.ExternalAuthProvidersNames); namesLen > 0 {
			externalAuthProviders = make([]database.ExternalAuthProvider, 0, namesLen)
			for _, providerID := range jobType.TemplateImport.ExternalAuthProvidersNames {
				externalAuthProviders = append(externalAuthProviders, database.ExternalAuthProvider{
					ID: providerID,
				})
			}
		}

		externalAuthProvidersMessage, err := json.Marshal(externalAuthProviders)
		if err != nil {
			return nil, xerrors.Errorf("failed to serialize external_auth_providers value: %w", err)
		}

		err = s.Database.UpdateTemplateVersionExternalAuthProvidersByJobID(ctx, database.UpdateTemplateVersionExternalAuthProvidersByJobIDParams{
			JobID:                 jobID,
			ExternalAuthProviders: json.RawMessage(externalAuthProvidersMessage),
			UpdatedAt:             dbtime.Now(),
		})
		if err != nil {
			return nil, xerrors.Errorf("update template version external auth providers: %w", err)
		}

		err = s.Database.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID:        jobID,
			UpdatedAt: dbtime.Now(),
			CompletedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
			Error:     completedError,
			ErrorCode: sql.NullString{},
		})
		if err != nil {
			return nil, xerrors.Errorf("update provisioner job: %w", err)
		}
		s.Logger.Debug(ctx, "marked import job as completed", slog.F("job_id", jobID))
		if err != nil {
			return nil, xerrors.Errorf("complete job: %w", err)
		}
	case *proto.CompletedJob_WorkspaceBuild_:
		var input WorkspaceProvisionJob
		err = json.Unmarshal(job.Input, &input)
		if err != nil {
			return nil, xerrors.Errorf("unmarshal job data: %w", err)
		}

		workspaceBuild, err := s.Database.GetWorkspaceBuildByID(ctx, input.WorkspaceBuildID)
		if err != nil {
			return nil, xerrors.Errorf("get workspace build: %w", err)
		}

		var workspace database.Workspace
		var getWorkspaceError error

		err = s.Database.InTx(func(db database.Store) error {
			// It's important we use s.timeNow() here because we want to be
			// able to customize the current time from within tests.
			now := s.timeNow()

			workspace, getWorkspaceError = db.GetWorkspaceByID(ctx, workspaceBuild.WorkspaceID)
			if getWorkspaceError != nil {
				s.Logger.Error(ctx,
					"fetch workspace for build",
					slog.F("workspace_build_id", workspaceBuild.ID),
					slog.F("workspace_id", workspaceBuild.WorkspaceID),
				)
				return getWorkspaceError
			}

			autoStop, err := schedule.CalculateAutostop(ctx, schedule.CalculateAutostopParams{
				Database:                    db,
				TemplateScheduleStore:       *s.TemplateScheduleStore.Load(),
				UserQuietHoursScheduleStore: *s.UserQuietHoursScheduleStore.Load(),
				Now:                         now,
				Workspace:                   workspace,
				// Allowed to be the empty string.
				WorkspaceAutostart: workspace.AutostartSchedule.String,
			})
			if err != nil {
				return xerrors.Errorf("calculate auto stop: %w", err)
			}

			err = db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
				ID:        jobID,
				UpdatedAt: now,
				CompletedAt: sql.NullTime{
					Time:  now,
					Valid: true,
				},
				Error:     sql.NullString{},
				ErrorCode: sql.NullString{},
			})
			if err != nil {
				return xerrors.Errorf("update provisioner job: %w", err)
			}
			err = db.UpdateWorkspaceBuildProvisionerStateByID(ctx, database.UpdateWorkspaceBuildProvisionerStateByIDParams{
				ID:               workspaceBuild.ID,
				ProvisionerState: jobType.WorkspaceBuild.State,
				UpdatedAt:        now,
			})
			if err != nil {
				return xerrors.Errorf("update workspace build provisioner state: %w", err)
			}
			err = db.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
				ID:          workspaceBuild.ID,
				Deadline:    autoStop.Deadline,
				MaxDeadline: autoStop.MaxDeadline,
				UpdatedAt:   now,
			})
			if err != nil {
				return xerrors.Errorf("update workspace build deadline: %w", err)
			}

			agentTimeouts := make(map[time.Duration]bool) // A set of agent timeouts.
			// This could be a bulk insert to improve performance.
			for _, protoResource := range jobType.WorkspaceBuild.Resources {
				for _, protoAgent := range protoResource.Agents {
					dur := time.Duration(protoAgent.GetConnectionTimeoutSeconds()) * time.Second
					agentTimeouts[dur] = true
				}
				err = InsertWorkspaceResource(ctx, db, job.ID, workspaceBuild.Transition, protoResource, telemetrySnapshot)
				if err != nil {
					return xerrors.Errorf("insert provisioner job: %w", err)
				}
			}

			// On start, we want to ensure that workspace agents timeout statuses
			// are propagated. This method is simple and does not protect against
			// notifying in edge cases like when a workspace is stopped soon
			// after being started.
			//
			// Agent timeouts could be minutes apart, resulting in an unresponsive
			// experience, so we'll notify after every unique timeout seconds.
			if !input.DryRun && workspaceBuild.Transition == database.WorkspaceTransitionStart && len(agentTimeouts) > 0 {
				timeouts := maps.Keys(agentTimeouts)
				slices.Sort(timeouts)

				var updates []<-chan time.Time
				for _, d := range timeouts {
					s.Logger.Debug(ctx, "triggering workspace notification after agent timeout",
						slog.F("workspace_build_id", workspaceBuild.ID),
						slog.F("timeout", d),
					)
					// Agents are inserted with `dbtime.Now()`, this triggers a
					// workspace event approximately after created + timeout seconds.
					updates = append(updates, time.After(d))
				}
				go func() {
					for _, wait := range updates {
						select {
						case <-s.lifecycleCtx.Done():
							// If the server is shutting down, we don't want to wait around.
							s.Logger.Debug(ctx, "stopping notifications due to server shutdown",
								slog.F("workspace_build_id", workspaceBuild.ID),
							)
							return
						case <-wait:
							// Wait for the next potential timeout to occur.
							if err := s.Pubsub.Publish(codersdk.WorkspaceNotifyChannel(workspaceBuild.WorkspaceID), []byte{}); err != nil {
								if s.lifecycleCtx.Err() != nil {
									// If the server is shutting down, we don't want to log this error, nor wait around.
									s.Logger.Debug(ctx, "stopping notifications due to server shutdown",
										slog.F("workspace_build_id", workspaceBuild.ID),
									)
									return
								}
								s.Logger.Error(ctx, "workspace notification after agent timeout failed",
									slog.F("workspace_build_id", workspaceBuild.ID),
									slog.Error(err),
								)
							}
						}
					}
				}()
			}

			if workspaceBuild.Transition != database.WorkspaceTransitionDelete {
				// This is for deleting a workspace!
				return nil
			}

			err = db.UpdateWorkspaceDeletedByID(ctx, database.UpdateWorkspaceDeletedByIDParams{
				ID:      workspaceBuild.WorkspaceID,
				Deleted: true,
			})
			if err != nil {
				return xerrors.Errorf("update workspace deleted: %w", err)
			}

			return nil
		}, nil)
		if err != nil {
			return nil, xerrors.Errorf("complete job: %w", err)
		}

		// audit the outcome of the workspace build
		if getWorkspaceError == nil {
			auditor := s.Auditor.Load()
			auditAction := auditActionFromTransition(workspaceBuild.Transition)

			previousBuildNumber := workspaceBuild.BuildNumber - 1
			previousBuild, prevBuildErr := s.Database.GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx, database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{
				WorkspaceID: workspace.ID,
				BuildNumber: previousBuildNumber,
			})
			if prevBuildErr != nil {
				previousBuild = database.WorkspaceBuild{}
			}

			// We pass the below information to the Auditor so that it
			// can form a friendly string for the user to view in the UI.
			buildResourceInfo := audit.AdditionalFields{
				WorkspaceName: workspace.Name,
				BuildNumber:   strconv.FormatInt(int64(workspaceBuild.BuildNumber), 10),
				BuildReason:   database.BuildReason(string(workspaceBuild.Reason)),
				WorkspaceID:   workspace.ID,
			}

			wriBytes, err := json.Marshal(buildResourceInfo)
			if err != nil {
				s.Logger.Error(ctx, "marshal resource info for successful job", slog.Error(err))
			}

			bag := audit.BaggageFromContext(ctx)

			audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.WorkspaceBuild]{
				Audit:            *auditor,
				Log:              s.Logger,
				UserID:           job.InitiatorID,
				OrganizationID:   workspace.OrganizationID,
				RequestID:        job.ID,
				IP:               bag.IP,
				Action:           auditAction,
				Old:              previousBuild,
				New:              workspaceBuild,
				Status:           http.StatusOK,
				AdditionalFields: wriBytes,
			})
		}

		err = s.Pubsub.Publish(codersdk.WorkspaceNotifyChannel(workspaceBuild.WorkspaceID), []byte{})
		if err != nil {
			return nil, xerrors.Errorf("update workspace: %w", err)
		}
	case *proto.CompletedJob_TemplateDryRun_:
		for _, resource := range jobType.TemplateDryRun.Resources {
			s.Logger.Info(ctx, "inserting template dry-run job resource",
				slog.F("job_id", job.ID.String()),
				slog.F("resource_name", resource.Name),
				slog.F("resource_type", resource.Type))

			err = InsertWorkspaceResource(ctx, s.Database, jobID, database.WorkspaceTransitionStart, resource, telemetrySnapshot)
			if err != nil {
				return nil, xerrors.Errorf("insert resource: %w", err)
			}
		}

		err = s.Database.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID:        jobID,
			UpdatedAt: dbtime.Now(),
			CompletedAt: sql.NullTime{
				Time:  dbtime.Now(),
				Valid: true,
			},
			Error:     sql.NullString{},
			ErrorCode: sql.NullString{},
		})
		if err != nil {
			return nil, xerrors.Errorf("update provisioner job: %w", err)
		}
		s.Logger.Debug(ctx, "marked template dry-run job as completed", slog.F("job_id", jobID))
		if err != nil {
			return nil, xerrors.Errorf("complete job: %w", err)
		}

	default:
		if completed.Type == nil {
			return nil, xerrors.Errorf("type payload must be provided")
		}
		return nil, xerrors.Errorf("unknown job type %q; ensure coderd and provisionerd versions match",
			reflect.TypeOf(completed.Type).String())
	}

	data, err := json.Marshal(provisionersdk.ProvisionerJobLogsNotifyMessage{EndOfLogs: true})
	if err != nil {
		return nil, xerrors.Errorf("marshal job log: %w", err)
	}
	err = s.Pubsub.Publish(provisionersdk.ProvisionerJobLogsNotifyChannel(jobID), data)
	if err != nil {
		s.Logger.Error(ctx, "failed to publish end of job logs", slog.F("job_id", jobID), slog.Error(err))
		return nil, xerrors.Errorf("publish end of job logs: %w", err)
	}

	s.Logger.Debug(ctx, "stage CompleteJob done", slog.F("job_id", jobID))
	return &proto.Empty{}, nil
}

func (s *server) startTrace(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return s.Tracer.Start(ctx, name, append(opts, trace.WithAttributes(
		semconv.ServiceNameKey.String("coderd.provisionerd"),
	))...)
}

func InsertWorkspaceResource(ctx context.Context, db database.Store, jobID uuid.UUID, transition database.WorkspaceTransition, protoResource *sdkproto.Resource, snapshot *telemetry.Snapshot) error {
	resource, err := db.InsertWorkspaceResource(ctx, database.InsertWorkspaceResourceParams{
		ID:         uuid.New(),
		CreatedAt:  dbtime.Now(),
		JobID:      jobID,
		Transition: transition,
		Type:       protoResource.Type,
		Name:       protoResource.Name,
		Hide:       protoResource.Hide,
		Icon:       protoResource.Icon,
		DailyCost:  protoResource.DailyCost,
		InstanceType: sql.NullString{
			String: protoResource.InstanceType,
			Valid:  protoResource.InstanceType != "",
		},
	})
	if err != nil {
		return xerrors.Errorf("insert provisioner job resource %q: %w", protoResource.Name, err)
	}
	snapshot.WorkspaceResources = append(snapshot.WorkspaceResources, telemetry.ConvertWorkspaceResource(resource))

	var (
		agentNames = make(map[string]struct{})
		appSlugs   = make(map[string]struct{})
	)
	for _, prAgent := range protoResource.Agents {
		if _, ok := agentNames[prAgent.Name]; ok {
			return xerrors.Errorf("duplicate agent name %q", prAgent.Name)
		}
		agentNames[prAgent.Name] = struct{}{}

		var instanceID sql.NullString
		if prAgent.GetInstanceId() != "" {
			instanceID = sql.NullString{
				String: prAgent.GetInstanceId(),
				Valid:  true,
			}
		}

		env := make(map[string]string)
		// For now, we only support adding extra envs, not overriding
		// existing ones or performing other manipulations. In future
		// we may write these to a separate table so we can perform
		// conditional logic on the agent.
		for _, e := range prAgent.ExtraEnvs {
			env[e.Name] = e.Value
		}
		// Allow the agent defined envs to override extra envs.
		for k, v := range prAgent.Env {
			env[k] = v
		}

		var envJSON pqtype.NullRawMessage
		if len(env) > 0 {
			data, err := json.Marshal(env)
			if err != nil {
				return xerrors.Errorf("marshal env: %w", err)
			}
			envJSON = pqtype.NullRawMessage{
				RawMessage: data,
				Valid:      true,
			}
		}
		authToken := uuid.New()
		if prAgent.GetToken() != "" {
			authToken, err = uuid.Parse(prAgent.GetToken())
			if err != nil {
				return xerrors.Errorf("invalid auth token format; must be uuid: %w", err)
			}
		}

		agentID := uuid.New()
		dbAgent, err := db.InsertWorkspaceAgent(ctx, database.InsertWorkspaceAgentParams{
			ID:                       agentID,
			CreatedAt:                dbtime.Now(),
			UpdatedAt:                dbtime.Now(),
			ResourceID:               resource.ID,
			Name:                     prAgent.Name,
			AuthToken:                authToken,
			AuthInstanceID:           instanceID,
			Architecture:             prAgent.Architecture,
			EnvironmentVariables:     envJSON,
			Directory:                prAgent.Directory,
			OperatingSystem:          prAgent.OperatingSystem,
			ConnectionTimeoutSeconds: prAgent.GetConnectionTimeoutSeconds(),
			TroubleshootingURL:       prAgent.GetTroubleshootingUrl(),
			MOTDFile:                 prAgent.GetMotdFile(),
			DisplayApps:              convertDisplayApps(prAgent.GetDisplayApps()),
			InstanceMetadata:         pqtype.NullRawMessage{},
			ResourceMetadata:         pqtype.NullRawMessage{},
			DisplayOrder:             int32(prAgent.Order),
		})
		if err != nil {
			return xerrors.Errorf("insert agent: %w", err)
		}
		snapshot.WorkspaceAgents = append(snapshot.WorkspaceAgents, telemetry.ConvertWorkspaceAgent(dbAgent))

		for _, md := range prAgent.Metadata {
			p := database.InsertWorkspaceAgentMetadataParams{
				WorkspaceAgentID: agentID,
				DisplayName:      md.DisplayName,
				Script:           md.Script,
				Key:              md.Key,
				Timeout:          md.Timeout,
				Interval:         md.Interval,
				DisplayOrder:     int32(md.Order),
			}
			err := db.InsertWorkspaceAgentMetadata(ctx, p)
			if err != nil {
				return xerrors.Errorf("insert agent metadata: %w, params: %+v", err, p)
			}
		}

		logSourceIDs := make([]uuid.UUID, 0, len(prAgent.Scripts))
		logSourceDisplayNames := make([]string, 0, len(prAgent.Scripts))
		logSourceIcons := make([]string, 0, len(prAgent.Scripts))
		scriptLogPaths := make([]string, 0, len(prAgent.Scripts))
		scriptSources := make([]string, 0, len(prAgent.Scripts))
		scriptCron := make([]string, 0, len(prAgent.Scripts))
		scriptTimeout := make([]int32, 0, len(prAgent.Scripts))
		scriptStartBlocksLogin := make([]bool, 0, len(prAgent.Scripts))
		scriptRunOnStart := make([]bool, 0, len(prAgent.Scripts))
		scriptRunOnStop := make([]bool, 0, len(prAgent.Scripts))

		for _, script := range prAgent.Scripts {
			logSourceIDs = append(logSourceIDs, uuid.New())
			logSourceDisplayNames = append(logSourceDisplayNames, script.DisplayName)
			logSourceIcons = append(logSourceIcons, script.Icon)
			scriptLogPaths = append(scriptLogPaths, script.LogPath)
			scriptSources = append(scriptSources, script.Script)
			scriptCron = append(scriptCron, script.Cron)
			scriptTimeout = append(scriptTimeout, script.TimeoutSeconds)
			scriptStartBlocksLogin = append(scriptStartBlocksLogin, script.StartBlocksLogin)
			scriptRunOnStart = append(scriptRunOnStart, script.RunOnStart)
			scriptRunOnStop = append(scriptRunOnStop, script.RunOnStop)
		}

		_, err = db.InsertWorkspaceAgentLogSources(ctx, database.InsertWorkspaceAgentLogSourcesParams{
			WorkspaceAgentID: agentID,
			ID:               logSourceIDs,
			CreatedAt:        dbtime.Now(),
			DisplayName:      logSourceDisplayNames,
			Icon:             logSourceIcons,
		})
		if err != nil {
			return xerrors.Errorf("insert agent log sources: %w", err)
		}

		_, err = db.InsertWorkspaceAgentScripts(ctx, database.InsertWorkspaceAgentScriptsParams{
			WorkspaceAgentID: agentID,
			LogSourceID:      logSourceIDs,
			LogPath:          scriptLogPaths,
			CreatedAt:        dbtime.Now(),
			Script:           scriptSources,
			Cron:             scriptCron,
			TimeoutSeconds:   scriptTimeout,
			StartBlocksLogin: scriptStartBlocksLogin,
			RunOnStart:       scriptRunOnStart,
			RunOnStop:        scriptRunOnStop,
		})
		if err != nil {
			return xerrors.Errorf("insert agent scripts: %w", err)
		}

		for _, app := range prAgent.Apps {
			slug := app.Slug
			if slug == "" {
				return xerrors.Errorf("app must have a slug or name set")
			}
			if !provisioner.AppSlugRegex.MatchString(slug) {
				return xerrors.Errorf("app slug %q does not match regex %q", slug, provisioner.AppSlugRegex.String())
			}
			if _, exists := appSlugs[slug]; exists {
				return xerrors.Errorf("duplicate app slug, must be unique per template: %q", slug)
			}
			appSlugs[slug] = struct{}{}

			health := database.WorkspaceAppHealthDisabled
			if app.Healthcheck == nil {
				app.Healthcheck = &sdkproto.Healthcheck{}
			}
			if app.Healthcheck.Url != "" {
				health = database.WorkspaceAppHealthInitializing
			}

			sharingLevel := database.AppSharingLevelOwner
			switch app.SharingLevel {
			case sdkproto.AppSharingLevel_AUTHENTICATED:
				sharingLevel = database.AppSharingLevelAuthenticated
			case sdkproto.AppSharingLevel_PUBLIC:
				sharingLevel = database.AppSharingLevelPublic
			}

			dbApp, err := db.InsertWorkspaceApp(ctx, database.InsertWorkspaceAppParams{
				ID:          uuid.New(),
				CreatedAt:   dbtime.Now(),
				AgentID:     dbAgent.ID,
				Slug:        slug,
				DisplayName: app.DisplayName,
				Icon:        app.Icon,
				Command: sql.NullString{
					String: app.Command,
					Valid:  app.Command != "",
				},
				Url: sql.NullString{
					String: app.Url,
					Valid:  app.Url != "",
				},
				External:             app.External,
				Subdomain:            app.Subdomain,
				SharingLevel:         sharingLevel,
				HealthcheckUrl:       app.Healthcheck.Url,
				HealthcheckInterval:  app.Healthcheck.Interval,
				HealthcheckThreshold: app.Healthcheck.Threshold,
				Health:               health,
				DisplayOrder:         int32(app.Order),
			})
			if err != nil {
				return xerrors.Errorf("insert app: %w", err)
			}
			snapshot.WorkspaceApps = append(snapshot.WorkspaceApps, telemetry.ConvertWorkspaceApp(dbApp))
		}
	}

	arg := database.InsertWorkspaceResourceMetadataParams{
		WorkspaceResourceID: resource.ID,
		Key:                 []string{},
		Value:               []string{},
		Sensitive:           []bool{},
	}
	for _, metadatum := range protoResource.Metadata {
		if metadatum.IsNull {
			continue
		}
		arg.Key = append(arg.Key, metadatum.Key)
		arg.Value = append(arg.Value, metadatum.Value)
		arg.Sensitive = append(arg.Sensitive, metadatum.Sensitive)
	}
	_, err = db.InsertWorkspaceResourceMetadata(ctx, arg)
	if err != nil {
		return xerrors.Errorf("insert workspace resource metadata: %w", err)
	}

	return nil
}

func workspaceSessionTokenName(workspace database.Workspace) string {
	return fmt.Sprintf("%s_%s_session_token", workspace.OwnerID, workspace.ID)
}

func (s *server) regenerateSessionToken(ctx context.Context, user database.User, workspace database.Workspace) (string, error) {
	newkey, sessionToken, err := apikey.Generate(apikey.CreateParams{
		UserID:          user.ID,
		LoginType:       user.LoginType,
		TokenName:       workspaceSessionTokenName(workspace),
		DefaultLifetime: s.DeploymentValues.Sessions.DefaultDuration.Value(),
		LifetimeSeconds: int64(s.DeploymentValues.Sessions.MaximumTokenDuration.Value().Seconds()),
	})
	if err != nil {
		return "", xerrors.Errorf("generate API key: %w", err)
	}

	err = s.Database.InTx(func(tx database.Store) error {
		err := deleteSessionToken(ctx, tx, workspace)
		if err != nil {
			return xerrors.Errorf("delete session token: %w", err)
		}

		_, err = tx.InsertAPIKey(ctx, newkey)
		if err != nil {
			return xerrors.Errorf("insert API key: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return "", xerrors.Errorf("create API key: %w", err)
	}

	return sessionToken, nil
}

func deleteSessionToken(ctx context.Context, db database.Store, workspace database.Workspace) error {
	err := db.InTx(func(tx database.Store) error {
		key, err := tx.GetAPIKeyByName(ctx, database.GetAPIKeyByNameParams{
			UserID:    workspace.OwnerID,
			TokenName: workspaceSessionTokenName(workspace),
		})
		if err == nil {
			err = tx.DeleteAPIKeyByID(ctx, key.ID)
		}

		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("get api key by name: %w", err)
		}

		return nil
	}, nil)
	if err != nil {
		return xerrors.Errorf("in tx: %w", err)
	}

	return nil
}

// obtainOIDCAccessToken returns a valid OpenID Connect access token
// for the user if it's able to obtain one, otherwise it returns an empty string.
func obtainOIDCAccessToken(ctx context.Context, db database.Store, oidcConfig promoauth.OAuth2Config, userID uuid.UUID) (string, error) {
	link, err := db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
		UserID:    userID,
		LoginType: database.LoginTypeOIDC,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return "", xerrors.Errorf("get owner oidc link: %w", err)
	}

	if link.OAuthExpiry.Before(dbtime.Now()) && !link.OAuthExpiry.IsZero() && link.OAuthRefreshToken != "" {
		token, err := oidcConfig.TokenSource(ctx, &oauth2.Token{
			AccessToken:  link.OAuthAccessToken,
			RefreshToken: link.OAuthRefreshToken,
			Expiry:       link.OAuthExpiry,
		}).Token()
		if err != nil {
			// If OIDC fails to refresh, we return an empty string and don't fail.
			// There isn't a way to hard-opt in to OIDC from a template, so we don't
			// want to fail builds if users haven't authenticated for a while or something.
			return "", nil
		}
		link.OAuthAccessToken = token.AccessToken
		link.OAuthRefreshToken = token.RefreshToken
		link.OAuthExpiry = token.Expiry

		link, err = db.UpdateUserLink(ctx, database.UpdateUserLinkParams{
			UserID:                 userID,
			LoginType:              database.LoginTypeOIDC,
			OAuthAccessToken:       link.OAuthAccessToken,
			OAuthAccessTokenKeyID:  sql.NullString{}, // set by dbcrypt if required
			OAuthRefreshToken:      link.OAuthRefreshToken,
			OAuthRefreshTokenKeyID: sql.NullString{}, // set by dbcrypt if required
			OAuthExpiry:            link.OAuthExpiry,
			DebugContext:           link.DebugContext,
		})
		if err != nil {
			return "", xerrors.Errorf("update user link: %w", err)
		}
	}

	return link.OAuthAccessToken, nil
}

func convertLogLevel(logLevel sdkproto.LogLevel) (database.LogLevel, error) {
	switch logLevel {
	case sdkproto.LogLevel_TRACE:
		return database.LogLevelTrace, nil
	case sdkproto.LogLevel_DEBUG:
		return database.LogLevelDebug, nil
	case sdkproto.LogLevel_INFO:
		return database.LogLevelInfo, nil
	case sdkproto.LogLevel_WARN:
		return database.LogLevelWarn, nil
	case sdkproto.LogLevel_ERROR:
		return database.LogLevelError, nil
	default:
		return database.LogLevel(""), xerrors.Errorf("unknown log level: %d", logLevel)
	}
}

func convertLogSource(logSource proto.LogSource) (database.LogSource, error) {
	switch logSource {
	case proto.LogSource_PROVISIONER_DAEMON:
		return database.LogSourceProvisionerDaemon, nil
	case proto.LogSource_PROVISIONER:
		return database.LogSourceProvisioner, nil
	default:
		return database.LogSource(""), xerrors.Errorf("unknown log source: %d", logSource)
	}
}

func convertRichParameterValues(workspaceBuildParameters []database.WorkspaceBuildParameter) []*sdkproto.RichParameterValue {
	protoParameters := make([]*sdkproto.RichParameterValue, len(workspaceBuildParameters))
	for i, buildParameter := range workspaceBuildParameters {
		protoParameters[i] = &sdkproto.RichParameterValue{
			Name:  buildParameter.Name,
			Value: buildParameter.Value,
		}
	}
	return protoParameters
}

func convertVariableValues(variableValues []codersdk.VariableValue) []*sdkproto.VariableValue {
	protoVariableValues := make([]*sdkproto.VariableValue, len(variableValues))
	for i, variableValue := range variableValues {
		protoVariableValues[i] = &sdkproto.VariableValue{
			Name:      variableValue.Name,
			Value:     variableValue.Value,
			Sensitive: true, // Without the template variable schema we have to assume that every variable may be sensitive.
		}
	}
	return protoVariableValues
}

func convertWorkspaceTransition(transition database.WorkspaceTransition) (sdkproto.WorkspaceTransition, error) {
	switch transition {
	case database.WorkspaceTransitionStart:
		return sdkproto.WorkspaceTransition_START, nil
	case database.WorkspaceTransitionStop:
		return sdkproto.WorkspaceTransition_STOP, nil
	case database.WorkspaceTransitionDelete:
		return sdkproto.WorkspaceTransition_DESTROY, nil
	default:
		return 0, xerrors.Errorf("unrecognized transition: %q", transition)
	}
}

func auditActionFromTransition(transition database.WorkspaceTransition) database.AuditAction {
	switch transition {
	case database.WorkspaceTransitionStart:
		return database.AuditActionStart
	case database.WorkspaceTransitionStop:
		return database.AuditActionStop
	case database.WorkspaceTransitionDelete:
		return database.AuditActionDelete
	default:
		return database.AuditActionWrite
	}
}

type TemplateVersionImportJob struct {
	TemplateVersionID  uuid.UUID                `json:"template_version_id"`
	UserVariableValues []codersdk.VariableValue `json:"user_variable_values"`
}

// WorkspaceProvisionJob is the payload for the "workspace_provision" job type.
type WorkspaceProvisionJob struct {
	WorkspaceBuildID uuid.UUID `json:"workspace_build_id"`
	DryRun           bool      `json:"dry_run"`
	LogLevel         string    `json:"log_level,omitempty"`
}

// TemplateVersionDryRunJob is the payload for the "template_version_dry_run" job type.
type TemplateVersionDryRunJob struct {
	TemplateVersionID   uuid.UUID                          `json:"template_version_id"`
	WorkspaceName       string                             `json:"workspace_name"`
	RichParameterValues []database.WorkspaceBuildParameter `json:"rich_parameter_values"`
}

func asVariableValues(templateVariables []database.TemplateVersionVariable) []*sdkproto.VariableValue {
	var apiVariableValues []*sdkproto.VariableValue
	for _, v := range templateVariables {
		value := v.Value
		if value == "" && v.DefaultValue != "" {
			value = v.DefaultValue
		}

		if value != "" || v.Required {
			apiVariableValues = append(apiVariableValues, &sdkproto.VariableValue{
				Name:      v.Name,
				Value:     v.Value,
				Sensitive: v.Sensitive,
			})
		}
	}
	return apiVariableValues
}

func redactTemplateVariable(templateVariable *sdkproto.TemplateVariable) *sdkproto.TemplateVariable {
	if templateVariable == nil {
		return nil
	}
	maybeRedacted := &sdkproto.TemplateVariable{
		Name:         templateVariable.Name,
		Description:  templateVariable.Description,
		Type:         templateVariable.Type,
		DefaultValue: templateVariable.DefaultValue,
		Required:     templateVariable.Required,
		Sensitive:    templateVariable.Sensitive,
	}
	if maybeRedacted.Sensitive {
		maybeRedacted.DefaultValue = "*redacted*"
	}
	return maybeRedacted
}

func convertDisplayApps(apps *sdkproto.DisplayApps) []database.DisplayApp {
	// This shouldn't happen but let's avoid panicking. It also makes
	// writing tests a bit easier.
	if apps == nil {
		return nil
	}
	dapps := make([]database.DisplayApp, 0, 5)
	if apps.Vscode {
		dapps = append(dapps, database.DisplayAppVscode)
	}
	if apps.VscodeInsiders {
		dapps = append(dapps, database.DisplayAppVscodeInsiders)
	}
	if apps.SshHelper {
		dapps = append(dapps, database.DisplayAppSSHHelper)
	}
	if apps.PortForwardingHelper {
		dapps = append(dapps, database.DisplayAppPortForwardingHelper)
	}
	if apps.WebTerminal {
		dapps = append(dapps, database.DisplayAppWebTerminal)
	}
	return dapps
}
