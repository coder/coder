package resourcesmonitor

import (
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
)

type State int

const (
	StateOK State = iota
	StateNOK
	StateUnknown
)

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

func CalculateConsecutiveNOK(states []State) int {
	maxLength := 0
	curLength := 0

	for _, state := range states {
		if state == StateNOK {
			curLength += 1
		} else {
			maxLength = max(maxLength, curLength)
			curLength = 0
		}
	}

	return max(maxLength, curLength)
}
