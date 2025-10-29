package aibridged

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/enterprise/aibridged/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestMCPRegex(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                  string
		allowRegex, denyRegex string
		expectedErr           error
	}{
		{
			name:        "invalid allow regex",
			allowRegex:  `\`,
			expectedErr: ErrCompileRegex,
		},
		{
			name:        "invalid deny regex",
			denyRegex:   `+`,
			expectedErr: ErrCompileRegex,
		},
		{
			name: "valid empty",
		},
		{
			name:       "valid",
			allowRegex: "(allowed|allowed2)",
			denyRegex:  ".*disallowed.*",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := testutil.Logger(t)
			f := NewMCPProxyFactory(logger, nil)

			_, err := f.newStreamableHTTPServerProxy(&proto.MCPServerConfig{
				Id:             "mock",
				Url:            "mock/mcp",
				ToolAllowRegex: tc.allowRegex,
				ToolDenyRegex:  tc.denyRegex,
			}, "")

			if tc.expectedErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.expectedErr)
			}
		})
	}
}
