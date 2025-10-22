package coderd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_validateProxyURL(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		Name          string
		URL           string
		ExpectedError bool
	}{
		{
			Name:          "Empty",
			URL:           "",
			ExpectedError: true,
		},
		{
			Name:          "EmptyWild",
			URL:           "",
			ExpectedError: true,
		},
		{
			Name:          "URL",
			URL:           "https://example.com",
			ExpectedError: false,
		},
		{
			Name:          "NoScheme",
			URL:           "example.com",
			ExpectedError: true,
		},
		{
			Name:          "BadScheme",
			URL:           "ssh://example.com",
			ExpectedError: true,
		},
		{
			Name:          "IncludePaths",
			URL:           "https://example.com/test",
			ExpectedError: true,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			err := validateProxyURL(tt.URL)
			if tt.ExpectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
