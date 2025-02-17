package resourcesmonitor

import (
	"math"
	"time"

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

type AlertConfig struct {
	// What percentage of datapoints in a row are
	// required to put the monitor in an alert state.
	ConsecutiveNOKsPercent int

	// What percentage of datapoints in a window are
	// required to put the monitor in an alert state.
	MinimumNOKsPercent int
}

type Config struct {
	// How many datapoints should the agent send
	NumDatapoints int32

	// How long between each datapoint should
	// collection occur.
	CollectionInterval time.Duration

	Alert AlertConfig
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
	if percent(consecutiveNOKs, len(states)) >= c.Alert.ConsecutiveNOKsPercent {
		return database.WorkspaceAgentMonitorStateNOK
	}

	// We do not explicitly handle StateUnknown because it could have
	// been either StateOK or StateNOK if collection didn't fail. As
	// it could be either, our best bet is to ignore it.
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
	if percent(nokCount, len(states)) >= c.Alert.MinimumNOKsPercent {
		return database.WorkspaceAgentMonitorStateNOK
	}

	// If all datapoints are OK, we should be in an OK state
	if okCount == len(states) {
		return database.WorkspaceAgentMonitorStateOK
	}

	// Otherwise we stay in the same state as last.
	return oldState
}

func percent[T int](numerator, denominator T) int {
	percent := float64(numerator*100) / float64(denominator)
	return int(math.Round(percent))
}
