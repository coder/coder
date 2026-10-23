package runner

import (
	"context"

	protobuf "google.golang.org/protobuf/proto"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfstate"
)

// templateResourcesStateDisposition is conservative once apply may have started.
func (r *Runner) templateResourcesStateDisposition() proto.StateDisposition {
	if r.templateResourcesApplyStarted.Load() {
		return proto.StateDisposition_UNKNOWN
	}
	return proto.StateDisposition_UNCHANGED
}

func (r *Runner) runTemplateResources(ctx context.Context) (completed *proto.CompletedJob, failed *proto.FailedJob) {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	job := r.job.GetTemplateResources()
	var (
		timings []*sdkproto.Timing
		state   []byte
	)
	disposition := proto.StateDisposition_UNCHANGED
	defer func() {
		if failed != nil {
			failed.Type = &proto.FailedJob_TemplateResources_{
				TemplateResources: &proto.FailedJob_TemplateResources{
					State: state, StateDisposition: disposition, Timings: timings,
				},
			}
		}
	}()

	if job.Operation != proto.TemplateResourceOperation_TEMPLATE_RESOURCE_PLAN && job.Operation != proto.TemplateResourceOperation_TEMPLATE_RESOURCE_APPLY && job.Operation != proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY {
		return nil, r.failedJobf("unknown template resource operation %d", job.Operation)
	}

	// Terraform requires a transition, but this root has no workspace.
	metadata := &sdkproto.Metadata{}
	if job.Metadata != nil {
		protobuf.Merge(metadata, job.Metadata)
	}
	metadata.WorkspaceTransition = sdkproto.WorkspaceTransition_START
	if job.Operation == proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY {
		metadata.WorkspaceTransition = sdkproto.WorkspaceTransition_DESTROY
		if len(job.State) == 0 {
			return nil, r.failedJobf("template resource destroy requires prior state")
		}
	}
	if len(job.State) != 0 {
		if err := tfstate.ValidateState(job.State); err != nil {
			return nil, r.failedJobf("template resource prior state: %s", err)
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
		return nil, r.failedJobf("template resource init returned nil response")
	}
	timings = append(timings, initComplete.Timings...)
	if initComplete.Error != "" {
		return nil, r.failedJobf("template resource init: %s", initComplete.Error)
	}

	planComplete, failed := r.plan(ctx, "Planning template resources", &sdkproto.PlanRequest{
		Metadata: metadata, VariableValues: job.VariableValues, State: job.State,
	})
	if failed != nil {
		return nil, failed
	}
	if planComplete == nil {
		return nil, r.failedJobf("template resource plan returned nil response")
	}
	timings = append(timings, planComplete.Timings...)
	if planComplete.Error != "" {
		return nil, r.failedJobf("template resource plan: %s", planComplete.Error)
	}

	if job.Operation != proto.TemplateResourceOperation_TEMPLATE_RESOURCE_PLAN {
		// Serialize with Fail so forced failure cannot report UNCHANGED while
		// this runner proceeds to start apply.
		r.mutex.Lock()
		err := r.notCanceled.Err()
		if err == nil {
			r.templateResourcesApplyStarted.Store(true)
		}
		r.mutex.Unlock()
		if err != nil {
			return nil, r.failedJobf("template resources canceled before apply: %s", err)
		}
		disposition = proto.StateDisposition_UNKNOWN
		applyComplete, failed := r.apply(ctx, "Applying template resources", &sdkproto.ApplyRequest{Metadata: metadata})
		if failed != nil {
			return nil, failed
		}
		if applyComplete == nil {
			return nil, r.failedJobf("template resource apply returned nil response")
		}
		timings = append(timings, applyComplete.Timings...)
		state = applyComplete.State
		if applyComplete.Error != "" {
			// Interrupted apply can return old state even after changing remote
			// infrastructure. Keep it only as a checkpoint with UNKNOWN status.
			return nil, r.failedJobf("template resource apply: %s", applyComplete.Error)
		}
		if len(state) == 0 {
			return nil, r.failedJobf("template resource apply returned no state")
		}
		if err := tfstate.ValidateState(state); err != nil {
			return nil, r.failedJobf("template resource apply: %s", err)
		}
		if job.Operation == proto.TemplateResourceOperation_TEMPLATE_RESOURCE_DESTROY {
			if err := tfstate.ValidateDestroyState(state); err != nil {
				return nil, r.failedJobf("template resource destroy: %s", err)
			}
		}
		disposition = proto.StateDisposition_RETURNED
	}

	return &proto.CompletedJob{
		JobId: r.job.JobId,
		Type: &proto.CompletedJob_TemplateResources_{
			TemplateResources: &proto.CompletedJob_TemplateResources{
				Plan: planComplete.Plan, State: state, StateDisposition: disposition, Timings: timings,
			},
		},
	}, nil
}
