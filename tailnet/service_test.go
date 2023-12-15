package tailnet_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/tailnet"
)

func TestValidateVersion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		version   string
		supported bool
	}{
		{
			name:      "Current",
			version:   fmt.Sprintf("%d.%d", tailnet.CurrentMajor, tailnet.CurrentMinor),
			supported: true,
		},
		{
			name:    "TooNewMinor",
			version: fmt.Sprintf("%d.%d", tailnet.CurrentMajor, tailnet.CurrentMinor+1),
		},
		{
			name:    "TooNewMajor",
			version: fmt.Sprintf("%d.%d", tailnet.CurrentMajor+1, tailnet.CurrentMinor),
		},
		{
			name:      "1.0",
			version:   "1.0",
			supported: true,
		},
		{
			name:      "2.0",
			version:   "2.0",
			supported: true,
		},
		{
			name:    "Malformed0",
			version: "cats",
		},
		{
			name:    "Malformed1",
			version: "cats.dogs",
		},
		{
			name:    "Malformed2",
			version: "1.0.1",
		},
		{
			name:    "Malformed3",
			version: "11",
		},
		{
			name:    "TooOld",
			version: "0.8",
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tailnet.ValidateVersion(tc.version)
			if tc.supported {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
