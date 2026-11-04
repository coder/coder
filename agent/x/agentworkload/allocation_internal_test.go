//go:build linux

package agentworkload

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOuterLimits(t *testing.T) {
	t.Parallel()
	cfg := Config{CPU: 2, MemoryBytes: 128 << 20}
	for _, tc := range []struct {
		name, file, value string
		reject            bool
	}{
		{"UnlimitedMemory", "memory.max", "max", false},
		{"UnlimitedCPU", "cpu.max", "max 100000", false},
		{"ExactMemory", "memory.max", "134217728", false},
		{"InsufficientMemory", "memory.max", "67108864", true},
		{"ExactCPU", "cpu.max", "200000 100000", false},
		{"FractionalCPU", "cpu.max", "150000 100000", true},
		{"MalformedCPU", "cpu.max", "200000", true},
		{"ZeroPeriod", "cpu.max", "200000 0", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := checkLimit(cfg, tc.file, tc.value)
			if tc.reject {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
