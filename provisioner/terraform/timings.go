package terraform

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

type timingKind string

// Copied from https://github.com/hashicorp/terraform/blob/01c0480e77263933b2b086dc8d600a69f80fad2d/internal/command/jsonformat/renderer.go
// We cannot reference these because they're in an internal package.
const (
	timingApplyStart        timingKind = "apply_start"
	timingApplyProgress     timingKind = "apply_progress"
	timingApplyComplete     timingKind = "apply_complete"
	timingApplyErrored      timingKind = "apply_errored"
	timingProvisionStart    timingKind = "provision_start"
	timingProvisionProgress timingKind = "provision_progress"
	timingProvisionComplete timingKind = "provision_complete"
	timingProvisionErrored  timingKind = "provision_errored"
	timingRefreshStart      timingKind = "refresh_start"
	timingRefreshComplete   timingKind = "refresh_complete"
	// Ignored.
	timingChangeSummary timingKind = "change_summary"
	timingDiagnostic    timingKind = "diagnostic"
	timingPlannedChange timingKind = "planned_change"
	timingOutputs       timingKind = "outputs"
	timingResourceDrift timingKind = "resource_drift"
	timingVersion       timingKind = "version"
	// These are not part of message_types, but we want to track init/graph timings as well.
	timingInitStart     timingKind = "init_start"
	timingInitComplete  timingKind = "init_complete"
	timingInitErrored   timingKind = "init_errored"
	timingGraphStart    timingKind = "graph_start"
	timingGraphComplete timingKind = "graph_complete"
	timingGraphErrored  timingKind = "graph_errored"
	// Other terraform log types which we ignore.
	timingLog        timingKind = "log"
	timingInitOutput timingKind = "init_output"
)

type timingAggregator struct {
	stage database.ProvisionerJobTimingStage

	// Protects the stateLookup map.
	lookupMu    sync.Mutex
	stateLookup map[uint64]*timingSpan
}

type timingSpan struct {
	kind                       timingKind
	start, end                 time.Time
	stage                      database.ProvisionerJobTimingStage
	action, provider, resource string
	state                      proto.TimingState
}

// newTimingAggregator creates a new aggregator which measures the duration of resource init/plan/apply actions; stage
// represents the stage of provisioning the timings are occurring within.
func newTimingAggregator(stage database.ProvisionerJobTimingStage) *timingAggregator {
	return &timingAggregator{
		stage:       stage,
		stateLookup: make(map[uint64]*timingSpan),
	}
}

// ingest accepts a timing span at a certain timestamp and assigns it a state according to the kind of timing event.
// We memoize start & completion events, and then calculate their total duration in aggregate.
// We ignore progress events because we only care about the full duration of the action (delta between *_start and *_complete events).
func (t *timingAggregator) ingest(ts time.Time, s *timingSpan) {
	if s == nil {
		return
	}

	s.stage = t.stage
	ts = dbtime.Time(ts.UTC())

	switch s.kind {
	case timingApplyStart, timingProvisionStart, timingRefreshStart, timingInitStart, timingGraphStart:
		s.start = ts
		s.state = proto.TimingState_STARTED
	case timingApplyComplete, timingProvisionComplete, timingRefreshComplete, timingInitComplete, timingGraphComplete:
		s.end = ts
		s.state = proto.TimingState_COMPLETED
	case timingApplyErrored, timingProvisionErrored, timingInitErrored, timingGraphErrored:
		s.end = ts
		s.state = proto.TimingState_FAILED
	default:
		// We just want start/end timings, ignore all other events.
		return
	}

	t.lookupMu.Lock()
	// Memoize this span by its unique attributes and the determined state.
	// This will be used in aggregate() to determine the duration of the resource action.
	t.stateLookup[s.hashByState(s.state)] = s
	t.lookupMu.Unlock()
}

// aggregate performs a pass through all memoized events to build up a slice of *proto.Timing instances which represent
// the total time taken to perform a certain action.
// The resulting slice of *proto.Timing is NOT sorted.
func (t *timingAggregator) aggregate() []*proto.Timing {
	t.lookupMu.Lock()
	defer t.lookupMu.Unlock()

	// Pre-allocate len(measurements)/2 since each timing will have one STARTED and one FAILED/COMPLETED entry.
	out := make([]*proto.Timing, 0, len(t.stateLookup)/2)

	for _, s := range t.stateLookup {
		// We are only concerned here with failed or completed events.
		if s.state != proto.TimingState_FAILED && s.state != proto.TimingState_COMPLETED {
			continue
		}

		// Look for a corresponding span for the STARTED state.
		startSpan, ok := t.stateLookup[s.hashByState(proto.TimingState_STARTED)]
		if !ok {
			// Not found, we'll ignore this span.
			continue
		}
		s.start = startSpan.start

		// Until faster-than-light travel is a possibility, let's prevent this.
		// Better to capture a zero delta than a negative one.
		if s.start.After(s.end) {
			s.start = s.end
		}

		// Let's only aggregate valid entries.
		// Later we can add support for partial / failed applies, perhaps.
		if s.start.IsZero() || s.end.IsZero() {
			continue
		}

		out = append(out, s.toProto())
	}

	return out
}

func (l timingKind) Valid() bool {
	return slices.Contains([]timingKind{
		timingApplyStart,
		timingApplyProgress,
		timingApplyComplete,
		timingApplyErrored,
		timingProvisionStart,
		timingProvisionProgress,
		timingProvisionComplete,
		timingProvisionErrored,
		timingRefreshStart,
		timingRefreshComplete,
		timingChangeSummary,
		timingDiagnostic,
		timingPlannedChange,
		timingOutputs,
		timingResourceDrift,
		timingVersion,
		timingInitStart,
		timingInitComplete,
		timingInitErrored,
		timingGraphStart,
		timingGraphComplete,
		timingGraphErrored,
		timingLog,
		timingInitOutput,
	}, l)
}

// Category returns the category for a giving timing state so that timings can be aggregated by this category.
// We can't use the state itself because we need an `apply_start` and an `apply_complete` to both hash to the same entry
// if all other attributes are identical.
func (l timingKind) Category() string {
	switch l {
	case timingInitStart, timingInitComplete, timingInitErrored:
		return "init"
	case timingGraphStart, timingGraphComplete, timingGraphErrored:
		return "graph"
	case timingApplyStart, timingApplyProgress, timingApplyComplete, timingApplyErrored:
		return "apply"
	case timingProvisionStart, timingProvisionProgress, timingProvisionComplete, timingProvisionErrored:
		return "provision"
	case timingRefreshStart, timingRefreshComplete:
		return "state refresh"
	default:
		return "?"
	}
}

// hashState computes a hash based on a timingSpan's unique properties and state.
// The combination of resource and provider names MUST be unique across entries.
func (e *timingSpan) hashByState(state proto.TimingState) uint64 {
	id := fmt.Sprintf("%s:%s:%s:%s:%s", e.kind.Category(), state.String(), e.action, e.resource, e.provider)
	return xxhash.Sum64String(id)
}

func (e *timingSpan) toProto() *proto.Timing {
	// Some log entries, like state refreshes, don't have any "action" logged.
	if e.action == "" {
		e.action = e.kind.Category()
	}

	return &proto.Timing{
		Start:    timestamppb.New(e.start),
		End:      timestamppb.New(e.end),
		Action:   e.action,
		Stage:    string(e.stage),
		Source:   e.provider,
		Resource: e.resource,
		State:    e.state,
	}
}

func createInitTimingsEvent(event timingKind) (time.Time, *timingSpan) {
	return dbtime.Now(), &timingSpan{
		kind:     event,
		action:   "initializing terraform",
		provider: "terraform",
		resource: "state file",
	}
}

func createGraphTimingsEvent(event timingKind) (time.Time, *timingSpan) {
	return dbtime.Now(), &timingSpan{
		kind:     event,
		action:   "building terraform dependency graph",
		provider: "terraform",
		resource: "state file",
	}
}
