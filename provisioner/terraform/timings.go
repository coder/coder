package terraform

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/cespare/xxhash"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

type timingKind string

// Copied from https://github.com/hashicorp/terraform/blob/ffbcaf8bef12bb1f4d79f06437f414e280d08761/internal/command/views/json/message_types.go
// We cannot reference these because they're in an internal package.
const (
	applyStart        timingKind = "apply_start"
	applyProgress     timingKind = "apply_progress"
	applyComplete     timingKind = "apply_complete"
	applyErrored      timingKind = "apply_errored"
	provisionStart    timingKind = "provision_start"
	provisionProgress timingKind = "provision_progress"
	provisionComplete timingKind = "provision_complete"
	provisionErrored  timingKind = "provision_errored"
	refreshStart      timingKind = "refresh_start"
	refreshComplete   timingKind = "refresh_complete"
	// These are not part of message_types, but we want to track init timings as well.
	initStart    timingKind = "init_start"
	initComplete timingKind = "init_complete"
	initErrored  timingKind = "init_errored"
)

type timingAggregator struct {
	mu sync.Mutex

	stage       database.ProvisionerJobTimingStage
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
	ts = dbtime.Time(ts)

	switch s.kind {
	case applyStart, provisionStart, refreshStart, initStart:
		s.start = ts
		s.state = proto.TimingState_STARTED
	case applyComplete, provisionComplete, refreshComplete, initComplete:
		s.end = ts
		s.state = proto.TimingState_COMPLETED
	case applyErrored, provisionErrored, initErrored:
		s.end = ts
		s.state = proto.TimingState_FAILED
	default:
		// Don't capture progress messages (or unhandled kinds); we just want start/end timings.
		return
	}

	t.mu.Lock()
	// Memoize this span by its unique attributes and the determined state.
	// This will be used in aggregate() to determine the duration of the resource action.
	t.stateLookup[s.hashByState(s.state)] = s
	t.mu.Unlock()
}

// aggregate performs a pass through all memoized events to build up a set of *proto.Timing instances which represent
// the total time taken to perform a certain action.
func (t *timingAggregator) aggregate() []*proto.Timing {
	t.mu.Lock()
	defer t.mu.Unlock()

	out := make([]*proto.Timing, 0, len(t.stateLookup))

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
		applyStart,
		applyProgress,
		applyComplete,
		applyErrored,
		provisionStart,
		provisionProgress,
		provisionComplete,
		provisionErrored,
		refreshStart,
		refreshComplete,
		initStart,
		initComplete,
		initErrored,
	}, l)
}

// hashState computes a hash based on a timingSpan's unique properties and state.
// The combination of resource and provider names MUST be unique across entries.
func (e *timingSpan) hashByState(state proto.TimingState) uint64 {
	id := fmt.Sprintf("%s:%s:%s", state.String(), e.resource, e.provider)
	return xxhash.Sum64String(id)
}

func (e *timingSpan) toProto() *proto.Timing {
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
