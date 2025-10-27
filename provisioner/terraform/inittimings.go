package terraform

import (
	"slices"
	"time"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

var (
	// resourceName maps init message codes to human-readable resource names.
	// This is purely for better readability in the timing spans.
	resourceName = map[initMessageCode]string{
		initInitializingBackendMessage:        "Backend",
		initInitializingModulesMessage:        "Modules",
		initInitializingProviderPluginMessage: "Provider Plugins",
	}

	// executionOrder is the expected sequential steps during `terraform init`.
	executionOrder = []initMessageCode{
		initInitializingBackendMessage,
		initInitializingModulesMessage,
		initInitializingProviderPluginMessage,
		initOutputInitSuccessMessage,
	}
)

// ingestInitTiming handles ingesting timing spans from `terraform init` logs.
// These logs are formatted differently from plan/apply logs, so they need their
// own ingestion logic.
//
// The logs are also less granular, only indicating the start and end of
// major init stages, rather than per-resource actions.
// Since initialization is done serially, we can infer the end time of each
// stage from the start time of the next stage.
func (t *timingAggregator) ingestInitTiming(ts time.Time, s *timingSpan) {
	switch s.initMessageCode {
	case initInitializingBackendMessage:
		// Backend loads the tfstate from the backend data source. For coder, this is
		// always a state file on disk, making it nearly an instantaneous operation.
		s.start = ts
		s.state = proto.TimingState_STARTED
	case initInitializingModulesMessage:
		s.start = ts
		s.state = proto.TimingState_STARTED
	case initInitializingProviderPluginMessage:
		s.start = ts
		s.state = proto.TimingState_STARTED
	case initOutputInitSuccessMessage:
		// The final message indicates successful completion of init. There is no start
		// message for this, but we want to continue the pattern such that this completes
		// the previous stage.
		s.end = ts
		s.state = proto.TimingState_COMPLETED
	default:
		return
	}

	// Init logs should be assigned to the init stage.
	s.stage = database.ProvisionerJobTimingStageInit
	// The default action is an empty string
	s.action = "load"
	// Resource name is an empty string. Name it something more useful.
	s.resource = resourceName[s.initMessageCode]

	t.finishPrevious(ts, s)

	t.lookupMu.Lock()
	// Memoize this span by its unique attributes and the determined state.
	// This will be used in aggregate() to determine the duration of the resource action.
	t.stateLookup[s.hashByState(s.state)] = s
	t.lookupMu.Unlock()
}

func (t *timingAggregator) finishPrevious(ts time.Time, s *timingSpan) {
	index := slices.Index(executionOrder, s.initMessageCode)
	if index <= 0 {
		// If the index is not found or is the first item, nothing to complete.
		return
	}

	// Complete the previous message.
	previous := executionOrder[index-1]

	cpy := *s
	cpy.start = time.Time{}
	cpy.end = ts
	cpy.initMessageCode = previous
	cpy.resource, _ = resourceName[previous]
	cpy.state = proto.TimingState_COMPLETED

	t.lookupMu.Lock()
	// Memoize this span by its unique attributes and the determined state.
	// This will be used in aggregate() to determine the duration of the resource action.
	t.stateLookup[cpy.hashByState(cpy.state)] = &cpy
	t.lookupMu.Unlock()
}

// mergeInitTimings merges manual init timings with existing timings that are
// sourced by the logs. This is done because prior to Terraform v1.9, init logs
// did not have a `-json` formatting option.
// So before v1.9, the init stage is manually timed outside the `terraform init`.
// After v1.9, the init stage is timed via logs.
func mergeInitTimings(manualInit []*proto.Timing, existing []*proto.Timing) []*proto.Timing {
	hasInitStage := slices.ContainsFunc(existing, func(timing *proto.Timing) bool {
		return timing.Stage == string(database.ProvisionerJobTimingStageInit)
	})

	if hasInitStage {
		return existing
	}

	return append(manualInit, existing...)
}
