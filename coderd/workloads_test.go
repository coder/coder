package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkloadPublicAuthentication(t *testing.T) {
	t.Parallel()
	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentWorkloads)}
	client := coderdtest.New(t, &coderdtest.Options{DeploymentValues: dv})
	admin := coderdtest.CreateFirstUser(t, client)
	member, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
	anonymous := codersdk.New(client.URL, codersdk.WithHTTPClient(client.HTTPClient))
	base := "/api/v2/organizations/" + admin.OrganizationID.String()
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/workload-hosts"},
		{http.MethodDelete, "/workload-hosts/" + uuid.NewString()},
		{http.MethodPost, "/workloads"},
		{http.MethodGet, "/workloads/" + uuid.NewString()},
		{http.MethodGet, "/workloads/" + uuid.NewString() + "?follow=true"},
		{http.MethodGet, "/workloads/" + uuid.NewString() + "/input"},
		{http.MethodDelete, "/workloads/" + uuid.NewString()},
	} {
		t.Run(route.method+route.path, func(t *testing.T) {
			t.Parallel()
			res, err := anonymous.Request(testutil.Context(t, testutil.WaitLong), route.method, base+route.path, nil)
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusUnauthorized, res.StatusCode)
		})
	}
	_, err := member.RegisterWorkloadHost(testutil.Context(t, testutil.WaitLong), admin.OrganizationID, codersdk.RegisterWorkloadHostRequest{AgentID: uuid.New(), Token: "not-a-valid-supervisor-token"})
	require.Error(t, err)
	var sdkError *codersdk.Error
	require.ErrorAs(t, err, &sdkError)
	require.Equal(t, http.StatusNotFound, sdkError.StatusCode())
}

func TestWorkloadPublicExperiment(t *testing.T) {
	t.Parallel()
	for _, experiments := range [][]string{nil, {"*"}} {
		name := "Disabled"
		if len(experiments) > 0 {
			name = "WildcardExcluded"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dv := coderdtest.DeploymentValues(t)
			dv.Experiments = experiments
			client := coderdtest.New(t, &coderdtest.Options{DeploymentValues: dv})
			admin := coderdtest.CreateFirstUser(t, client)
			_, err := client.CreateWorkload(testutil.Context(t, testutil.WaitLong), admin.OrganizationID, codersdk.CreateWorkloadRequest{})
			require.Error(t, err)
			var sdkError *codersdk.Error
			require.ErrorAs(t, err, &sdkError)
			require.Equal(t, http.StatusForbidden, sdkError.StatusCode())
		})
	}
}
