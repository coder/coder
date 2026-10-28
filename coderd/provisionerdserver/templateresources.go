package provisionerdserver

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"
	protobuf "google.golang.org/protobuf/proto"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfstate"
)

const templateResourcesStateUnknown = "TEMPLATE_RESOURCES_STATE_UNKNOWN"

// TemplateResourcesJob identifies the immutable run for a shared resource job.
// The database job ID is authoritative for looking up the run.
type TemplateResourcesJob struct {
	TemplateResourceRunID uuid.UUID `json:"template_resource_run_id"`
}

func validateTemplateResourceReservation(job database.ProvisionerJob, run database.TemplateResourceRun, state database.TemplateResourceState) error {
	if run.JobID != job.ID || run.OrganizationID != job.OrganizationID || state.OrganizationID != job.OrganizationID || run.FoundationID != state.FoundationID {
		return xerrors.New("shared resource run does not match job and Foundation")
	}
	if !state.ActiveRunID.Valid || state.ActiveRunID.UUID != run.ID || state.Status == "unknown" {
		return xerrors.New("shared resource reservation is stale or quarantined")
	}
	return nil
}

func (s *server) acquireTemplateResourcesJob(ctx context.Context, acquired database.ProvisionerJob) (*proto.AcquiredJob_TemplateResources, error) {
	var payload *proto.AcquiredJob_TemplateResources
	err := s.Database.InTx(func(db database.Store) error {
		// Workers and the reaper lock the job before Foundation-owned state.
		job, err := db.GetProvisionerJobByIDWithLock(ctx, acquired.ID)
		if err != nil {
			return xerrors.Errorf("lock shared resources job: %w", err)
		}
		if !job.WorkerID.Valid || job.WorkerID.UUID != s.ID {
			return xerrors.New("you don't own this job")
		}
		if job.Type != database.ProvisionerJobTypeTemplateResources || job.CompletedAt.Valid || job.CanceledAt.Valid {
			return xerrors.New("shared resources job is no longer runnable")
		}
		run, err := db.GetTemplateResourceRunByJobID(ctx, job.ID)
		if err != nil {
			return xerrors.Errorf("get shared resource run: %w", err)
		}
		var input TemplateResourcesJob
		if err := json.Unmarshal(job.Input, &input); err != nil {
			return xerrors.Errorf("decode shared resources job input: %w", err)
		}
		if input.TemplateResourceRunID != run.ID {
			return xerrors.New("shared resources job input does not match its run")
		}
		state, err := db.GetTemplateResourceStateForUpdate(ctx, run.FoundationID)
		if err != nil {
			return xerrors.Errorf("lock shared resource state: %w", err)
		}
		if err := validateTemplateResourceReservation(job, run, state); err != nil {
			return err
		}
		revision, err := db.GetTemplateResourceRevisionByID(ctx, run.RevisionID)
		if err != nil {
			return xerrors.Errorf("get frozen shared resource revision: %w", err)
		}
		if revision.FoundationID != run.FoundationID || revision.OrganizationID != job.OrganizationID || revision.FileID != job.FileID || acquired.FileID != job.FileID {
			return xerrors.New("shared resource revision does not match job archive")
		}
		foundation, err := db.GetFoundationByID(ctx, run.FoundationID)
		if err != nil {
			return xerrors.Errorf("get Foundation: %w", err)
		}
		if foundation.OrganizationID != job.OrganizationID || foundation.Deleted {
			return xerrors.New("shared resources job does not match an active Foundation")
		}
		operation := proto.TemplateResourceOperation_TEMPLATE_RESOURCE_PLAN
		transition := sdkproto.WorkspaceTransition_START
		switch run.Action {
		case "plan":
		case "apply":
			operation = proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY
		case "destroy":
			operation = proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY
			transition = sdkproto.WorkspaceTransition_DESTROY
			if !state.CheckpointRevisionID.Valid || state.CheckpointRevisionID.UUID != run.RevisionID || len(state.State) == 0 {
				return xerrors.New("destroy does not match state-producing revision")
			}
		default:
			return xerrors.New("invalid shared resource action")
		}
		metadata := &sdkproto.Metadata{FoundationId: foundation.ID.String(), FoundationName: foundation.Name, WorkspaceTransition: transition, TemplateOutputs: "{}"}
		if foundation.TemplateID.Valid {
			template, err := db.GetTemplateByID(ctx, foundation.TemplateID.UUID)
			if err != nil {
				return err
			}
			metadata.TemplateId, metadata.TemplateName = template.ID.String(), template.Name
			metadata.FoundationTemplateIds = []string{template.ID.String()}
		}
		if s.AccessURL != nil {
			metadata.CoderUrl = s.AccessURL.String()
		}
		payload = &proto.AcquiredJob_TemplateResources{
			Operation: operation, State: state.State, Metadata: metadata,
			RunId: protobuf.String(run.ID.String()), TemplateId: protobuf.String(metadata.TemplateId), FoundationId: foundation.ID.String(), RevisionId: protobuf.String(revision.ID.String()),
		}
		return nil
	}, nil)
	return payload, err
}

type templateResourcesJobResult struct {
	success     bool
	state       []byte
	disposition proto.StateDisposition
	plan        []byte
	timings     []*sdkproto.Timing
	error       string
	errorCode   string
}

func (s *server) completeTemplateResourcesJob(ctx context.Context, jobID uuid.UUID, completed *proto.CompletedJob) (*proto.Empty, error) {
	payload := completed.GetTemplateResources()
	if payload == nil {
		return nil, xerrors.New("shared resources completion payload must be provided")
	}
	if protobuf.Size(completed) > drpcsdk.MaxMessageSize {
		return nil, xerrors.New("shared resources completion exceeds maximum message size")
	}
	if err := s.finalizeTemplateResourcesJob(ctx, jobID, templateResourcesJobResult{
		success: true, state: payload.State, disposition: payload.StateDisposition, plan: payload.Plan, timings: payload.Timings,
	}); err != nil {
		return nil, err
	}
	return &proto.Empty{}, nil
}

func (s *server) failTemplateResourcesJob(ctx context.Context, jobID uuid.UUID, failed *proto.FailedJob) (*proto.Empty, error) {
	if protobuf.Size(failed) > drpcsdk.MaxMessageSize {
		return nil, xerrors.New("shared resources failure exceeds maximum message size")
	}
	result := templateResourcesJobResult{error: failed.Error, errorCode: failed.ErrorCode}
	if result.error == "" {
		result.error = "shared resources job failed"
	}
	if payload := failed.GetTemplateResources(); payload != nil {
		result.state, result.disposition, result.timings = payload.State, payload.StateDisposition, payload.Timings
	} else if failed.Type != nil {
		return nil, xerrors.New("failure payload does not match stored shared resources job type")
	}
	if err := s.finalizeTemplateResourcesJob(ctx, jobID, result); err != nil {
		return nil, err
	}
	return &proto.Empty{}, nil
}

func (s *server) finalizeTemplateResourcesJob(ctx context.Context, jobID uuid.UUID, result templateResourcesJobResult) error {
	var finalized database.ProvisionerJob
	err := s.Database.InTx(func(db database.Store) error {
		job, err := db.GetProvisionerJobByIDWithLock(ctx, jobID)
		if err != nil {
			return xerrors.Errorf("lock shared resources job: %w", err)
		}
		if job.Type != database.ProvisionerJobTypeTemplateResources {
			return xerrors.New("stored job is not a shared resources job")
		}
		if !job.WorkerID.Valid || job.WorkerID.UUID != s.ID {
			return xerrors.New("you don't own this job")
		}
		// No result may revise a terminal outcome, especially a reaped job
		// whose infrastructure mutation may still be running remotely.
		if job.CompletedAt.Valid {
			return xerrors.New("shared resources job is complete; late results are rejected")
		}
		run, err := db.GetTemplateResourceRunByJobID(ctx, jobID)
		if err != nil {
			return xerrors.Errorf("get shared resource run: %w", err)
		}
		state, err := db.GetTemplateResourceStateForUpdate(ctx, run.FoundationID)
		if err != nil {
			return xerrors.Errorf("lock shared resource state: %w", err)
		}
		if err := validateTemplateResourceReservation(job, run, state); err != nil {
			return err
		}
		if run.Action != "plan" && run.Action != "apply" && run.Action != "destroy" {
			return xerrors.New("invalid shared resource action")
		}
		var outputs json.RawMessage
		if run.Action == "plan" {
			if result.disposition != proto.StateDisposition_UNCHANGED && (result.success || result.disposition != proto.StateDisposition_UNKNOWN) {
				return xerrors.New("shared resource plan state must be unchanged")
			}
			if len(result.state) != 0 || result.disposition == proto.StateDisposition_RETURNED {
				return xerrors.New("shared resource plan must not return state")
			}
			if len(result.plan) > 0 && !json.Valid(result.plan) {
				return xerrors.New("shared resource plan preview must be JSON")
			}
			if result.success {
				preview, err := json.Marshal(provisionersdk.FoundationPlanPreview(result.plan))
				if err != nil {
					return xerrors.Errorf("encode Foundation plan preview: %w", err)
				}
				if err := db.UpdateTemplateResourceRunPlanPreview(ctx, database.UpdateTemplateResourceRunPlanPreviewParams{ID: run.ID, PlanPreview: pqtype.NullRawMessage{RawMessage: preview, Valid: true}}); err != nil {
					return xerrors.Errorf("save Foundation plan preview: %w", err)
				}
			}
		} else {
			// RETURNED attests that execution terminated with an authoritative
			// checkpoint, even when Terraform reported a partial apply failure.
			// Cancellation cannot make that promise: a remote worker may still
			// be mutating infrastructure after the request was canceled.
			switch {
			case job.CanceledAt.Valid && result.disposition != proto.StateDisposition_UNCHANGED:
				err = xerrors.New("shared resource mutation was canceled")
			case result.disposition == proto.StateDisposition_RETURNED:
				err = tfstate.ValidateState(result.state)
				if err == nil && run.Action == "destroy" && (!state.CheckpointRevisionID.Valid || state.CheckpointRevisionID.UUID != run.RevisionID) {
					err = xerrors.New("destroy does not match state-producing revision")
				}
			case result.success:
				err = xerrors.New("successful shared resource mutation must return authoritative state")
			}
			if err != nil {
				result.success = false
				result.disposition = proto.StateDisposition_UNKNOWN
				result.error = err.Error()
			} else if result.success {
				outputs, err = templateResourceOutputs(result.state, run.Action)
				if err == nil && run.Action == "apply" {
					outputs, err = stampFoundationOutputSnapshot(outputs, run.ID)
				}
				if err != nil {
					// An invalid output contract does not invalidate the state
					// checkpoint or imply that the worker is still executing.
					result.success = false
					result.error = err.Error()
				}
			}
		}
		now := s.timeNow()
		unchanged := result.disposition == proto.StateDisposition_UNCHANGED && len(result.state) == 0
		unknown := run.Action != "plan" && !result.success && !unchanged && result.disposition != proto.StateDisposition_RETURNED
		if unknown {
			checkpoint := state.State
			if len(result.state) > 0 {
				checkpoint = result.state
			}
			_, err := db.UpdateTemplateResourceState(ctx, database.UpdateTemplateResourceStateParams{
				FoundationID: state.FoundationID, RunID: state.ActiveRunID, State: checkpoint, Status: "unknown", UpdatedAt: now,
			})
			if err != nil {
				return xerrors.Errorf("quarantine shared resource state: %w", err)
			}
			result.error += "; shared resource infrastructure outcome is unknown; reconciliation required"
			result.errorCode = templateResourcesStateUnknown
		} else {
			if run.Action != "plan" && result.disposition == proto.StateDisposition_RETURNED {
				state.State = result.state
				state.CheckpointRevisionID = uuid.NullUUID{UUID: run.RevisionID, Valid: true}
				state.Status = "failed"
			}
			if result.success && run.Action == "apply" {
				state.Outputs, state.Status = outputs, "applied"
				state.AppliedRevisionID = uuid.NullUUID{UUID: run.RevisionID, Valid: true}
			} else if result.success && run.Action == "destroy" {
				state.Outputs, state.Status = json.RawMessage(`{}`), "destroyed"
				state.AppliedRevisionID = uuid.NullUUID{}
				state.CheckpointRevisionID = uuid.NullUUID{}
			}
			// Plans and pre-apply failures leave the checkpoint and published
			// snapshot intact. Completed partial mutations change only the
			// checkpoint, so normal recovery can plan against actual state.
			_, err := db.CompleteTemplateResourceRun(ctx, database.CompleteTemplateResourceRunParams{
				FoundationID: state.FoundationID, RunID: state.ActiveRunID, Status: state.Status, State: state.State,
				Outputs: state.Outputs, AppliedRevisionID: state.AppliedRevisionID,
				CheckpointRevisionID: state.CheckpointRevisionID, UpdatedAt: now,
			})
			if err != nil {
				return xerrors.Errorf("complete shared resource run: %w", err)
			}
		}
		if err := insertNativeJobTimings(ctx, db, jobID, result.timings); err != nil {
			return err
		}
		job.CompletedAt = sql.NullTime{Time: now, Valid: true}
		job.UpdatedAt = now
		job.Error = sql.NullString{String: result.error, Valid: result.error != ""}
		job.ErrorCode = sql.NullString{String: result.errorCode, Valid: result.errorCode != ""}
		if err := db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: jobID, CompletedAt: job.CompletedAt, UpdatedAt: now, Error: job.Error, ErrorCode: job.ErrorCode,
		}); err != nil {
			return xerrors.Errorf("complete shared resources job: %w", err)
		}
		finalized = job
		return nil
	}, nil)
	if err != nil {
		return err
	}
	defer s.jobFinished(jobID)
	if s.Telemetry != nil {
		s.Telemetry.Report(&telemetry.Snapshot{ProvisionerJobs: []telemetry.ProvisionerJob{telemetry.ConvertProvisionerJob(finalized)}})
	}
	data, err := json.Marshal(provisionersdk.ProvisionerJobLogsNotifyMessage{EndOfLogs: true})
	if err != nil {
		return xerrors.Errorf("marshal shared resources end of logs: %w", err)
	}
	if err := s.Pubsub.Publish(provisionersdk.ProvisionerJobLogsNotifyChannel(jobID), data); err != nil {
		s.Logger.Warn(ctx, "publish shared resources end of logs", slog.Error(err))
		return xerrors.Errorf("publish shared resources end of logs: %w", err)
	}
	return nil
}

func templateResourceRevisionString(revision uuid.NullUUID) string {
	if revision.Valid {
		return revision.UUID.String()
	}
	return ""
}

// validateTemplateOutputsWorker prevents older protocols from silently dropping
// output metadata, including when resources were first applied after enqueue.
func (s *server) validateTemplateOutputsWorker(outputs, revisionID string) error {
	if (outputs == "" || outputs == "{}") && revisionID == "" {
		return nil
	}
	tags := provisionersdk.TemplateResourcesProvisionerTags(s.Tags, s.apiVersion)
	if tags[provisionersdk.TagTemplateResources] != provisionersdk.TemplateResourcesCapabilityVersion {
		return xerrors.Errorf("shared resource outputs require a provisioner with API v1.25 or later and %s=%s capability", provisionersdk.TagTemplateResources, provisionersdk.TemplateResourcesCapabilityVersion)
	}
	return nil
}

// templateValidationMetadata uses the last successful outputs for validation.
// Workspace jobs must instead use the snapshot stored on their build.
func (s *server) templateValidationMetadata(ctx context.Context, templateID uuid.NullUUID, metadata *sdkproto.Metadata) error {
	metadata.TemplateOutputs = "{}"
	if !templateID.Valid {
		return nil
	}
	template, err := s.Database.GetTemplateByID(ctx, templateID.UUID)
	if err != nil {
		return xerrors.Errorf("get validation template: %w", err)
	}
	metadata.TemplateId, metadata.TemplateName = template.ID.String(), template.Name
	foundation, err := s.Database.GetFoundationByTemplateID(ctx, templateID)
	if xerrors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	metadata.FoundationId, metadata.FoundationName = foundation.ID.String(), foundation.Name
	metadata.FoundationTemplateIds = []string{template.ID.String()}
	state, err := s.Database.GetTemplateResourceState(ctx, foundation.ID)
	if xerrors.Is(err, sql.ErrNoRows) {
		return s.validateTemplateOutputsWorker(metadata.TemplateOutputs, metadata.FoundationId)
	}
	if err != nil {
		return xerrors.Errorf("get shared resource validation outputs: %w", err)
	}
	if state.AppliedRevisionID.Valid {
		if err := populateFoundationOutputMetadata(state.Outputs, metadata); err != nil {
			return err
		}
		metadata.TemplateResourceRevisionId = state.AppliedRevisionID.UUID.String()
	}
	return s.validateTemplateOutputsWorker(metadata.TemplateOutputs, metadata.FoundationId)
}

// workspaceFoundationMetadata resolves historical identity through the pinned revision,
// never through the template's current association.
func (s *server) workspaceFoundationMetadata(ctx context.Context, revisionID uuid.NullUUID, metadata *sdkproto.Metadata) error {
	if !revisionID.Valid {
		// Only a start without a published snapshot may use the current
		// binding. Cleanup must never inherit a newly attached Foundation.
		if metadata.WorkspaceTransition != sdkproto.WorkspaceTransition_START || metadata.TemplateId == "" {
			return nil
		}
		templateID, err := uuid.Parse(metadata.TemplateId)
		if err != nil {
			return xerrors.Errorf("parse Foundation template ID: %w", err)
		}
		foundation, err := s.Database.GetFoundationByTemplateID(ctx, uuid.NullUUID{UUID: templateID, Valid: true})
		if xerrors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("get start Foundation: %w", err)
		}
		metadata.FoundationId, metadata.FoundationName = foundation.ID.String(), foundation.Name
		metadata.FoundationTemplateIds = []string{templateID.String()}
		return s.validateTemplateOutputsWorker(metadata.TemplateOutputs, metadata.FoundationId)
	}
	revision, err := s.Database.GetTemplateResourceRevisionByID(ctx, revisionID.UUID)
	if err != nil {
		return xerrors.Errorf("get pinned Foundation revision: %w", err)
	}
	f, err := s.Database.GetFoundationByID(ctx, revision.FoundationID)
	if err != nil {
		return xerrors.Errorf("get pinned Foundation: %w", err)
	}
	metadata.FoundationId, metadata.FoundationName = f.ID.String(), f.Name
	if f.TemplateID.Valid {
		metadata.FoundationTemplateIds = []string{f.TemplateID.UUID.String()}
	}
	return s.validateTemplateOutputsWorker(metadata.TemplateOutputs, metadata.FoundationId)
}
