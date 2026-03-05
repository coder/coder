package wsbuilder_test

import (
	"net/http"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/coderd/wsbuilder"
)

func TestBuildErrorResponseDelegation(t *testing.T) {
	t.Parallel()

	t.Run("plain_error", func(t *testing.T) {
		t.Parallel()

		be := wsbuilder.BuildError{
			Status:  http.StatusBadRequest,
			Message: "bad",
			Wrapped: xerrors.New("oops"),
		}

		status, resp := be.Response()
		require.Equal(t, http.StatusBadRequest, status)
		require.Equal(t, "bad", resp.Message)
		require.Contains(t, resp.Detail, "oops")
		require.Empty(t, resp.Validations)
	})

	t.Run("responder_error", func(t *testing.T) {
		t.Parallel()

		inner := &dynamicparameters.DiagnosticError{
			Message: "resolve parameters",
			KeyedDiagnostics: map[string]hcl.Diagnostics{
				"param1": {
					{
						Severity: hcl.DiagError,
						Summary:  "required parameter",
					},
				},
			},
		}

		be := wsbuilder.BuildError{
			Status:  http.StatusBadRequest,
			Message: "build error wrapper",
			Wrapped: inner,
		}

		status, resp := be.Response()

		// Should delegate to the inner DiagnosticError's response.
		innerStatus, innerResp := inner.Response()
		require.Equal(t, innerStatus, status)
		require.Equal(t, innerResp.Message, resp.Message)
		require.Len(t, resp.Validations, 1)
		require.Equal(t, "param1", resp.Validations[0].Field)
	})
}
