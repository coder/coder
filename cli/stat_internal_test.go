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
			Expected: "1.2/5.7 quatloos",
			Stat:     stat{Used: 1.234, Total: 5.678, Unit: "quatloos"},
		},
		{
			Expected: "0.0/0.0 HP",
			Stat:     stat{Used: 0, Total: 0, Unit: "HP"},
		},
	} {
		assert.Equal(t, tt.Expected, tt.Stat.String())
	}
}
