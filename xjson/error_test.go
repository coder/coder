package xjson

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
	"golang.org/x/xerrors"

	"github.com/coder/coder/srverr"
)

func TestDefaultErrorParams(t *testing.T) {
	t.Parallel()

	t.Run("VerboseDetails", func(t *testing.T) {
		var (
			testMsg = "Testing."
			testErr = xerrors.Errorf("testing verbose")
		)
		p := defaultErrorParams{
			msg:     testMsg,
			verbose: testErr,
		}

		actualResponse, err := p.MarshalJSON()
		require.NoError(t, err, "marshal error params")

		expectedResponse, err := json.Marshal(errorResponse{
			Error: errorPayload{
				Msg:  testMsg,
				Code: codeVerbose,
				Details: detailsVerbose{
					Verbose: testErr.Error(),
				},
			},
		})
		require.NoError(t, err, "marshal expected response")
		require.Equal(t, expectedResponse, actualResponse, "responses differ")
	})

	t.Run("EmptyDetails", func(t *testing.T) {
		var (
			testMsg = "Testing."
		)
		p := defaultErrorParams{
			msg: testMsg,
		}

		actualResponse, err := p.MarshalJSON()
		require.NoError(t, err, "marshal error params")

		expectedResponse, err := json.Marshal(errorResponse{
			Error: errorPayload{
				Msg:  testMsg,
				Code: codeEmpty,
			},
		})
		require.NoError(t, err, "marshal expected response")
		require.Equal(t, string(expectedResponse), string(actualResponse), "responses differ")
	})
}

// Checking Xjson errors write the correct code
func Test_JsonErrors(t *testing.T) {
	t.Parallel()
	const testMessage = "test message"

	vs := []struct {
		Name               string
		Write              func(w http.ResponseWriter)
		ExpectedStatusCode int
		ErrorCode          srverr.Code
		RespContains       string
	}{
		{
			Name: "CustomBadRequest",
			Write: func(w http.ResponseWriter) {
				WriteBadRequestWithCode(w, "test", testMessage, nil)
			},
			ExpectedStatusCode: http.StatusBadRequest,
			RespContains:       testMessage,
			ErrorCode:          "test",
		},
		{
			Name: "BadRequest",
			Write: func(w http.ResponseWriter) {
				WriteBadRequest(w, testMessage)
			},
			ExpectedStatusCode: http.StatusBadRequest,
			RespContains:       testMessage,
		},
		{
			Name: "Unauthorized",
			Write: func(w http.ResponseWriter) {
				WriteUnauthorized(w, testMessage)
			},
			ExpectedStatusCode: http.StatusUnauthorized,
			RespContains:       testMessage,
		},
		{
			Name: "Forbidden",
			Write: func(w http.ResponseWriter) {
				WriteForbidden(w, testMessage)
			},
			ExpectedStatusCode: http.StatusForbidden,
			RespContains:       testMessage,
		},
		{
			Name: "Conflict",
			Write: func(w http.ResponseWriter) {
				WriteConflict(w, testMessage)
			},
			ExpectedStatusCode: http.StatusConflict,
			RespContains:       testMessage,
		},
		{
			Name: "PreconditionFailed",
			Write: func(w http.ResponseWriter) {
				WritePreconditionFailed(w, testMessage, xerrors.New("random"))
			},
			ExpectedStatusCode: http.StatusPreconditionFailed,
			RespContains:       testMessage,
			ErrorCode:          codeVerbose,
		},
		{
			Name: "FieldedPreconditionFailed",
			Write: func(w http.ResponseWriter) {
				WriteFieldedPreconditionFailed(w, testMessage, "this is a solution", xerrors.New("random"))
			},
			ExpectedStatusCode: http.StatusPreconditionFailed,
			RespContains:       testMessage,
			ErrorCode:          codeSolution,
		},
		{
			Name: "NotFound",
			Write: func(w http.ResponseWriter) {
				WriteNotFound(w, testMessage)
			},
			ExpectedStatusCode: http.StatusNotFound,
			RespContains:       testMessage,
		},
		{
			Name: "CustomNotFound",
			Write: func(w http.ResponseWriter) {
				WriteCustomNotFound(w, testMessage)
			},
			ExpectedStatusCode: http.StatusNotFound,
			RespContains:       testMessage,
		},
		{
			Name: "CustomServerError",
			Write: func(w http.ResponseWriter) {
				WriteCustomInternalServerError(w, testMessage, xerrors.New("random"))
			},
			ExpectedStatusCode: http.StatusInternalServerError,
			RespContains:       testMessage,
			ErrorCode:          codeVerbose,
		},
		{
			Name: "ServerError",
			Write: func(w http.ResponseWriter) {
				WriteInternalServerError(w, xerrors.New(testMessage))
			},
			ExpectedStatusCode: http.StatusInternalServerError,
			RespContains:       "server error",
			ErrorCode:          codeVerbose,
		},
	}

	for _, v := range vs {
		if v.ErrorCode == "" {
			v.ErrorCode = codeEmpty
		}
		t.Run(v.Name, func(t *testing.T) {
			w := httptest.NewRecorder()
			v.Write(w)

			resp := w.Result()
			require.Equal(t, v.ExpectedStatusCode, resp.StatusCode, "BadRequest")

			respErr := BodyError(resp)
			_ = resp.Body.Close()

			// Assure the body is a full json payload
			var eResp errorResponse
			err := json.Unmarshal(respErr.Body, &eResp)

			require.True(t, strings.Contains(respErr.Error(), v.RespContains), "contains")

			require.NoError(t, err, "body decode")
			require.Equal(t, v.ErrorCode, eResp.Error.Code, "correct code")
		})
	}
}

// Test_EmptyHTTPError checks to ensure the error works on empty responses.
// Athought the response is not an error code, the BodyError should still wrap the response data
// corectly.
func Test_EmptyHTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	r, err := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)
	require.NoError(t, err, "new request")
	resp, err := http.DefaultClient.Do(r)
	require.NoError(t, err, "GET request")

	e := BodyError(resp)
	require.Contains(t, e.Error(), strconv.Itoa(resp.StatusCode), "has status code")
}

// TestHTMLErrorPage tests that the embedded page is valid HTML.
func TestHTMLErrorPage(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	WriteErrPage(recorder, ErrPage{
		DevURL:    "https://*.master.cdr.dev",
		AccessURL: "https://master.cdr.dev",
	})

	node, err := html.Parse(recorder.Body)
	require.NoError(t, err, "HTML error page does not appear valid")
	require.NotNil(t, node, "require the node to be non-nil")
	require.Nil(t, node.Parent, "parent should be nil (root element)")
	require.Equal(t, html.DocumentNode, node.Type, "node is document")
}
