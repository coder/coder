package codersdk

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoveTrailingVersionInfo(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Version                    string
		ExpectedAfterStrippingInfo string
	}{
		{
			Version:                    "v2.16.0+683a720",
			ExpectedAfterStrippingInfo: "v2.16.0",
		},
		{
			Version:                    "v2.16.0-devel+683a720",
			ExpectedAfterStrippingInfo: "v2.16.0",
		},
		{
			Version:                    "v2.16.0+683a720-devel",
			ExpectedAfterStrippingInfo: "v2.16.0",
		},
	}

	for _, tc := range testCases {
		tc := tc

		stripped := removeTrailingVersionInfo(tc.Version)
		require.Equal(t, tc.ExpectedAfterStrippingInfo, stripped)
	}
}
