package resourcesmonitor

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/agent/proto"
)

type Datapoint struct {
	CollectedAt time.Time
	Memory      *MemoryDatapoint
	Volumes     []*VolumeDatapoint
}

type MemoryDatapoint struct {
	Total int64
	Used  int64
}

type VolumeDatapoint struct {
	Path  string
	Total int64
	Used  int64
}

// Queue represents a FIFO queue with a fixed size
type Queue struct {
	items []Datapoint
	size  int
}

// newQueue creates a new Queue with the given size
func NewQueue(size int) *Queue {
	return &Queue{
		items: make([]Datapoint, 0, size),
		size:  size,
	}
}

// Push adds a new item to the queue
func (q *Queue) Push(item Datapoint) {
	if len(q.items) >= q.size {
		// Remove the first item (FIFO)
		q.items = q.items[1:]
	}
	q.items = append(q.items, item)
}

func (q *Queue) IsFull() bool {
	return len(q.items) == q.size
}

func (q *Queue) Items() []Datapoint {
	return q.items
}

func (q *Queue) ItemsAsProto() []*proto.PushResourcesMonitoringUsageRequest_Datapoint {
	items := make([]*proto.PushResourcesMonitoringUsageRequest_Datapoint, 0, len(q.items))

	for _, item := range q.items {
		protoItem := &proto.PushResourcesMonitoringUsageRequest_Datapoint{
			CollectedAt: timestamppb.New(item.CollectedAt),
		}
		if item.Memory != nil {
			protoItem.Memory = &proto.PushResourcesMonitoringUsageRequest_Datapoint_MemoryUsage{
				Total: item.Memory.Total,
				Used:  item.Memory.Used,
			}
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
