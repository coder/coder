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

func TestClientChatModelACL(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	modelID := uuid.New()
	userID := uuid.New()
	groupID := uuid.New()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v2/organizations/"+organizationID.String()+"/chats/models/"+modelID.String()+"/acl", r.URL.Path)
		http.Error(rw, `{"users":[{"id":"`+userID.String()+`","username":"alice","name":"Alice","role":"read"}],"groups":[{"id":"`+groupID.String()+`","organization_id":"`+organizationID.String()+`","name":"developers","display_name":"Developers","members":[],"total_member_count":2,"role":"read"}]}`, http.StatusOK)
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := codersdk.New(serverURL)

	modelACL, err := client.ChatModelACL(context.Background(), organizationID, modelID)
	require.NoError(t, err)
	require.Equal(t, []codersdk.ChatUser{{
		MinimalUser: codersdk.MinimalUser{ID: userID, Username: "alice", Name: "Alice"},
		Role:        codersdk.ChatRoleRead,
	}}, modelACL.Users)
	require.Equal(t, groupID, modelACL.Groups[0].ID)
	require.Equal(t, organizationID, modelACL.Groups[0].OrganizationID)
	require.Equal(t, "developers", modelACL.Groups[0].Name)
	require.Equal(t, "Developers", modelACL.Groups[0].DisplayName)
	require.Equal(t, 2, modelACL.Groups[0].TotalMemberCount)
	require.Empty(t, modelACL.Groups[0].Members)
	require.Equal(t, codersdk.ChatRoleRead, modelACL.Groups[0].Role)
}

func TestClientUpdateChatModelACL(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	modelID := uuid.New()
	userID := uuid.New()
	groupID := uuid.New()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		require.Equal(t, "/api/v2/organizations/"+organizationID.String()+"/chats/models/"+modelID.String()+"/acl", r.URL.Path)
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
	client := codersdk.New(serverURL)

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
