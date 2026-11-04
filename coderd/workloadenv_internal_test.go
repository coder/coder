package coderd

import (
	"bytes"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestWorkloadEnv(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	req := f.request()
	req.Env = map[string]string{"TOKEN": "private-env-secret", "EMPTY": "", "MULTILINE": "a=b,c\nnext line\n"}
	f.startHook = func(_ http.ResponseWriter, r *http.Request) bool {
		data, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		var dispatched codersdk.StartHostWorkloadRequest
		assert.NoError(t, json.Unmarshal(data, &dispatched))
		assert.Equal(t, req.Env, dispatched.Env)
		r.Body = io.NopCloser(bytes.NewReader(data))
		return false
	}
	f.register(t)
	created := f.call(t, "owner", http.MethodPost, "/workloads", req)
	require.Equal(t, http.StatusCreated, created.Code)
	require.NotContains(t, created.Body.String(), req.Env["TOKEN"])
	require.NotContains(t, created.Body.String(), `"env"`)
	retry := req
	retry.Env = maps.Clone(req.Env)
	duplicate := f.call(t, "owner", http.MethodPost, "/workloads", retry)
	require.Equal(t, http.StatusOK, duplicate.Code)
	require.JSONEq(t, created.Body.String(), duplicate.Body.String())
	retry.Env["TOKEN"] = "changed-env-secret"
	conflict := f.call(t, "owner", http.MethodPost, "/workloads", retry)
	require.Equal(t, http.StatusConflict, conflict.Code)
	require.NotContains(t, conflict.Body.String(), req.Env["TOKEN"])
	require.NotContains(t, conflict.Body.String(), retry.Env["TOKEN"])
	require.EqualValues(t, 1, f.starts.Load())
	output := f.call(t, "owner", http.MethodGet, "/workloads/"+req.ID.String(), nil)
	require.Equal(t, http.StatusOK, output.Code)
	require.NotContains(t, output.Body.String(), req.Env["TOKEN"])
	require.NotContains(t, output.Body.String(), `"env"`)
}

func TestWorkloadEnvRejected(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	req := f.request()
	req.Env = map[string]string{"TOKEN": "private-env-secret\x00"}
	response := f.call(t, "owner", http.MethodPost, "/workloads", req)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.NotContains(t, response.Body.String(), "private-env-secret")
	require.Zero(t, f.connections.Load())
	require.Empty(t, f.api.workloads.runs)
}
