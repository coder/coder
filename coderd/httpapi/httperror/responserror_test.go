package httperror_test

import (
	"net/http"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/codersdk"
)

func TestResponseError_Unwrap_PreservesPQError(t *testing.T) {
	t.Parallel()

	inner := &pq.Error{Code: "40001"}
	wrapped := httperror.NewResponseWithError(
		http.StatusInternalServerError,
		codersdk.Response{Message: "boom", Detail: inner.Error()},
		inner,
	)

	var pqe *pq.Error
	require.True(t, xerrors.As(wrapped, &pqe),
		"xerrors.As must walk the responseError chain to the inner pq.Error")
	require.Equal(t, pq.ErrorCode("40001"), pqe.Code)
}

func TestResponseError_Unwrap_NoInner(t *testing.T) {
	t.Parallel()

	// NewResponseError carries no inner cause. errors.As against a
	// non-Responder target must not match anything.
	wrapped := httperror.NewResponseError(
		http.StatusBadRequest,
		codersdk.Response{Message: "validation failed"},
	)

	var pqe *pq.Error
	require.False(t, xerrors.As(wrapped, &pqe),
		"NewResponseError without inner err must not surface a spurious match")
}

func TestResponseError_Responder_Unaffected(t *testing.T) {
	t.Parallel()

	// IsResponder must keep returning the responseError regardless of
	// whether it was built with NewResponseError or NewResponseWithError.
	inner := xerrors.New("underlying cause")
	wrapped := httperror.NewResponseWithError(
		http.StatusInternalServerError,
		codersdk.Response{Message: "boom"},
		inner,
	)
	resp, ok := httperror.IsResponder(wrapped)
	require.True(t, ok)
	require.NotNil(t, resp)
	status, body := resp.Response()
	require.Equal(t, http.StatusInternalServerError, status)
	require.Equal(t, "boom", body.Message)
}
