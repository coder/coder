package clistat

import (
	"testing"

	"tailscale.com/types/ptr"

	"github.com/stretchr/testify/assert"
)

func TestResultString(t *testing.T) {
	for _, tt := range []struct {
		Expected string
		Result   Result
	}{
		{
			Expected: "1.2/5.7 quatloos",
			Result:   Result{Used: 1.234, Total: ptr.To(5.678), Unit: "quatloos"},
		},
		{
			Expected: "0.0/0.0 HP",
			Result:   Result{Used: 0.0, Total: ptr.To(0.0), Unit: "HP"},
		},
		{
			Expected: "123.0 seconds",
			Result:   Result{Used: 123.01, Total: nil, Unit: "seconds"},
		},
		{
			Expected: "12.3",
			Result:   Result{Used: 12.34, Total: nil, Unit: ""},
		},
	} {
		assert.Equal(t, tt.Expected, tt.Result.String())
	}
}
