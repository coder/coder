package agent

import (
	"github.com/coder/coder/v2/agent/proto"
)

type ResourcesMonitorDatapoint struct {
	Memory  *ResourcesMonitorMemoryDatapoint
	Volumes []*ResourcesMonitorVolumeDatapoint
}

type ResourcesMonitorMemoryDatapoint struct {
	Total int64
	Used  int64
}

type ResourcesMonitorVolumeDatapoint struct {
	Path  string
	Total int64
	Used  int64
}

// ResourcesMonitorQueue represents a FIFO queue with a fixed size
type ResourcesMonitorQueue struct {
	items []ResourcesMonitorDatapoint
	size  int
}

// newResourcesMonitorQueue creates a new ResourcesMonitorQueue with the given size
func NewResourcesMonitorQueue(size int) *ResourcesMonitorQueue {
	return &ResourcesMonitorQueue{
		items: make([]ResourcesMonitorDatapoint, 0, size),
		size:  size,
	}
}

// Push adds a new item to the queue
func (q *ResourcesMonitorQueue) Push(item ResourcesMonitorDatapoint) {
	if len(q.items) >= q.size {
		// Remove the first item (FIFO)
		q.items = q.items[1:]
	}
	q.items = append(q.items, item)
}

func (q *ResourcesMonitorQueue) IsFull() bool {
	return len(q.items) == q.size
}

func (q *ResourcesMonitorQueue) Items() []ResourcesMonitorDatapoint {
	return q.items
}

func (q *ResourcesMonitorQueue) ItemsAsProto() []*proto.PushResourcesMonitoringUsageRequest_Datapoint {
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
