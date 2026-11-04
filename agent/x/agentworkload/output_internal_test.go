//go:build linux

package agentworkload

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestBoundedOutput(t *testing.T) {
	t.Parallel()
	for _, limit := range []string{"bytes", "events"} {
		t.Run(limit, func(t *testing.T) {
			t.Parallel()
			rt := newFake()
			if limit == "bytes" {
				rt.output = frame(1, bytes.Repeat([]byte("x"), maxOutputBytes+1))
			} else {
				for range maxOutputEvents + 1 {
					rt.output = append(rt.output, frame(2, []byte("x"))...)
				}
			}
			rt.finish <- 0
			s := testSupervisor(t, testConfig(t), rt)
			req := testRequest()
			require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
			joinRuns(t, s)
			var offset int64
			totalBytes, totalEvents := 0, 0
			for {
				out := getOutput(t, s, req.ID, strconv.FormatInt(offset, 10))
				require.True(t, out.Truncated)
				require.LessOrEqual(t, len(out.Events), maxBatchEvents)
				batchBytes := 0
				for _, event := range out.Events {
					batchBytes += len(event.Data)
				}
				require.LessOrEqual(t, batchBytes, maxBatchBytes)
				totalBytes += batchBytes
				totalEvents += len(out.Events)
				require.Equal(t, offset+int64(len(out.Events)), out.NextOffset)
				if out.NextOffset == offset {
					break
				}
				offset = out.NextOffset
			}
			if limit == "bytes" {
				require.Equal(t, maxOutputBytes, totalBytes)
			} else {
				require.Equal(t, maxOutputEvents, totalEvents)
			}
		})
	}
}

func TestInteractiveOutputRetention(t *testing.T) {
	t.Parallel()
	for _, limit := range []string{"bytes", "events"} {
		t.Run(limit, func(t *testing.T) {
			t.Parallel()
			rt := newFake()
			cfg := testConfig(t)
			s := testSupervisor(t, cfg, rt)
			req := testRequest()
			req.Stdin = true
			require.Equal(t, http.StatusCreated, requestHTTP(t, s, http.MethodPost, "/workloads", req).Code)
			receive(t, rt.waiting)
			s.mu.Lock()
			run := s.state.Runs[req.ID]
			s.mu.Unlock()
			size, capacity := 1, maxOutputEvents
			if limit == "bytes" {
				size, capacity = 16<<10, maxOutputBytes/(16<<10)
			}
			count := capacity + 7
			data := bytes.Repeat([]byte("x"), size)
			for i := range count {
				s.appendOutput(run, "stdout", data)
				out, _, err := s.output(req.ID, int64(i))
				require.NoError(t, err)
				require.False(t, out.Truncated, "a live reader must not lose output at the archive cap")
				require.Equal(t, int64(i+1), out.NextOffset)
				require.Equal(t, []codersdk.WorkloadEvent{{Stream: "stdout", Data: data}}, out.Events)
			}
			lagged, _, err := s.output(req.ID, 0)
			require.NoError(t, err)
			require.True(t, lagged.Truncated)
			require.Equal(t, int64(7+len(lagged.Events)), lagged.NextOffset)
			s.mu.Lock()
			require.Equal(t, int64(7), run.output.BaseOffset)
			require.Len(t, run.output.Events, capacity)
			require.LessOrEqual(t, run.outputBytes, maxOutputBytes)
			require.True(t, run.output.valid())
			s.mu.Unlock()
			rt.finish <- 0
			joinRuns(t, s)
			tail := getOutput(t, s, req.ID, strconv.Itoa(count-1))
			require.Len(t, tail.Events, 1)
			require.Equal(t, int64(count), tail.NextOffset)
			require.False(t, tail.Truncated)
			require.NoError(t, s.Close())
			recovered := testSupervisor(t, cfg, newFake())
			require.Equal(t, tail, getOutput(t, recovered, req.ID, strconv.Itoa(count-1)))
			require.True(t, getOutput(t, recovered, req.ID, "0").Truncated)
			require.Empty(t, getOutput(t, recovered, req.ID, strconv.Itoa(count)).Events)
			require.Equal(t, http.StatusBadRequest, requestHTTP(t, recovered, http.MethodGet, "/workloads/"+req.ID.String()+"?offset="+strconv.Itoa(count+1), nil).Code)
		})
	}
}

func TestRetainedOutputLegacyOffsets(t *testing.T) {
	t.Parallel()
	var old retainedOutput
	require.NoError(t, json.Unmarshal([]byte(`{"events":[{"stream":"stdout","data":"b2xk"}],"truncated":false}`), &old))
	require.True(t, old.valid())
	require.Zero(t, old.BaseOffset)
	require.Equal(t, []byte("old"), old.Events[0].Data)
	old.BaseOffset = -1
	require.False(t, old.valid())
	old.BaseOffset = math.MaxInt64
	require.False(t, old.valid())
}

func TestMalformedOutput(t *testing.T) {
	t.Parallel()
	for _, input := range [][]byte{{1, 0}, frame(3, []byte("private daemon error")), frame(1, []byte("short"))[:10]} {
		s := &Supervisor{}
		run := &record{output: retainedOutput{Events: []codersdk.WorkloadEvent{}}}
		require.Error(t, s.collect(run, bytes.NewReader(input)))
		require.True(t, run.output.Truncated)
	}
	// The declared frame length must not become an allocation size.
	var huge [8]byte
	huge[0] = 1
	binary.BigEndian.PutUint32(huge[4:], ^uint32(0))
	s := &Supervisor{}
	run := &record{}
	require.Error(t, s.collect(run, bytes.NewReader(huge[:])))
	require.Empty(t, run.output.Events)
}
