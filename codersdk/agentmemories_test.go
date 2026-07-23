package codersdk_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestAgentMemoriesClient(t *testing.T) {
	t.Parallel()

	memoryID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/experimental/users/me/agent-memories", r.URL.Path)
		require.Equal(t, "/projects", r.URL.Query().Get("directory"))
		require.Equal(t, "25", r.URL.Query().Get("offset"))
		_ = json.NewEncoder(rw).Encode(codersdk.AgentMemoryChildrenResponse{
			Entries:    []codersdk.AgentMemoryEntry{{Kind: codersdk.AgentMemoryEntryKindMemory, Path: "/projects/a.md", ID: new(memoryID)}},
			NextOffset: new(int32(50)),
		})
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := codersdk.NewExperimentalClient(codersdk.New(serverURL))
	response, err := client.AgentMemoryChildren(context.Background(), "me", "/projects", 25)
	require.NoError(t, err)
	require.Equal(t, int32(50), *response.NextOffset)
	require.Equal(t, memoryID, *response.Entries[0].ID)
}

func TestUpdateAgentMemoryClient(t *testing.T) {
	t.Parallel()

	memoryID := uuid.New()
	expected := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		require.Equal(t, "/api/experimental/users/alice/agent-memories/"+memoryID.String(), r.URL.Path)
		var request codersdk.UpdateAgentMemoryRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, "updated", request.Content)
		require.True(t, expected.Equal(request.ExpectedUpdatedAt))
		_ = json.NewEncoder(rw).Encode(codersdk.AgentMemory{ID: memoryID, Path: "/memory.md", Content: request.Content, UpdatedAt: expected})
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := codersdk.NewExperimentalClient(codersdk.New(serverURL))
	memory, err := client.UpdateAgentMemory(context.Background(), "alice", memoryID, codersdk.UpdateAgentMemoryRequest{Content: "updated", ExpectedUpdatedAt: expected})
	require.NoError(t, err)
	require.Equal(t, "updated", memory.Content)
}

func TestAgentMemoryClientError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		http.Error(rw, `{"message":"stale"}`, http.StatusConflict)
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := codersdk.NewExperimentalClient(codersdk.New(serverURL))
	_, err = client.UpdateAgentMemory(context.Background(), "me", uuid.New(), codersdk.UpdateAgentMemoryRequest{})
	require.Error(t, err)
	var apiErr *codersdk.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusConflict, apiErr.StatusCode())
}
