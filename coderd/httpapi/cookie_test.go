package httpapi_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpapi"
)

func TestStripCoderCookies(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		Input  string
		Output string
	}{{
		"testing=hello; wow=test",
		"testing=hello; wow=test",
	}, {
		"coder_session_token=moo; wow=test",
		"wow=test",
	}, {
		"another_token=wow; coder_session_token=ok",
		"another_token=wow",
	}, {
		"coder_session_token=ok; oauth_state=wow; oauth_redirect=/",
		"",
	}} {
		t.Run(tc.Input, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.Output, httpapi.StripCoderCookies(tc.Input))
		})
	}
}
