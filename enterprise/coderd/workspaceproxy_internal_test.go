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
		Wild          bool
		ExpectedError bool
	}{
		{
			Name:          "Empty",
			URL:           "",
			Wild:          false,
			ExpectedError: true,
		},
		{
			Name:          "EmptyWild",
			URL:           "",
			Wild:          true,
			ExpectedError: true,
		},
		{
			Name:          "URL",
			URL:           "https://example.com",
			Wild:          false,
			ExpectedError: false,
		},
		{
			Name:          "WildcardURL",
			URL:           "https://*.example.com",
			Wild:          true,
			ExpectedError: false,
		},
		{
			Name:          "URLMissingWild",
			URL:           "https://example.com",
			Wild:          true,
			ExpectedError: true,
		},
		{
			Name:          "NoScheme",
			URL:           "*.example.com",
			Wild:          true,
			ExpectedError: true,
		},
		{
			Name:          "BadScheme",
			URL:           "ssh://*.example.com",
			Wild:          true,
			ExpectedError: true,
		},
		{
			Name:          "IncludePaths",
			URL:           "https://*.example.com/test",
			Wild:          true,
			ExpectedError: true,
		},
	}

	for _, tt := range testcases {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			err := validateProxyURL(tt.URL, tt.Wild)
			if tt.ExpectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
