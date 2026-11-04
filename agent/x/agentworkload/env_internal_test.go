//go:build linux

package agentworkload

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvAdmissionOwnership(t *testing.T) {
	t.Parallel()
	rt := newFake()
	rt.finish <- 0
	s := testSupervisor(t, testConfig(t), rt)
	req := testRequest()
	req.Env = map[string]string{"TOKEN": "private-env-secret"}
	_, status, err := s.admit(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)
	joinRuns(t, s)
	req.Env["TOKEN"] = "changed-env-secret"
	history := getHistory(t, s, "")
	require.Len(t, history.Items, 1)
	require.Equal(t, map[string]string{"TOKEN": "private-env-secret"}, history.Items[0].Request.Env)
	_, status, err = s.admit(req)
	require.Error(t, err)
	require.Equal(t, http.StatusConflict, status)
	require.NotContains(t, err.Error(), "private-env-secret")
	require.NotContains(t, err.Error(), "changed-env-secret")
}

func TestEnvAdmissionValidation(t *testing.T) {
	t.Parallel()
	s := testSupervisor(t, testConfig(t), newFake())
	req := testRequest()
	req.Env = map[string]string{"TOKEN": "private-env-secret\x00"}
	response := requestHTTP(t, s, http.MethodPost, "/workloads", req)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.NotContains(t, response.Body.String(), "private-env-secret")
	require.Empty(t, getHistory(t, s, "").Items)
}
