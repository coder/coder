package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

func TestStatsNearestRank(t *testing.T) {
	t.Parallel()
	values := make([]float64, 100)
	for i := range values {
		values[i] = float64(100 - i)
	}
	require.Equal(t, distribution{Count: 100, P50: 50, P95: 95, P99: 99}, stats(values))
	require.Equal(t, float64(100), values[0], "statistics must not mutate caller samples")
	require.Equal(t, distribution{}, stats(nil))
	require.Equal(t, distribution{Count: 1, P50: 7, P95: 7, P99: 7}, stats([]float64{7}))
}

func TestSummarizeFailuresAndTargetBoundary(t *testing.T) {
	t.Parallel()
	fast, boundary := 1000.0, 5000.0
	for _, tc := range []struct {
		name     string
		extra    sample
		failures int
	}{
		{name: "Timeout", extra: sample{Error: "context deadline exceeded", AttemptMS: 60000}, failures: 1},
		{name: "MissingMeasurement", extra: sample{AttemptMS: 60000}, failures: 1},
		{name: "Boundary", extra: sample{ReadyMS: &boundary, AttemptMS: boundary}},
		{name: "Cleanup", extra: sample{ReadyMS: &fast, AttemptMS: fast, CleanupError: "runtime unavailable"}},
		{name: "Preparation", extra: sample{ReadyMS: &fast, AttemptMS: fast, PrepareError: "exit status 1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.extra.Group = "sequential"
			s := summarize("sequential", []sample{{Group: "sequential", ReadyMS: &fast, AttemptMS: fast}, tc.extra, {Group: "burst", Error: "irrelevant"}}, 5000)
			require.Equal(t, 2, s.Attempts)
			require.Equal(t, 2, s.AllAttemptsMS.Count)
			require.Equal(t, tc.failures, s.ReadinessFailures)
			require.Equal(t, 2-tc.failures, s.SuccessfulReadinessMS.Count)
			require.False(t, s.Passed)
		})
	}
	require.False(t, summarize("sequential", nil, 5000).Passed)
	require.True(t, summarize("sequential", []sample{{Group: "sequential", ReadyMS: &fast, AttemptMS: fast}}, 5000).Passed)
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func testClient(t *testing.T, handler func(*http.Request) (int, any)) *codersdk.Client {
	t.Helper()
	client := codersdk.New(&url.URL{Scheme: "http", Host: "sandbox-test.invalid"})
	client.HTTPClient = &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		status, body := handler(req)
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(data))), Request: req}, nil
	})}
	return client
}

func TestFailedProvisionIsRecordedAndCleanedAfterCancellation(t *testing.T) {
	t.Parallel()
	wsID, buildID, deletionID := uuid.New(), uuid.New(), uuid.New()
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	deleted := false
	workspace := codersdk.Workspace{ID: wsID, LatestBuild: codersdk.WorkspaceBuild{ID: buildID, Job: codersdk.ProvisionerJob{Status: codersdk.ProvisionerJobFailed, Error: "runtime unavailable"}}}
	client := testClient(t, func(req *http.Request) (int, any) {
		switch req.Method + " " + req.URL.Path {
		case "POST /api/v2/users/me/workspaces":
			cancel()
			return http.StatusCreated, workspace
		case "GET /api/v2/workspacebuilds/" + buildID.String() + "/timings":
			require.NoError(t, req.Context().Err(), "failed samples should still retrieve server timings")
			return http.StatusOK, codersdk.WorkspaceBuildTimings{}
		case "GET /api/v2/workspaces/" + wsID.String():
			require.NoError(t, req.Context().Err(), "cleanup must use an independent context")
			return http.StatusOK, workspace
		case "POST /api/v2/workspaces/" + wsID.String() + "/builds":
			var request codersdk.CreateWorkspaceBuildRequest
			require.NoError(t, json.NewDecoder(req.Body).Decode(&request))
			require.Equal(t, codersdk.WorkspaceTransitionDelete, request.Transition)
			require.False(t, request.Orphan)
			deleted = true
			return http.StatusCreated, codersdk.WorkspaceBuild{ID: deletionID}
		case "GET /api/v2/workspacebuilds/" + deletionID.String():
			return http.StatusOK, codersdk.WorkspaceBuild{ID: deletionID, Job: codersdk.ProvisionerJob{Status: codersdk.ProvisionerJobSucceeded}}
		default:
			t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
			return http.StatusNotFound, codersdk.Response{Message: "unexpected request"}
		}
	})
	s := runSample(parent, client, config{templateID: uuid.New(), timeout: time.Second, cleanupTimeout: time.Second, poll: time.Millisecond}, "sequential", 1)
	require.Equal(t, "provision", s.FailureStage)
	require.Contains(t, s.Error, "runtime unavailable")
	require.Nil(t, s.ReadyMS)
	require.Equal(t, wsID, s.WorkspaceID)
	require.Equal(t, buildID, s.BuildID)
	require.Empty(t, s.CleanupError)
	require.True(t, deleted)
}

func TestCleanupCancelsRunningBuildAndReportsDeletionFailure(t *testing.T) {
	t.Parallel()
	wsID, buildID, deletionID := uuid.New(), uuid.New(), uuid.New()
	canceled := false
	client := testClient(t, func(req *http.Request) (int, any) {
		switch req.Method + " " + req.URL.Path {
		case "GET /api/v2/workspaces/" + wsID.String():
			return http.StatusOK, codersdk.Workspace{ID: wsID, LatestBuild: codersdk.WorkspaceBuild{ID: buildID, Job: codersdk.ProvisionerJob{Status: codersdk.ProvisionerJobRunning}}}
		case "PATCH /api/v2/workspacebuilds/" + buildID.String() + "/cancel":
			canceled = true
			return http.StatusOK, codersdk.Response{}
		case "GET /api/v2/workspacebuilds/" + buildID.String():
			require.True(t, canceled)
			return http.StatusOK, codersdk.WorkspaceBuild{ID: buildID, Job: codersdk.ProvisionerJob{Status: codersdk.ProvisionerJobCanceled}}
		case "POST /api/v2/workspaces/" + wsID.String() + "/builds":
			return http.StatusCreated, codersdk.WorkspaceBuild{ID: deletionID}
		case "GET /api/v2/workspacebuilds/" + deletionID.String():
			return http.StatusOK, codersdk.WorkspaceBuild{ID: deletionID, Job: codersdk.ProvisionerJob{Status: codersdk.ProvisionerJobFailed, Error: "host disconnected"}}
		default:
			t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
			return http.StatusNotFound, codersdk.Response{}
		}
	})
	s := sample{WorkspaceID: wsID, BuildID: buildID, WorkspaceName: "still-needs-cleanup"}
	err := cleanup(t.Context(), client, &s, nil, time.Millisecond)
	require.ErrorContains(t, err, "host disconnected")
	require.Equal(t, wsID, s.WorkspaceID)
	require.Equal(t, "still-needs-cleanup", s.WorkspaceName)
}

func TestCanceledRunRetainsMissingSampleCount(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r := run(ctx, nil, config{runs: 100, bursts: 25, concurrency: 4, target: 5 * time.Second})
	require.False(t, r.Passed)
	require.Equal(t, 200, r.NotStarted)
	require.Empty(t, r.Samples)
}

func TestCleanupUncertainCreateKeepsWorkspaceName(t *testing.T) {
	t.Parallel()
	client := testClient(t, func(*http.Request) (int, any) { return http.StatusNotFound, codersdk.Response{Message: "not found"} })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s := sample{WorkspaceName: "uncertain-create"}
	err := cleanup(ctx, client, &s, xerrors.New("connection lost"), time.Millisecond)
	require.ErrorContains(t, err, "creation outcome uncertain")
	require.ErrorContains(t, err, s.WorkspaceName)
}

func TestCommandUsesAuthenticatedCoderAgentConnection(t *testing.T) {
	t.Parallel()
	agentID := uuid.New()
	requested := false
	client := testClient(t, func(req *http.Request) (int, any) {
		require.Equal(t, "/api/v2/workspaceagents/"+agentID.String()+"/connection", req.URL.Path)
		require.Equal(t, "synthetic-test-token", req.Header.Get(codersdk.SessionTokenHeader))
		requested = true
		return http.StatusUnauthorized, codersdk.Response{Message: "unauthorized"}
	})
	client.SetSessionToken("synthetic-test-token")
	_, err := command(t.Context(), client, agentID, "true")
	require.ErrorContains(t, err, "get connection info")
	require.True(t, requested, "benchmark must connect through Coder authorization")
}

func TestRuntimeTimingExcludesLifecycleOverhead(t *testing.T) {
	t.Parallel()
	start := time.Now()
	buildID := uuid.New()
	client := testClient(t, func(req *http.Request) (int, any) {
		require.Equal(t, "/api/v2/workspacebuilds/"+buildID.String()+"/timings", req.URL.Path)
		return http.StatusOK, codersdk.WorkspaceBuildTimings{ProvisionerTimings: []codersdk.ProvisionerTiming{
			{Stage: codersdk.TimingStageApply, Source: "sandbox", Action: "runtime lifecycle", StartedAt: start, EndedAt: start.Add(time.Second)},
			{Stage: codersdk.TimingStageApply, Source: "containerd", Action: "create", StartedAt: start, EndedAt: start.Add(25 * time.Millisecond)},
			{Stage: codersdk.TimingStageApply, Source: "containerd", Action: "delete", StartedAt: start, EndedAt: start.Add(50 * time.Millisecond)},
		}}
	})
	s := sample{BuildID: buildID}
	collectTimings(client, &s, time.Second)
	require.Empty(t, s.TimingsError)
	require.NotNil(t, s.RuntimeMS)
	require.Equal(t, 25.0, *s.RuntimeMS)
	require.Len(t, s.ServerTimings.ProvisionerTimings, 3, "raw timings retain the surrounding lifecycle measurement")
}

func TestMissingRuntimeTimingFailsSuccessfulSample(t *testing.T) {
	t.Parallel()
	client := testClient(t, func(*http.Request) (int, any) {
		return http.StatusOK, codersdk.WorkspaceBuildTimings{}
	})
	readyMS := 1000.0
	s := sample{BuildID: uuid.New(), Group: "sequential", ReadyMS: &readyMS}
	collectTimings(client, &s, time.Second)
	require.Equal(t, "containerd create timing was not returned", s.TimingsError)
	require.Nil(t, s.RuntimeMS)
	require.False(t, summarize("sequential", []sample{s}, 5000).Passed)
}

func TestReportFileIsPrivate(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "results.json")
	// Begin with public permissions to verify the report writer corrects them.
	require.NoError(t, os.WriteFile(path, []byte("old results"), 0o644)) //nolint:gosec // Intentional insecure fixture for the permissions regression test.
	r := report{TemplateID: uuid.New(), RequestedSamples: 200, NotStarted: 200}
	require.NoError(t, writeReport(path, r))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var decoded report
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, r.TemplateID, decoded.TemplateID)
	require.Equal(t, 200, decoded.RequestedSamples)
}

func TestRunRetainsEveryConcurrentFailure(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	client := testClient(t, func(req *http.Request) (int, any) {
		if req.Method != "POST" || req.URL.Path != "/api/v2/users/me/workspaces" {
			t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
		}
		requests.Add(1)
		return http.StatusBadRequest, codersdk.Response{Message: "template unavailable"}
	})
	r := run(t.Context(), client, config{templateID: uuid.New(), runs: 2, bursts: 2, concurrency: 4, timeout: time.Second, cleanupTimeout: time.Second, target: 5 * time.Second})
	require.Equal(t, int32(10), requests.Load())
	require.Len(t, r.Samples, 10)
	require.Zero(t, r.NotStarted)
	require.False(t, r.Passed)
	require.Equal(t, 2, r.Summaries[0].ReadinessFailures)
	require.Equal(t, 8, r.Summaries[1].ReadinessFailures)
}
