package resourcesmonitor_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/proto/resourcesmonitor"
)

func TestResourceMonitorQueue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pushCount int
		expected  []resourcesmonitor.Datapoint
	}{
		{
			name:      "Push zero",
			pushCount: 0,
			expected:  []resourcesmonitor.Datapoint{},
		},
		{
			name:      "Push less than capacity",
			pushCount: 3,
			expected: []resourcesmonitor.Datapoint{
				{Memory: &resourcesmonitor.MemoryDatapoint{Total: 1, Used: 1}},
				{Memory: &resourcesmonitor.MemoryDatapoint{Total: 2, Used: 2}},
				{Memory: &resourcesmonitor.MemoryDatapoint{Total: 3, Used: 3}},
			},
		},
		{
			name:      "Push exactly capacity",
			pushCount: 20,
			expected: func() []resourcesmonitor.Datapoint {
				var result []resourcesmonitor.Datapoint
				for i := 1; i <= 20; i++ {
					result = append(result, resourcesmonitor.Datapoint{
						Memory: &resourcesmonitor.MemoryDatapoint{
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
			expected: func() []resourcesmonitor.Datapoint {
				var result []resourcesmonitor.Datapoint
				for i := 6; i <= 25; i++ {
					result = append(result, resourcesmonitor.Datapoint{
						Memory: &resourcesmonitor.MemoryDatapoint{
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
			queue := resourcesmonitor.NewQueue(20)
			for i := 1; i <= tt.pushCount; i++ {
				queue.Push(resourcesmonitor.Datapoint{
					Memory: &resourcesmonitor.MemoryDatapoint{
						Total: int64(i),
						Used:  int64(i),
					},
				})
			}

			if tt.pushCount < 20 {
				require.False(t, queue.IsFull())
			} else {
				require.True(t, queue.IsFull())
				require.Equal(t, 20, len(queue.Items()))
			}

			require.EqualValues(t, tt.expected, queue.Items())
		})
	}
}
