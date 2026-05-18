package coderd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

func TestRewriteChatStartWorkspaceManualUpdateResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		resp           codersdk.Response
		fallbackDetail string
		wantDetail     string
	}{
		{
			name: "NoValidationsAndEmptyDetail",
			resp: codersdk.Response{
				Message: "missing required parameter",
			},
			fallbackDetail: "wrapped missing required parameter",
			wantDetail:     "missing required parameter",
		},
		{
			name: "NoValidationsAndExistingDetail",
			resp: codersdk.Response{
				Message: "missing required parameter",
				Detail:  "region must be set before the workspace can start",
			},
			fallbackDetail: "wrapped missing required parameter",
			wantDetail:     "missing required parameter: region must be set before the workspace can start",
		},
		{
			name: "ValidationsAndEmptyDetail",
			resp: codersdk.Response{
				Message: "missing required parameter",
				Validations: []codersdk.ValidationError{{
					Field:  "region",
					Detail: "region must be set before the workspace can start",
				}},
			},
			fallbackDetail: "wrapped missing required parameter",
			wantDetail:     "wrapped missing required parameter",
		},
		{
			name: "ValidationsAndExistingDetail",
			resp: codersdk.Response{
				Message: "missing required parameter",
				Detail:  "region must be set before the workspace can start",
				Validations: []codersdk.ValidationError{{
					Field:  "region",
					Detail: "region must be set before the workspace can start",
				}},
			},
			fallbackDetail: "wrapped missing required parameter",
			wantDetail:     "region must be set before the workspace can start",
		},
	}

	const retryInstructions = "Use read_template before retrying start_workspace."
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := rewriteChatStartWorkspaceManualUpdateResponse(tt.resp, tt.fallbackDetail, retryInstructions)
			require.Equal(t, retryInstructions, got.Message)
			require.Equal(t, tt.wantDetail, got.Detail)
			require.Equal(t, tt.resp.Validations, got.Validations)
		})
	}
}

func TestMaybeWriteManualTitleTimeoutErr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		wantWrote   bool
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "DeadlineExceededMapsTo504",
			err:         xerrors.Errorf("generate manual title: %w", context.DeadlineExceeded),
			wantWrote:   true,
			wantStatus:  http.StatusGatewayTimeout,
			wantMessage: "Title generation timed out. Try again or rename manually.",
		},
		{
			name:        "CanceledMapsTo499",
			err:         xerrors.Errorf("generate manual title: %w", context.Canceled),
			wantWrote:   true,
			wantStatus:  statusClientClosedRequest,
			wantMessage: "Title generation was canceled.",
		},
		{
			// Unrelated errors must fall through so the handler keeps
			// its existing 500 surface for genuine failures.
			name:      "UnrelatedErrorFallsThrough",
			err:       xerrors.New("something else"),
			wantWrote: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rw := httptest.NewRecorder()
			wrote := maybeWriteManualTitleTimeoutErr(context.Background(), rw, tt.err)
			require.Equal(t, tt.wantWrote, wrote)
			if !tt.wantWrote {
				require.Equal(t, http.StatusOK, rw.Code, "must not write a response when err is unrelated")
				return
			}
			require.Equal(t, tt.wantStatus, rw.Code)

			var resp codersdk.Response
			require.NoError(t, json.NewDecoder(rw.Body).Decode(&resp))
			require.Equal(t, tt.wantMessage, resp.Message)
			require.Empty(t, resp.Detail, "translated copy must not leak the raw error detail")
		})
	}
}
