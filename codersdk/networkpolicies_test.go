package codersdk_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestNetworkPolicyClient(t *testing.T) {
	t.Parallel()
	organization, policyID, revisionID, targetID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	expected := codersdk.PublishNetworkPolicyRequest{RevisionID: revisionID, TargetIDs: []uuid.UUID{targetID}, Operation: codersdk.NetworkPolicyOperationPlan, ExpectedGeneration: 7}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/experimental/organizations/"+organization.String()+"/network-policies/"+policyID.String()+"/publications", r.URL.Path)
		var request codersdk.PublishNetworkPolicyRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, expected, request)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		require.NoError(t, json.NewEncoder(w).Encode([]codersdk.NetworkPolicyRun{{RevisionID: revisionID, Operation: codersdk.NetworkPolicyOperationPlan, Status: "pending"}}))
	}))
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := codersdk.New(serverURL)
	runs, err := client.PublishNetworkPolicy(t.Context(), organization, policyID, expected)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, revisionID, runs[0].RevisionID)
	require.Equal(t, "pending", runs[0].Status)
}

func TestNetworkPolicyDeleteClient(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusOK, http.StatusAccepted} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			organization, policyID := uuid.New(), uuid.New()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodDelete, r.Method)
				require.Equal(t, "/api/experimental/organizations/"+organization.String()+"/network-policies/"+policyID.String(), r.URL.Path)
				var request codersdk.DeleteNetworkPolicyRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				require.EqualValues(t, 7, request.ExpectedGeneration)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				require.NoError(t, json.NewEncoder(w).Encode(codersdk.NetworkPolicy{ID: policyID, Generation: 8}))
			}))
			t.Cleanup(server.Close)
			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)
			p, err := codersdk.New(serverURL).DeleteNetworkPolicy(t.Context(), organization, policyID, codersdk.DeleteNetworkPolicyRequest{ExpectedGeneration: 7})
			require.NoError(t, err)
			require.Equal(t, policyID, p.ID)
			require.EqualValues(t, 8, p.Generation)
		})
	}
}

func TestNetworkPolicyClientConflict(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		require.NoError(t, json.NewEncoder(w).Encode(codersdk.Response{Message: "Target has an active job."}))
	}))
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := codersdk.New(serverURL)
	_, err = client.PublishNetworkPolicy(t.Context(), uuid.New(), uuid.New(), codersdk.PublishNetworkPolicyRequest{})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
}
