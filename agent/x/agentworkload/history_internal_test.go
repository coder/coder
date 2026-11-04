//go:build linux

package agentworkload

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func getStats(t *testing.T, s *Supervisor) codersdk.WorkloadHostStats {
	t.Helper()
	w := requestHTTP(t, s, http.MethodGet, "/stats", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var stats codersdk.WorkloadHostStats
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))
	return stats
}

func getHistory(t *testing.T, s *Supervisor, query string) codersdk.WorkloadHostHistory {
	t.Helper()
	w := requestHTTP(t, s, http.MethodGet, "/history"+query, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var history codersdk.WorkloadHostHistory
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &history))
	require.NotNil(t, history.Items)
	return history
}

func TestStatsPendingAndRejected(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	cfg.CPU = 1
	rt := newFake()
	gate := make(chan struct{})
	rt.createGate = gate
	s := testSupervisor(t, cfg, rt)
	require.Equal(t, codersdk.WorkloadHostStats{}, getStats(t, s))
	require.Equal(t, codersdk.WorkloadHostHistory{Items: []codersdk.WorkloadHistoryEntry{}}, getHistory(t, s, ""))

	req := testRequest()
	wrongGeneration := req
	wrongGeneration.GenerationID = "wrong"
	require.Equal(t, http.StatusConflict, requestHTTP(t, s, http.MethodPost, "/workloads", wrongGeneration).Code)
	invalid := req
	invalid.CPU = 0
	require.Equal(t, http.StatusBadRequest, requestHTTP(t, s, http.MethodPost, "/workloads", invalid).Code)
	require.Equal(t, codersdk.WorkloadHostStats{}, getStats(t, s))

	require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	receive(t, rt.created)
	pending := codersdk.WorkloadHostStats{Total: 1, Pending: 1}
	require.Equal(t, pending, getStats(t, s))
	require.Equal(t, http.StatusTooManyRequests, requestHTTP(t, s, http.MethodPost, "/workloads", testRequest()).Code)
	require.Equal(t, http.StatusOK, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	changed := req
	changed.Command = "not admitted"
	require.Equal(t, http.StatusConflict, requestHTTP(t, s, http.MethodPost, "/workloads", changed).Code)
	require.Equal(t, pending, getStats(t, s))
	history := getHistory(t, s, "")
	require.Equal(t, int64(1), history.Total)
	require.Equal(t, 1, history.NextOffset)
	require.Len(t, history.Items, 1)
	require.Equal(t, req, history.Items[0].Request)
	require.Equal(t, "requested", history.Items[0].Workload.State)

	close(gate)
	receive(t, rt.waiting)
	require.Equal(t, codersdk.WorkloadHostStats{Total: 1, Running: 1}, getStats(t, s))
	rt.finish <- 0
	joinRuns(t, s)
	require.Equal(t, codersdk.WorkloadHostStats{Total: 1, Succeeded: 1}, getStats(t, s))
}

func TestStatsStates(t *testing.T) {
	t.Parallel()
	s := testSupervisor(t, testConfig(t), newFake())
	// Synthetic records cover states accepted by the API contract, including
	// intermediate states the current runtime does not produce.
	s.mu.Lock()
	for _, state := range []string{"requested", "reserved", "starting", "pending", "running", "succeeded", "failed", "canceled", "unknown"} {
		req := testRequest()
		s.state.Runs[req.ID] = &record{Request: req, Workload: codersdk.Workload{ID: req.ID, State: state}}
	}
	s.mu.Unlock()
	w := requestHTTP(t, s, http.MethodGet, "/stats", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.JSONEq(t, `{"total":9,"pending":4,"running":1,"succeeded":1,"failed":1,"canceled":1,"unknown":1}`, w.Body.String())
	require.NotContains(t, w.Body.String(), "sensitive-command")
	require.Equal(t, codersdk.WorkloadHostStats{Total: 9, Pending: 4, Running: 1, Succeeded: 1, Failed: 1, Canceled: 1, Unknown: 1}, getStats(t, s))
}

func TestHistoryPagination(t *testing.T) {
	t.Parallel()
	s := testSupervisor(t, testConfig(t), newFake())
	// Timestamp ties and reverse insertion exercise both ordering keys without
	// relying on map iteration or the system clock's resolution.
	ids := make([]string, 12)
	for i := range ids {
		ids[i] = uuid.NewString()
	}
	slices.Sort(ids)
	base := time.Now().UTC().Add(-time.Hour)
	entries := make([]codersdk.WorkloadHistoryEntry, len(ids))
	s.mu.Lock()
	for i := len(ids) - 1; i >= 0; i-- {
		req := testRequest()
		req.ID = uuid.MustParse(ids[i])
		req.Command = fmt.Sprintf("printf 'command %d\\n'", i)
		workload := codersdk.Workload{ID: req.ID, CreatedAt: base.Add(time.Duration(i/2) * time.Second), State: "succeeded"}
		s.state.Runs[req.ID] = &record{Request: req, Workload: workload}
		entries[i] = codersdk.WorkloadHistoryEntry{Request: req, Workload: workload}
	}
	s.mu.Unlock()

	page := getHistory(t, s, "")
	require.Equal(t, codersdk.WorkloadHostHistory{Items: entries[:10], Total: 12, NextOffset: 10}, page)
	page = getHistory(t, s, "?offset=3&limit=2")
	require.Equal(t, codersdk.WorkloadHostHistory{Items: entries[3:5], Total: 12, NextOffset: 5}, page)

	// A new admission must append after pages already observed, even if its
	// UUID sorts earlier. Live state changes must not reorder history either.
	rt := s.runtime.(*fakeRuntime)
	req := testRequest()
	req.ID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
	receive(t, rt.waiting)
	require.Equal(t, codersdk.WorkloadHostHistory{Items: entries[:10], Total: 13, NextOffset: 10}, getHistory(t, s, ""))
	page = getHistory(t, s, "?offset=10&limit=10")
	require.Equal(t, int64(13), page.Total)
	require.Equal(t, 13, page.NextOffset)
	require.Len(t, page.Items, 3)
	require.Equal(t, entries[10:], page.Items[:2])
	require.Equal(t, req, page.Items[2].Request)
	require.Equal(t, "running", page.Items[2].Workload.State)
	rt.finish <- 0
	joinRuns(t, s)
	page = getHistory(t, s, "?offset=12&limit=1")
	require.Len(t, page.Items, 1)
	require.Equal(t, req, page.Items[0].Request)
	require.Equal(t, "succeeded", page.Items[0].Workload.State)
	for _, offset := range []int{13, 14, math.MaxInt} {
		page = getHistory(t, s, fmt.Sprintf("?offset=%d", offset))
		require.Empty(t, page.Items)
		require.Equal(t, int64(13), page.Total)
		require.Equal(t, 13, page.NextOffset)
	}
}

func TestHistoryInvalidPagination(t *testing.T) {
	t.Parallel()
	s := testSupervisor(t, testConfig(t), newFake())
	for _, query := range []string{
		"offset=-1", "offset=no", "offset=1.5", "offset=", "offset=9223372036854775808", "offset=0&offset=1",
		"limit=-1", "limit=0", "limit=11", "limit=no", "limit=1.5", "limit=", "limit=9223372036854775808", "limit=1&limit=2",
	} {
		w := requestHTTP(t, s, http.MethodGet, "/history?"+query, nil)
		require.Equal(t, http.StatusBadRequest, w.Code, query)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
}

func TestHistoryResponseBound(t *testing.T) {
	t.Parallel()
	s := testSupervisor(t, testConfig(t), newFake())
	s.mu.Lock()
	for range 10 {
		req := testRequest()
		// Each byte needs six JSON bytes when HTML escaping is enabled.
		req.Command = strings.Repeat("<", 64<<10)
		s.state.Runs[req.ID] = &record{Request: req, Workload: codersdk.Workload{ID: req.ID, State: "succeeded"}}
	}
	s.mu.Unlock()
	w := requestHTTP(t, s, http.MethodGet, "/history", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Less(t, w.Body.Len(), 4<<20)
	var history codersdk.WorkloadHostHistory
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &history))
	require.Len(t, history.Items, 10)
	for _, item := range history.Items {
		require.Equal(t, strings.Repeat("<", 64<<10), item.Request.Command)
	}
}

func TestHistoryLargeMetadata(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	cfg.Repositories = make(map[string]string)
	for i := range 128 {
		cfg.Repositories[fmt.Sprintf("%03d%s", i, strings.Repeat("<", 1021))] = strings.Repeat("a", 40)
	}
	s := testSupervisor(t, cfg, newFake())
	s.mu.Lock()
	for range 10 {
		req := testRequest()
		req.Command = strings.Repeat("<", 64<<10)
		req.Env = map[string]string{"KEY": strings.Repeat("<", (64<<10)-3)}
		s.state.Runs[req.ID] = &record{Request: req, Workload: codersdk.Workload{ID: req.ID, Generation: s.generation, State: "succeeded"}}
	}
	s.mu.Unlock()
	seen := make(map[uuid.UUID]bool)
	offset := 0
	for offset < 10 {
		w := requestHTTP(t, s, http.MethodGet, fmt.Sprintf("/history?offset=%d", offset), nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.LessOrEqual(t, w.Body.Len(), 4<<20)
		var history codersdk.WorkloadHostHistory
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &history))
		require.NotEmpty(t, history.Items)
		require.Equal(t, int64(10), history.Total)
		require.Equal(t, offset+len(history.Items), history.NextOffset)
		for _, item := range history.Items {
			require.False(t, seen[item.Request.ID])
			seen[item.Request.ID] = true
			require.Equal(t, cfg.Repositories, item.Workload.Generation.Repositories)
			require.Equal(t, strings.Repeat("<", 64<<10), item.Request.Command)
			require.Equal(t, map[string]string{"KEY": strings.Repeat("<", (64<<10)-3)}, item.Request.Env)
		}
		offset = history.NextOffset
	}
	require.Len(t, seen, 10)
}

func TestHistoryOversizedEntry(t *testing.T) {
	t.Parallel()
	s := testSupervisor(t, testConfig(t), newFake())
	req := testRequest()
	s.mu.Lock()
	s.state.Runs[req.ID] = &record{Request: req, Workload: codersdk.Workload{
		ID: req.ID, State: "succeeded", Generation: codersdk.WorkloadGeneration{Workdir: "/" + strings.Repeat("<", 1<<20)},
	}}
	s.mu.Unlock()
	w := requestHTTP(t, s, http.MethodGet, "/history", nil)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.NotContains(t, w.Body.String(), req.Command)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, codersdk.WorkloadHostStats{Total: 1, Succeeded: 1}, getStats(t, s))
}

func TestHistoryAndStatsAvailability(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"draining", "failed", "closed"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			s := testSupervisor(t, testConfig(t), newFake())
			switch state {
			case "draining":
				require.Equal(t, http.StatusNoContent, requestHTTP(t, s, http.MethodPost, "/drain", nil).Code)
			case "failed":
				s.mu.Lock()
				s.failed = true
				s.mu.Unlock()
			case "closed":
				require.NoError(t, s.Close())
			}
			for _, path := range []string{"/stats", "/history"} {
				w := requestHTTP(t, s, http.MethodGet, path, nil)
				require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
				if state == "draining" {
					require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				} else {
					require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
					require.NotContains(t, w.Body.String(), `"total"`)
					require.NotContains(t, w.Body.String(), `"items"`)
				}
			}
		})
	}
}
