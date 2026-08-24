package codersdk_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestExperimentalClientChatModelACL(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	modelID := uuid.New()
	userID := uuid.New()
	groupID := uuid.New()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/experimental/organizations/"+organizationID.String()+"/chats/models/"+modelID.String()+"/acl", r.URL.Path)
		http.Error(rw, `{"user_roles":{"`+userID.String()+`":"read"},"group_roles":{"`+groupID.String()+`":"read"}}`, http.StatusOK)
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := codersdk.NewExperimentalClient(codersdk.New(serverURL))

	modelACL, err := client.ChatModelACL(context.Background(), organizationID, modelID)
	require.NoError(t, err)
	require.Equal(t, map[string]codersdk.ChatRole{userID.String(): codersdk.ChatRoleRead}, modelACL.UserRoles)
	require.Equal(t, map[string]codersdk.ChatRole{groupID.String(): codersdk.ChatRoleRead}, modelACL.GroupRoles)
}

func TestExperimentalClientUpdateChatModelACL(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	modelID := uuid.New()
	userID := uuid.New()
	groupID := uuid.New()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		require.Equal(t, "/api/experimental/organizations/"+organizationID.String()+"/chats/models/"+modelID.String()+"/acl", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var payload map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(body, &payload))
		require.JSONEq(t, `{"`+userID.String()+`":"read"}`, string(payload["user_roles"]))
		require.JSONEq(t, `{"`+groupID.String()+`":""}`, string(payload["group_roles"]))
		rw.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := codersdk.NewExperimentalClient(codersdk.New(serverURL))

	err = client.UpdateChatModelACL(context.Background(), organizationID, modelID, codersdk.UpdateChatModelACLRequest{
		UserRoles:  map[string]codersdk.ChatRole{userID.String(): codersdk.ChatRoleRead},
		GroupRoles: map[string]codersdk.ChatRole{groupID.String(): codersdk.ChatRoleDeleted},
	})
	require.NoError(t, err)
}

func TestUpdateChatModelACLRequestOmittedMaps(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(codersdk.UpdateChatModelACLRequest{})
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(payload))

	payload, err = json.Marshal(codersdk.UpdateChatModelACLRequest{
		UserRoles:  map[string]codersdk.ChatRole{},
		GroupRoles: map[string]codersdk.ChatRole{},
	})
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(payload))
}
