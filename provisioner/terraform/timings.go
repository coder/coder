package terraform

import (
	"fmt"
	"slices"
	"time"

	"github.com/cespare/xxhash"
	"google.golang.org/protobuf/types/known/timestamppb"

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
)

type timingsAggregator struct {
	stateLookup map[uint64]*entry
}

type entry struct {
	kind                       logType
	start, end                 time.Time
	action, provider, resource string
	state                      proto.TimingState
}

func newTimingsAggregator() *timingsAggregator {
	return &timingsAggregator{
		stateLookup: make(map[uint64]*entry),
	}
}

func (t *timingsAggregator) ingest(log terraformProvisionLog) {
	// Input is not well-formed, bail out.
	if log.Type == "" {
		return
	}

	typ := logType(log.Type)
	if !typ.Valid() {
		// TODO: log
		return
	}

	ts, err := time.Parse("2006-01-02T15:04:05.000000Z07:00", log.Timestamp)
	if err != nil {
		// TODO: log
		ts = time.Now()
	}
	ts = ts.UTC()

	e := &entry{
		kind:     typ,
		action:   log.Hook.Action,
		provider: log.Hook.Resource.Provider,
		resource: log.Hook.Resource.Addr,
	}

	switch typ {
	case applyStart, provisionStart, refreshStart:
		e.start = ts
		e.state = proto.TimingState_INCOMPLETE
	case applyComplete, provisionComplete, refreshComplete:
		e.end = ts
		e.state = proto.TimingState_COMPLETED
	case applyErrored, provisionErrored:
		e.end = ts
		e.state = proto.TimingState_FAILED
	case applyProgress, provisionProgress:
		// Don't capture progress messages; we just want start/end timings.
		return
	}

	t.stateLookup[e.hashByState(e.state)] = e
}

func (t *timingsAggregator) aggregate() ([]*proto.Timing, error) {
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

	return out, nil
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

// hashState computes a hash based on an entry's unique properties and state.
// The combination of resource and provider names MUST be unique across entries.
func (e *entry) hashByState(state proto.TimingState) uint64 {
	id := fmt.Sprintf("%s:%s:%s", state.String(), e.resource, e.provider)
	return xxhash.Sum64String(id)
}

func (e *entry) toProto() *proto.Timing {
	return &proto.Timing{
		Start:    timestamppb.New(e.start),
		End:      timestamppb.New(e.end),
		Action:   e.action,
		Provider: e.provider,
		Resource: e.resource,
		State:    e.state,
	}
}
