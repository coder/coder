package provisionerdserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	protobuf "google.golang.org/protobuf/proto"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/networkpolicy"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfstate"
)

// NetworkPolicyJob identifies the immutable run for a native policy job.
// The database job ID, not this input, is authoritative for finding the run.
type NetworkPolicyJob struct {
	NetworkPolicyRunID uuid.UUID `json:"network_policy_run_id"`
}

func (s *server) acquireNetworkPolicyJob(ctx context.Context, acquired database.ProvisionerJob) (*proto.AcquiredJob_NetworkPolicy, error) {
	var payload *proto.AcquiredJob_NetworkPolicy
	err := s.Database.InTx(func(db database.Store) error {
		// Match the finalizer and reaper lock order: job, policy, target.
		job, err := db.GetProvisionerJobByIDWithLock(ctx, acquired.ID)
		if err != nil {
			return xerrors.Errorf("lock network policy job: %w", err)
		}
		if !job.WorkerID.Valid || job.WorkerID.UUID != s.ID {
			return xerrors.New("you don't own this job")
		}
		if job.Type != database.ProvisionerJobTypeNetworkPolicy || job.CompletedAt.Valid || job.CanceledAt.Valid {
			return xerrors.New("network policy job is no longer runnable")
		}
		run, err := db.GetNetworkPolicyRunByJobID(ctx, job.ID)
		if err != nil {
			return xerrors.Errorf("get network policy run: %w", err)
		}
		var input NetworkPolicyJob
		if err := json.Unmarshal(job.Input, &input); err != nil {
			return xerrors.Errorf("decode network policy job input: %w", err)
		}
		if input.NetworkPolicyRunID != run.ID {
			return xerrors.New("network policy job input does not match its run")
		}
		policy, err := db.GetNetworkPolicyByIDForUpdate(ctx, run.PolicyID)
		if err != nil {
			return xerrors.Errorf("lock network policy: %w", err)
		}
		target, err := db.GetNetworkPolicyTargetByIDForUpdate(ctx, run.TargetID)
		if err != nil {
			return xerrors.Errorf("lock network policy target: %w", err)
		}
		if err := validateNetworkPolicyReservation(job, run, target); err != nil {
			return err
		}
		revision, err := db.GetNetworkPolicyRevisionByID(ctx, run.RevisionID)
		if err != nil {
			return xerrors.Errorf("get frozen network policy revision: %w", err)
		}
		if revision.PolicyID != run.PolicyID || revision.OrganizationID != job.OrganizationID || revision.FileID != job.FileID || acquired.FileID != job.FileID {
			return xerrors.New("network policy revision does not match job archive")
		}
		if policy.DeletedAt.Valid || (policy.DeletionRequestedAt.Valid && run.Operation != "destroy") || (run.Operation == "destroy" && !policy.DeletionRequestedAt.Valid) {
			return xerrors.New("network policy deletion lifecycle does not permit this operation")
		}
		operation := proto.NetworkPolicyOperation_PLAN
		transition := sdkproto.WorkspaceTransition_START
		switch run.Operation {
		case "plan":
		case "apply":
			operation = proto.NetworkPolicyOperation_APPLY
		case "destroy":
			operation = proto.NetworkPolicyOperation_DESTROY
			transition = sdkproto.WorkspaceTransition_DESTROY
			snapshot := target
			snapshot.ActiveJobID = uuid.NullUUID{}
			source, err := networkpolicy.DestroyProvenance(ctx, db, snapshot)
			if err != nil {
				return err
			}
			if source == nil || source.RevisionID != run.RevisionID || !slices.Equal(source.TemplateIDs, run.TemplateIDs) {
				return xerrors.New("destroy does not match state-producing configuration")
			}
		default:
			return xerrors.New("invalid network policy operation")
		}
		templateIDs := make([]string, len(run.TemplateIDs))
		for i, id := range run.TemplateIDs {
			templateIDs[i] = id.String()
		}
		templateIDsJSON, err := json.Marshal(templateIDs)
		if err != nil {
			return xerrors.Errorf("encode network policy template IDs: %w", err)
		}
		payload = &proto.AcquiredJob_NetworkPolicy{
			Operation: operation,
			State:     target.State,
			VariableValues: []*sdkproto.VariableValue{
				{Name: "policy_id", Value: run.PolicyID.String()},
				{Name: "namespace", Value: target.Namespace},
				{Name: "template_ids", Value: string(templateIDsJSON)},
				{Name: "kubeconfig_context", Value: target.Context},
			},
			Metadata:   &sdkproto.Metadata{WorkspaceTransition: transition},
			RunId:      protobuf.String(run.ID.String()),
			TargetId:   protobuf.String(target.ID.String()),
			RevisionId: protobuf.String(revision.ID.String()),
		}
		return nil
	}, nil)
	return payload, err
}

func validateNetworkPolicyReservation(job database.ProvisionerJob, run database.NetworkPolicyRun, target database.NetworkPolicyTarget) error {
	if run.JobID != job.ID || run.OrganizationID != job.OrganizationID || target.OrganizationID != job.OrganizationID || target.PolicyID != run.PolicyID || target.ID != run.TargetID {
		return xerrors.New("network policy run does not match job and target")
	}
	if !target.ActiveJobID.Valid || target.ActiveJobID.UUID != job.ID || target.Generation != run.Generation || !target.DesiredRevisionID.Valid || target.DesiredRevisionID.UUID != run.RevisionID {
		return xerrors.New("network policy target reservation is stale")
	}
	return nil
}

type networkPolicyJobResult struct {
	success     bool
	state       []byte
	disposition proto.StateDisposition
	plan        []byte
	timings     []*sdkproto.Timing
	error       string
	errorCode   string
}

func (s *server) completeNetworkPolicyJob(ctx context.Context, jobID uuid.UUID, completed *proto.CompletedJob) (*proto.Empty, error) {
	payload := completed.GetNetworkPolicy()
	if payload == nil {
		return nil, xerrors.New("network policy completion payload must be provided")
	}
	if protobuf.Size(completed) > drpcsdk.MaxMessageSize {
		return nil, xerrors.New("network policy completion exceeds maximum message size")
	}
	err := s.finalizeNetworkPolicyJob(ctx, jobID, networkPolicyJobResult{
		success:     true,
		state:       payload.State,
		disposition: payload.StateDisposition,
		plan:        payload.Plan,
		timings:     payload.Timings,
	})
	if err != nil {
		return nil, err
	}
	return &proto.Empty{}, nil
}

func (s *server) failNetworkPolicyJob(ctx context.Context, jobID uuid.UUID, failed *proto.FailedJob) (*proto.Empty, error) {
	if protobuf.Size(failed) > drpcsdk.MaxMessageSize {
		return nil, xerrors.New("network policy failure exceeds maximum message size")
	}
	result := networkPolicyJobResult{error: failed.Error, errorCode: failed.ErrorCode}
	if result.error == "" {
		result.error = "network policy job failed"
	}
	if payload := failed.GetNetworkPolicy(); payload != nil {
		result.state = payload.State
		result.disposition = payload.StateDisposition
		result.timings = payload.Timings
	} else if failed.Type != nil {
		return nil, xerrors.New("failure payload does not match stored network policy job type")
	}
	if err := s.finalizeNetworkPolicyJob(ctx, jobID, result); err != nil {
		return nil, err
	}
	return &proto.Empty{}, nil
}

// finalizeNetworkPolicyJob commits the outcome and state together. An unknown
// apply outcome completes the job but retains the reservation: retrying against
// the previous state could orphan resources created by the lost apply.
func (s *server) finalizeNetworkPolicyJob(ctx context.Context, jobID uuid.UUID, result networkPolicyJobResult) error {
	var finalized database.ProvisionerJob
	err := s.Database.InTx(func(db database.Store) error {
		job, err := db.GetProvisionerJobByIDWithLock(ctx, jobID)
		if err != nil {
			return xerrors.Errorf("lock network policy job: %w", err)
		}
		if job.Type != database.ProvisionerJobTypeNetworkPolicy {
			return xerrors.New("stored job is not a network policy job")
		}
		if !job.WorkerID.Valid || job.WorkerID.UUID != s.ID {
			return xerrors.New("you don't own this job")
		}
		run, err := db.GetNetworkPolicyRunByJobID(ctx, jobID)
		if err != nil {
			return xerrors.Errorf("get network policy run: %w", err)
		}
		policy, err := db.GetNetworkPolicyByIDForUpdate(ctx, run.PolicyID)
		if err != nil {
			return xerrors.Errorf("lock network policy: %w", err)
		}
		target, err := db.GetNetworkPolicyTargetByIDForUpdate(ctx, run.TargetID)
		if err != nil {
			return xerrors.Errorf("lock network policy target: %w", err)
		}
		if job.CompletedAt.Valid {
			// A reaped or unknown-outcome job is not a normal RPC retry.
			// Never let a late result clear its quarantine or overwrite state.
			if job.ErrorCode.String == networkpolicy.ErrorCodeStateUnknown || (target.ActiveJobID.Valid && target.ActiveJobID.UUID == jobID) {
				return xerrors.New("network policy job requires reconciliation; late results are rejected")
			}
			return nil
		}
		if err := validateNetworkPolicyReservation(job, run, target); err != nil {
			return err
		}
		if run.Operation == "destroy" && !policy.DeletionRequestedAt.Valid {
			return xerrors.New("network policy deletion has not been requested")
		}
		// A nominally successful destroy with invalid or nonempty state is an
		// uncertain infrastructure outcome, not a retryable pre-apply failure.
		if run.Operation == "destroy" && result.success && result.disposition == proto.StateDisposition_RETURNED {
			if err := tfstate.ValidateNetworkPolicyDestroyState(result.state); err != nil {
				result.success = false
				result.disposition = proto.StateDisposition_UNKNOWN
				result.error = err.Error()
			}
		}
		if err := validateNetworkPolicyResult(run.Operation, result); err != nil {
			return err
		}
		now := s.timeNow()
		unknown := (run.Operation == "apply" || run.Operation == "destroy") && result.disposition == proto.StateDisposition_UNKNOWN
		if unknown {
			// Even a failed apply can return an older checkpoint, not its
			// authoritative final state. Persist available bytes for recovery
			// without releasing the reservation or claiming the revision.
			if len(result.state) > 0 {
				if _, err := db.UpdateNetworkPolicyTargetState(ctx, database.UpdateNetworkPolicyTargetStateParams{
					ID: target.ID, JobID: target.ActiveJobID, Generation: run.Generation,
					State: result.state, UpdatedAt: now,
				}); err != nil {
					return xerrors.Errorf("save uncertain network policy state checkpoint: %w", err)
				}
			}
			result.error += "; network policy infrastructure outcome is unknown; reconciliation required"
			result.errorCode = networkpolicy.ErrorCodeStateUnknown
		} else {
			if !result.success {
				// Startup fallback needs affirmative proof that an intervening
				// failed run left the last applied state unchanged.
				result.errorCode = networkpolicy.ErrorCodeStateUnchanged
			}
			state := target.State
			stateRunID := target.StateRunID
			destroyedAt := target.DestroyedAt
			appliedRevision := target.AppliedRevisionID
			drifted := target.Drifted
			status := "failed"
			if job.CanceledAt.Valid {
				status = "canceled"
			} else if result.success {
				status = "succeeded"
			}
			if run.Operation == "plan" && result.success && len(result.plan) > 0 {
				var preview struct {
					ResourceDrift []json.RawMessage `json:"resource_drift"`
				}
				if err := json.Unmarshal(result.plan, &preview); err != nil {
					return xerrors.Errorf("read network policy drift preview: %w", err)
				}
				// A preview can establish observed drift, not that the
				// cluster is safe. Only a successful apply clears this flag.
				drifted = drifted || len(preview.ResourceDrift) > 0
			}
			if run.Operation == "apply" && result.disposition == proto.StateDisposition_RETURNED {
				state = result.state
				// Even a canceled successful apply establishes the exact source
				// of its returned state, without claiming startup readiness.
				stateRunID = uuid.NullUUID{UUID: run.ID, Valid: true}
				drifted = true
				if result.success && !job.CanceledAt.Valid {
					appliedRevision = uuid.NullUUID{UUID: run.RevisionID, Valid: true}
					drifted = false
				}
			}
			if run.Operation == "destroy" && result.success && result.disposition == proto.StateDisposition_RETURNED {
				state = result.state
				destroyedAt = sql.NullTime{Time: now, Valid: true}
				drifted = false
			}
			_, err := db.CompleteNetworkPolicyTargetJob(ctx, database.CompleteNetworkPolicyTargetJobParams{
				ID: target.ID, JobID: uuid.NullUUID{UUID: job.ID, Valid: true}, Generation: run.Generation,
				State: state, AppliedRevisionID: appliedRevision, StateRunID: stateRunID, DestroyedAt: destroyedAt,
				Status: status, Drifted: drifted, LastError: result.error, UpdatedAt: now,
			})
			if err != nil {
				return xerrors.Errorf("complete network policy target: %w", err)
			}
			if destroyedAt.Valid {
				// The policy lock serializes final targets across separate jobs.
				// Never soft-delete before their state and proof are committed.
				if _, err := db.CompleteNetworkPolicyDeletion(ctx, database.CompleteNetworkPolicyDeletionParams{ID: policy.ID, UpdatedAt: now}); err != nil {
					return xerrors.Errorf("complete network policy deletion: %w", err)
				}
			}
		}
		if err := insertNetworkPolicyTimings(ctx, db, job.ID, result.timings); err != nil {
			return err
		}
		job.CompletedAt = sql.NullTime{Time: now, Valid: true}
		job.UpdatedAt = now
		job.Error = sql.NullString{String: result.error, Valid: result.error != ""}
		job.ErrorCode = sql.NullString{String: result.errorCode, Valid: result.errorCode != ""}
		if err := db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: job.ID, UpdatedAt: now, CompletedAt: job.CompletedAt, Error: job.Error, ErrorCode: job.ErrorCode,
		}); err != nil {
			return xerrors.Errorf("complete network policy job: %w", err)
		}
		finalized = job
		return nil
	}, nil)
	if err != nil {
		return err
	}
	defer s.jobFinished(jobID)
	if finalized.ID != uuid.Nil && s.Telemetry != nil {
		s.Telemetry.Report(&telemetry.Snapshot{ProvisionerJobs: []telemetry.ProvisionerJob{telemetry.ConvertProvisionerJob(finalized)}})
	}
	data, err := json.Marshal(provisionersdk.ProvisionerJobLogsNotifyMessage{EndOfLogs: true})
	if err != nil {
		return xerrors.Errorf("marshal network policy end of logs: %w", err)
	}
	if err := s.Pubsub.Publish(provisionersdk.ProvisionerJobLogsNotifyChannel(jobID), data); err != nil {
		s.Logger.Warn(ctx, "publish network policy end of logs", slog.Error(err))
		return xerrors.Errorf("publish network policy end of logs: %w", err)
	}
	return nil
}

func validateNetworkPolicyResult(operation string, result networkPolicyJobResult) error {
	// Plans are JSON previews only. No binary executable plan is stored or
	// accepted as input to an apply; the daemon replans in its own session.
	if len(result.plan) > 0 && !json.Valid(result.plan) {
		return xerrors.New("network policy plan preview must be JSON")
	}
	switch operation {
	case "plan":
		if len(result.state) != 0 || result.disposition == proto.StateDisposition_RETURNED {
			return xerrors.New("network policy plan must not return state")
		}
		if result.disposition != proto.StateDisposition_UNCHANGED && (result.disposition != proto.StateDisposition_UNKNOWN || result.success) {
			return xerrors.New("network policy plan state must be unchanged")
		}
	case "apply", "destroy":
		switch result.disposition {
		case proto.StateDisposition_RETURNED:
			if !result.success || len(result.state) == 0 {
				return xerrors.New("authoritative network policy state requires successful apply and nonempty state")
			}
		case proto.StateDisposition_UNCHANGED:
			if result.success || len(result.state) != 0 {
				return xerrors.New("unchanged network policy state requires a pre-apply failure with no state")
			}
		case proto.StateDisposition_UNKNOWN:
			if result.success {
				return xerrors.New("successful network policy apply must return authoritative state")
			}
		default:
			return xerrors.New("invalid network policy state disposition")
		}
	default:
		return xerrors.New("invalid network policy operation")
	}
	return nil
}

func insertNetworkPolicyTimings(ctx context.Context, db database.Store, jobID uuid.UUID, timings []*sdkproto.Timing) error {
	//nolint:exhaustruct // Validated timing entries are appended below.
	params := database.InsertProvisionerJobTimingsParams{JobID: jobID}
	for _, timing := range timings {
		start, end := timing.GetStart(), timing.GetEnd()
		if !start.IsValid() || !end.IsValid() || start.AsTime().IsZero() || end.AsTime().IsZero() || end.AsTime().Before(start.AsTime()) {
			continue
		}
		stage := database.ProvisionerJobTimingStage(timing.Stage)
		if !stage.Valid() {
			continue
		}
		params.Stage = append(params.Stage, stage)
		params.Source = append(params.Source, timing.Source)
		params.Resource = append(params.Resource, timing.Resource)
		params.Action = append(params.Action, timing.Action)
		params.StartedAt = append(params.StartedAt, start.AsTime())
		params.EndedAt = append(params.EndedAt, end.AsTime())
	}
	if len(params.Stage) == 0 {
		return nil
	}
	if _, err := db.InsertProvisionerJobTimings(ctx, params); err != nil {
		return xerrors.Errorf("insert network policy job timings: %w", err)
	}
	return nil
}
