package agent

import (
	"testing"
)

func TestResourceMonitorQueue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pushCount int
		expected  []resourcesMonitorDatapoint
	}{
		{
			name:      "Push zero",
			pushCount: 0,
			expected:  []resourcesMonitorDatapoint{},
		},
		{
			name:      "Push less than capacity",
			pushCount: 3,
			expected: []resourcesMonitorDatapoint{
				{Memory: &resourcesMonitorMemoryDatapoint{Total: 1, Used: 1}},
				{Memory: &resourcesMonitorMemoryDatapoint{Total: 2, Used: 2}},
				{Memory: &resourcesMonitorMemoryDatapoint{Total: 3, Used: 3}},
			},
		},
		{
			name:      "Push exactly capacity",
			pushCount: 20,
			expected: func() []resourcesMonitorDatapoint {
				var result []resourcesMonitorDatapoint
				for i := 1; i <= 20; i++ {
					result = append(result, resourcesMonitorDatapoint{
						Memory: &resourcesMonitorMemoryDatapoint{
							Total: int64(i),
							Used:  int64(i),
						},
					})
				}
				return result
			}(),
		},
		{
			name:      "Push more than capacity",
			pushCount: 25,
			expected: func() []resourcesMonitorDatapoint {
				var result []resourcesMonitorDatapoint
				for i := 6; i <= 25; i++ {
					result = append(result, resourcesMonitorDatapoint{
						Memory: &resourcesMonitorMemoryDatapoint{
							Total: int64(i),
							Used:  int64(i),
						},
					})
				}
				return result
			}(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			queue := newResourcesMonitorQueue(20)
			for i := 1; i <= tt.pushCount; i++ {
				queue.Push(resourcesMonitorDatapoint{
					Memory: &resourcesMonitorMemoryDatapoint{
						Total: int64(i),
						Used:  int64(i),
					},
				})
			}
			if tt.pushCount < 20 && queue.IsFull() {
				t.Errorf("expected %v, got %v", false, queue.IsFull())
			}
			if tt.pushCount >= 20 && len(queue.Items()) != 20 {
				t.Errorf("expected %v, got %v", 20, tt.pushCount)
			}
			if got := queue.Items(); !equal(got, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func equal(a, b []resourcesMonitorDatapoint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Memory.Total != b[i].Memory.Total || a[i].Memory.Used != b[i].Memory.Used {
			return false
		}
	}
	return true
}
