package httperror_test

import (
	"database/sql"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/codersdk"
)

func TestWrapResponseError(t *testing.T) {
	t.Parallel()

	resp := codersdk.Response{Message: "Not found."}
	err := xerrors.Errorf("lookup: %w", httperror.WrapResponseError(http.StatusNotFound, resp, sql.ErrNoRows))

	require.ErrorIs(t, err, sql.ErrNoRows)
	responder, ok := httperror.IsResponder(err)
	require.True(t, ok)
	status, got := responder.Response()
	require.Equal(t, http.StatusNotFound, status)
	require.Equal(t, resp, got)
	require.Equal(t, http.StatusNotFound, httperror.StatusCode(err))
}

func TestStatusCode(t *testing.T) {
	t.Parallel()

	require.Equal(t, http.StatusInternalServerError, httperror.StatusCode(xerrors.New("plain")))
}
