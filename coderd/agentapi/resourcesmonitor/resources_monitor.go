package resourcesmonitor

import (
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/slice"
)

type State int

const (
	StateOK State = iota
	StateNOK
	StateUnknown
)

type Config struct {
	// How many datapoints in a row are required to
	// put the monitor in an alert state.
	ConsecutiveNOKsToAlert int

	// How many datapoints in total are required to
	// put the monitor in an alert state.
	MinimumNOKsToAlert int
}

func CalculateMemoryUsageStates(
	monitor database.WorkspaceAgentMemoryResourceMonitor,
	datapoints []*proto.PushResourcesMonitoringUsageRequest_Datapoint_MemoryUsage,
) []State {
	states := make([]State, 0, len(datapoints))

	for _, datapoint := range datapoints {
		state := StateUnknown

		if datapoint != nil {
			percent := int32(float64(datapoint.Used) / float64(datapoint.Total) * 100)

			if percent < monitor.Threshold {
				state = StateOK
			} else {
				state = StateNOK
			}
		}

		states = append(states, state)
	}

	return states
}

func CalculateVolumeUsageStates(
	monitor database.WorkspaceAgentVolumeResourceMonitor,
	datapoints []*proto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage,
) []State {
	states := make([]State, 0, len(datapoints))

	for _, datapoint := range datapoints {
		state := StateUnknown

		if datapoint != nil {
			percent := int32(float64(datapoint.Used) / float64(datapoint.Total) * 100)

			if percent < monitor.Threshold {
				state = StateOK
			} else {
				state = StateNOK
			}
		}

		states = append(states, state)
	}

	return states
}

func NextState(c Config, oldState database.WorkspaceAgentMonitorState, states []State) database.WorkspaceAgentMonitorState {
	// If there are enough consecutive NOK states, we should be in an
	// alert state.
	consecutiveNOKs := slice.CountConsecutive(StateNOK, states...)
	if consecutiveNOKs >= c.ConsecutiveNOKsToAlert {
		return database.WorkspaceAgentMonitorStateNOK
	}

	nokCount, okCount := 0, 0
	for _, state := range states {
		switch state {
		case StateOK:
			okCount++
		case StateNOK:
			nokCount++
		}
	}

	// If there are enough NOK datapoints, we should be in an alert state.
	if nokCount >= c.MinimumNOKsToAlert {
		return database.WorkspaceAgentMonitorStateNOK
	}

	// If all datapoints are OK, we should be in an OK state
	if okCount == len(states) {
		return database.WorkspaceAgentMonitorStateOK
	}

	// Otherwise we stay in the same state as last.
	return oldState
}
