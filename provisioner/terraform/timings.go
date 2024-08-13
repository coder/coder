package terraform

import (
	"fmt"
	"slices"
	"time"

	"github.com/cespare/xxhash"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

type logType string

// Copied from https://github.com/hashicorp/terraform/blob/ffbcaf8bef12bb1f4d79f06437f414e280d08761/internal/command/views/json/message_types.go
// We cannot reference these because they're in an internal package.
const (
	applyStart        logType = "apply_start"
	applyProgress     logType = "apply_progress"
	applyComplete     logType = "apply_complete"
	applyErrored      logType = "apply_errored"
	provisionStart    logType = "provision_start"
	provisionProgress logType = "provision_progress"
	provisionComplete logType = "provision_complete"
	provisionErrored  logType = "provision_errored"
	refreshStart      logType = "refresh_start"
	refreshComplete   logType = "refresh_complete"
	initStart         logType = "init_start"
	initComplete      logType = "init_complete"
	initErrored       logType = "init_errored"
)

type timingsAggregator struct {
	stage       database.ProvisionerJobTimingStage
	stateLookup map[uint64]*timingsEntry
}

type timingsEntry struct {
	kind                       logType
	start, end                 time.Time
	stage                      database.ProvisionerJobTimingStage
	action, provider, resource string
	state                      proto.TimingState
}

func newTimingsAggregator(stage database.ProvisionerJobTimingStage) *timingsAggregator {
	return &timingsAggregator{
		stage:       stage,
		stateLookup: make(map[uint64]*timingsEntry),
	}
}

func (t *timingsAggregator) ingest(ts time.Time, e *timingsEntry) {
	e.stage = t.stage

	switch e.kind {
	case applyStart, provisionStart, refreshStart, initStart:
		e.start = ts
		e.state = proto.TimingState_INCOMPLETE
	case applyComplete, provisionComplete, refreshComplete, initComplete:
		e.end = ts
		e.state = proto.TimingState_COMPLETED
	case applyErrored, provisionErrored, initErrored:
		e.end = ts
		e.state = proto.TimingState_FAILED
	case applyProgress, provisionProgress:
		// Don't capture progress messages; we just want start/end timings.
		return
	}

	t.stateLookup[e.hashByState(e.state)] = e
}

func (t *timingsAggregator) aggregate() []*proto.Timing {
	out := make([]*proto.Timing, 0, len(t.stateLookup))

	for _, e := range t.stateLookup {
		switch e.state {
		case proto.TimingState_FAILED, proto.TimingState_COMPLETED:
			i, ok := t.stateLookup[e.hashByState(proto.TimingState_INCOMPLETE)]
			if !ok {
				// could not find corresponding "incomplete" event, marking as unknown.
				e.state = proto.TimingState_UNKNOWN
			} else {
				e.start = i.start
			}
		default:
			continue
		}

		out = append(out, e.toProto())
	}

	return out
}

func (l logType) Valid() bool {
	return slices.Contains([]logType{
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
	}, l)
}

// hashState computes a hash based on a timingsEntry's unique properties and state.
// The combination of resource and provider names MUST be unique across entries.
func (e *timingsEntry) hashByState(state proto.TimingState) uint64 {
	id := fmt.Sprintf("%s:%s:%s", state.String(), e.resource, e.provider)
	return xxhash.Sum64String(id)
}

func (e *timingsEntry) toProto() *proto.Timing {
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
