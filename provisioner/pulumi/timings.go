package pulumi

import (
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

type timingAggregator struct {
	stage database.ProvisionerJobTimingStage

	mu      sync.Mutex
	timings []*proto.Timing
}

func newTimingAggregator(stage database.ProvisionerJobTimingStage) *timingAggregator {
	return &timingAggregator{stage: stage}
}

func (t *timingAggregator) aggregate() []*proto.Timing {
	if t == nil {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	aggregated := make([]*proto.Timing, len(t.timings))
	copy(aggregated, t.timings)
	return aggregated
}

func (t *timingAggregator) startStage(stage database.ProvisionerJobTimingStage) func(error) {
	startedAt := time.Now().UTC()
	return func(err error) {
		if t == nil {
			return
		}
		t.recordCommand("stage_"+string(stage), startedAt, err)
	}
}

func (t *timingAggregator) recordCommand(command string, startedAt time.Time, err error) {
	if t == nil || command == "" {
		return
	}

	finishedAt := time.Now().UTC()
	startedAt = startedAt.UTC()
	if finishedAt.Before(startedAt) {
		finishedAt = startedAt
	}

	state := proto.TimingState_COMPLETED
	if err != nil {
		state = proto.TimingState_FAILED
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.timings = append(t.timings, &proto.Timing{
		Start:    timestamppb.New(startedAt),
		End:      timestamppb.New(finishedAt),
		Action:   command,
		Source:   "pulumi",
		Resource: "pulumi_command_" + command,
		Stage:    string(t.stage),
		State:    state,
	})
}
