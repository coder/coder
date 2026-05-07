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
		// RC versions: preserve the -rc.X suffix, strip build metadata.
		{
			Version:                    "v2.32.0-rc.1+abc123",
			ExpectedAfterStrippingInfo: "v2.32.0-rc.1",
		},
		{
			Version:                    "v2.32.0-rc.0",
			ExpectedAfterStrippingInfo: "v2.32.0-rc.0",
		},
		{
			Version:                    "v2.32.0-rc.1+683a720-devel",
			ExpectedAfterStrippingInfo: "v2.32.0-rc.1",
		},
		// Bare devel suffix, no build metadata.
		{
			Version:                    "v2.32.0-devel",
			ExpectedAfterStrippingInfo: "v2.32.0",
		},
		// Plain release, identity case.
		{
			Version:                    "v2.16.0",
			ExpectedAfterStrippingInfo: "v2.16.0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Version, func(t *testing.T) {
			t.Parallel()
			stripped := removeTrailingVersionInfo(tc.Version)
			require.Equal(t, tc.ExpectedAfterStrippingInfo, stripped)
		})
	}
}
