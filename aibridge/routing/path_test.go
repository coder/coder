package routing_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/routing"
)

func TestValidateForwardPath(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		rawPath string
		wantErr bool
	}{
		{name: "EncodedDotThroughEscapedSlash", rawPath: "/models/safe%2F%2e%2e%2Fadmin", wantErr: true},
		{name: "EscapedSeparator", rawPath: "/models/a%2Fb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			u, err := url.Parse("http://example.test" + tc.rawPath)
			require.NoError(t, err)
			err = routing.ValidateForwardPath(u)
			if tc.wantErr {
				require.ErrorContains(t, err, "traversal segment")
				return
			}
			require.NoError(t, err)
		})
	}
}
