package cli

import (
	"testing"

	"tailscale.com/types/ptr"

	"github.com/stretchr/testify/assert"
)

func TestStatString(t *testing.T) {
	for _, tt := range []struct {
		Expected string
		Stat     stat
	}{
		{
			Expected: "1.2/5.7 quatloos",
			Stat:     stat{Used: 1.234, Total: ptr.To(5.678), Unit: "quatloos"},
		},
		{
			Expected: "0.0/0.0 HP",
			Stat:     stat{Used: 0, Total: ptr.To(0.0), Unit: "HP"},
		},
		{
			Expected: "123.0 seconds",
			Stat:     stat{Used: 123.0, Total: nil, Unit: "seconds"},
		},
	} {
		assert.Equal(t, tt.Expected, tt.Stat.String())
	}
}
