package codersdk_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestWorkloadHostHistoryClient(t *testing.T) {
	t.Parallel()
	org, agent := uuid.New(), uuid.New()
	request := codersdk.StartHostWorkloadRequest{CreateWorkloadRequest: codersdk.CreateWorkloadRequest{
		ID: uuid.New(), TemplateID: uuid.New(), CPU: 2, MemoryBytes: 4 << 30, Command: "private-command\necho 'second line'",
	}, OrganizationID: org, OwnerID: uuid.New(), GenerationID: "prepared-image"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v2/organizations/" + org.String() + "/workload-hosts/" + agent.String() + "/stats":
			_ = json.NewEncoder(w).Encode(codersdk.WorkloadHostStats{Total: 1, Succeeded: 1})
		case "/api/v2/organizations/" + org.String() + "/workload-hosts/" + agent.String() + "/history":
			assert.Equal(t, "offset=0&limit=1", r.URL.RawQuery)
			_ = json.NewEncoder(w).Encode(codersdk.WorkloadHostHistory{Total: 1, NextOffset: 1, Items: []codersdk.WorkloadHistoryEntry{{Request: request}}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := codersdk.New(u, codersdk.WithLogBodies())
	var logs bytes.Buffer
	client.PlainLogger = &logs
	stats, err := client.WorkloadHostStats(t.Context(), org, agent)
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkloadHostStats{Total: 1, Succeeded: 1}, stats)
	history, err := client.WorkloadHostHistory(t.Context(), org, agent, 0, 1)
	require.NoError(t, err)
	require.Len(t, history.Items, 1)
	require.Equal(t, request, history.Items[0].Request)
	require.Equal(t, int64(1), history.Total)
	require.Equal(t, 1, history.NextOffset)
	require.NotContains(t, logs.String(), "private-command")
	require.True(t, client.LogBodies())
}
