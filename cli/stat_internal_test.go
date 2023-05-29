package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatString(t *testing.T) {
	for _, tt := range []struct {
		Expected string
		Stat     stat
	}{
		{
			Expected: "1.2/3.4 quatloos (50%)",
			Stat:     stat{Used: 1.2, Total: 3.4, Unit: "quatloos"},
		},
		{
			Expected: "0/0 HP (NaN%)",
			Stat:     stat{Used: 0, Total: 0, Unit: "HP"},
		},
	} {
		assert.Equal(t, tt.Expected, tt.Stat.String())
	}
}
