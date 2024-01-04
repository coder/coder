package apiversion_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/apiversion"
)

func TestAPIVersionValidate(t *testing.T) {
	// Given
	v := apiversion.New([]int{2, 1}, 0)

	t.Parallel()
	for _, tc := range []struct {
		name          string
		version       string
		expectedError string
	}{
		{
			name:    "OK",
			version: "2.0",
		},
		{
			name:          "TooNewMinor",
			version:       "2.1",
			expectedError: "behind requested minor version",
		},
		{
			name:          "TooNewMajor",
			version:       "3.1",
			expectedError: "behind requested major version",
		},
		{
			name:    "1.0",
			version: "1.0",
		},
		{
			name:    "2.0",
			version: "2.0",
		},
		{
			name:          "Malformed0",
			version:       "cats",
			expectedError: "invalid version string",
		},
		{
			name:          "Malformed1",
			version:       "cats.dogs",
			expectedError: "invalid major version",
		},
		{
			name:          "Malformed2",
			version:       "1.dogs",
			expectedError: "invalid minor version",
		},
		{
			name:          "Malformed3",
			version:       "1.0.1",
			expectedError: "invalid version string",
		},
		{
			name:          "Malformed4",
			version:       "11",
			expectedError: "invalid version string",
		},
		{
			name:          "TooOld",
			version:       "0.8",
			expectedError: "no longer supported",
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := v.Validate(tc.version)
			if tc.expectedError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.expectedError)
			}
		})
	}
}
