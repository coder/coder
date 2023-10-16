package httpmw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

const (
	testParam            = "workspaceagent"
	testWorkspaceAgentID = "8a70c576-12dc-42bc-b791-112a32b5bd43"
)

func TestParseUUID_Valid(t *testing.T) {
	t.Parallel()

	rw := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/{workspaceagent}", nil)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add(testParam, testWorkspaceAgentID)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))

	parsed, ok := ParseUUIDParam(rw, r, "workspaceagent")
	assert.True(t, ok, "UUID should be parsed")
	assert.Equal(t, testWorkspaceAgentID, parsed.String())
}

func TestParseUUID_Invalid(t *testing.T) {
	t.Parallel()

	rw := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/{workspaceagent}", nil)

	ctx := chi.NewRouteContext()
	ctx.URLParams.Add(testParam, "wrong-id")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))

	_, ok := ParseUUIDParam(rw, r, "workspaceagent")
	assert.False(t, ok, "UUID should not be parsed")
	assert.Equal(t, http.StatusBadRequest, rw.Code)

	var response codersdk.Response
	err := json.Unmarshal(rw.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response.Message, `Invalid UUID "wrong-id"`)
}
