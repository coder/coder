package agent

import (
	"github.com/coder/coder/v2/agent/proto"
)

type resourcesMonitorDatapoint struct {
	Memory  *resourcesMonitorMemoryDatapoint
	Volumes []*resourcesMonitorVolumeDatapoint
}

type resourcesMonitorMemoryDatapoint struct {
	Total int64
	Used  int64
}

type resourcesMonitorVolumeDatapoint struct {
	Path  string
	Total int64
	Used  int64
}

// resourcesMonitorQueue represents a FIFO queue with a fixed size
type resourcesMonitorQueue struct {
	items []resourcesMonitorDatapoint
	size  int
}

// newResourcesMonitorQueue creates a new resourcesMonitorQueue with the given size
func newResourcesMonitorQueue(size int) *resourcesMonitorQueue {
	return &resourcesMonitorQueue{
		items: make([]resourcesMonitorDatapoint, 0, size),
		size:  size,
	}
}

// Push adds a new item to the queue
func (q *resourcesMonitorQueue) Push(item resourcesMonitorDatapoint) {
	if len(q.items) >= q.size {
		// Remove the first item (FIFO)
		q.items = q.items[1:]
	}
	q.items = append(q.items, item)
}

func (q *resourcesMonitorQueue) IsFull() bool {
	return len(q.items) == q.size
}

func (q *resourcesMonitorQueue) Items() []resourcesMonitorDatapoint {
	return q.items
}

func (q *resourcesMonitorQueue) ItemsAsProto() []*proto.PushResourcesMonitoringUsageRequest_Datapoint {
	items := make([]*proto.PushResourcesMonitoringUsageRequest_Datapoint, 0, len(q.items))

	for _, item := range q.items {
		protoItem := &proto.PushResourcesMonitoringUsageRequest_Datapoint{
			Memory: &proto.PushResourcesMonitoringUsageRequest_Datapoint_MemoryUsage{
				Total: item.Memory.Total,
				Used:  item.Memory.Used,
			},
		}

		for _, volume := range item.Volumes {
			protoItem.Volumes = append(protoItem.Volumes, &proto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage{
				Volume: volume.Path,
				Total:  volume.Total,
				Used:   volume.Used,
			})
		}

		items = append(items, protoItem)
	}

	return items
}
