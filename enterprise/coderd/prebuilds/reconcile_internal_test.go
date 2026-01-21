package prebuilds

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalculateReconciliationConcurrency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		maxDBConnections    int
		expectedConcurrency int
	}{
		{"base pool size", 10, 5},
		{"default pool size", 30, 5},
		{"large pool size", 100, 5},
		{"small pool", 4, 2},
		{"minimum pool", 2, 1},
		{"single connection", 1, 1},
		{"zero connections floors to 1", 0, 1},
		{"negative floors to 1", -5, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := calculateReconciliationConcurrency(tt.maxDBConnections)
			require.Equal(t, tt.expectedConcurrency, result)
		})
	}
}
