package dynamicparameters_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/dynamicparameters"
)

func TestProvisionerVersionSupportsDynamicParameters(t *testing.T) {
	t.Parallel()

	for v, dyn := range map[string]bool{
		"":     false,
		"na":   false,
		"0.0":  false,
		"0.10": false,
		"1.4":  false,
		"1.5":  false,
		"1.6":  true,
		"1.7":  true,
		"1.8":  true,
		"2.0":  true,
		"2.17": true,
		"4.0":  true,
	} {
		t.Run(v, func(t *testing.T) {
			t.Parallel()

			does := dynamicparameters.ProvisionerVersionSupportsDynamicParameters(v)
			require.Equal(t, dyn, does)
		})
	}
}
