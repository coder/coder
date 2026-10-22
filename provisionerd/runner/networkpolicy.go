package runner

import (
	"context"

	protobuf "google.golang.org/protobuf/proto"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfstate"
)

// networkPolicyStateDisposition is conservative once apply may have started.
// Only a successful ApplyComplete carrying state establishes the resulting state.
func (r *Runner) networkPolicyStateDisposition() proto.StateDisposition {
	if r.networkPolicyApplyStarted.Load() {
		return proto.StateDisposition_UNKNOWN
	}
	return proto.StateDisposition_UNCHANGED
}

func (r *Runner) runNetworkPolicy(ctx context.Context) (completed *proto.CompletedJob, failed *proto.FailedJob) {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	job := r.job.GetNetworkPolicy()
	var (
		timings []*sdkproto.Timing
		state   []byte
	)
	disposition := proto.StateDisposition_UNCHANGED
	defer func() {
		if failed != nil {
			failed.Type = &proto.FailedJob_NetworkPolicy_{
				NetworkPolicy: &proto.FailedJob_NetworkPolicy{
					State:            state,
					StateDisposition: disposition,
					Timings:          timings,
				},
			}
		}
	}()

	if job.Operation != proto.NetworkPolicyOperation_PLAN && job.Operation != proto.NetworkPolicyOperation_APPLY && job.Operation != proto.NetworkPolicyOperation_DESTROY {
		return nil, r.failedJobf("unknown network policy operation %d", job.Operation)
	}

	// Terraform requires a transition, but a policy job has no workspace.
	metadata := &sdkproto.Metadata{WorkspaceTransition: sdkproto.WorkspaceTransition_START}
	if job.Metadata != nil {
		protobuf.Merge(metadata, job.Metadata)
		metadata.WorkspaceTransition = sdkproto.WorkspaceTransition_START
	}
	if job.Operation == proto.NetworkPolicyOperation_DESTROY {
		metadata.WorkspaceTransition = sdkproto.WorkspaceTransition_DESTROY
		if len(job.State) == 0 {
			return nil, r.failedJobf("network policy destroy requires prior state")
		}
	}
	if failed := r.configure(&sdkproto.Config{}); failed != nil {
		return nil, failed
	}
	initComplete, failed := r.init(ctx, true, r.job.TemplateSourceArchive, nil)
	if failed != nil {
		return nil, failed
	}
	if initComplete == nil {
		return nil, r.failedJobf("network policy init returned nil response")
	}
	timings = append(timings, initComplete.Timings...)
	if initComplete.Error != "" {
		return nil, r.failedJobf("network policy init: %s", initComplete.Error)
	}

	planComplete, failed := r.plan(ctx, "Planning network policy", &sdkproto.PlanRequest{
		Metadata:       metadata,
		VariableValues: job.VariableValues,
		State:          job.State,
	})
	if failed != nil {
		return nil, failed
	}
	if planComplete == nil {
		return nil, r.failedJobf("network policy plan returned nil response")
	}
	timings = append(timings, planComplete.Timings...)
	if planComplete.Error != "" {
		return nil, r.failedJobf("network policy plan: %s", planComplete.Error)
	}

	if job.Operation == proto.NetworkPolicyOperation_APPLY || job.Operation == proto.NetworkPolicyOperation_DESTROY {
		// Serialize with Fail so a forced failure cannot report UNCHANGED
		// while the runner proceeds to start apply.
		r.mutex.Lock()
		err := r.notCanceled.Err()
		if err == nil {
			r.networkPolicyApplyStarted.Store(true)
		}
		r.mutex.Unlock()
		if err != nil {
			return nil, r.failedJobf("network policy canceled before apply: %s", err)
		}
		disposition = proto.StateDisposition_UNKNOWN
		applyComplete, failed := r.apply(ctx, "Applying network policy", &sdkproto.ApplyRequest{Metadata: metadata})
		if failed != nil {
			return nil, failed
		}
		if applyComplete == nil {
			return nil, r.failedJobf("network policy apply returned nil response")
		}
		timings = append(timings, applyComplete.Timings...)
		state = applyComplete.State
		if applyComplete.Error != "" {
			// Terraform may return an old state file after an interrupted apply
			// that changed infrastructure. Preserve these bytes as a checkpoint,
			// but keep UNKNOWN so the server requires reconciliation.
			return nil, r.failedJobf("network policy apply: %s", applyComplete.Error)
		}
		if len(state) == 0 {
			return nil, r.failedJobf("network policy apply returned no state")
		}
		if job.Operation == proto.NetworkPolicyOperation_DESTROY {
			if err := tfstate.ValidateNetworkPolicyDestroyState(state); err != nil {
				return nil, r.failedJobf("network policy destroy: %s", err)
			}
		}
		disposition = proto.StateDisposition_RETURNED
	}

	return &proto.CompletedJob{
		JobId: r.job.JobId,
		Type: &proto.CompletedJob_NetworkPolicy_{
			NetworkPolicy: &proto.CompletedJob_NetworkPolicy{
				Plan:             planComplete.Plan,
				State:            state,
				StateDisposition: disposition,
				Timings:          timings,
			},
		},
	}, nil
}
