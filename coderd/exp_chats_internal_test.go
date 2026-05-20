package coderd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

func TestWriteWorkspaceAgentUploadError(t *testing.T) {
	t.Parallel()

	t.Run("AgentStatus", func(t *testing.T) {
		t.Parallel()

		res := &http.Response{
			StatusCode: http.StatusConflict,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Body: io.NopCloser(strings.NewReader(`{"message":"too many existing files"}`)),
		}
		err := codersdk.ReadBodyAsError(res)
		rw := httptest.NewRecorder()

		writeWorkspaceAgentUploadError(context.Background(), rw, err)

		require.Equal(t, http.StatusConflict, rw.Code)
		require.Contains(t, rw.Body.String(), "too many existing files")
	})

	t.Run("TransportError", func(t *testing.T) {
		t.Parallel()

		rw := httptest.NewRecorder()

		writeWorkspaceAgentUploadError(context.Background(), rw, xerrors.New("dial failed"))

		require.Equal(t, http.StatusBadGateway, rw.Code)
		require.Contains(t, rw.Body.String(), "Failed to upload file to workspace agent")
	})
}

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
