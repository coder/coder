package agentsdk_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
)

func TestQueueStartupLogs(t *testing.T) {
	t.Parallel()
	t.Run("Spam", func(t *testing.T) {
		t.Parallel()
		lastLog := 0
		totalLogs := 1000
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			req := agentsdk.PatchStartupLogs{}
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			for _, log := range req.Logs {
				require.Equal(t, strconv.Itoa(lastLog), log.Output)
				lastLog++
			}
		}))
		srvURL, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := agentsdk.New(srvURL)
		sendLog, closer := client.QueueStartupLogs(context.Background(), 0)
		for i := 0; i < totalLogs; i++ {
			sendLog(agentsdk.StartupLog{
				CreatedAt: time.Now(),
				Output:    strconv.Itoa(i),
				Level:     codersdk.LogLevelInfo,
			})
		}
		err = closer.Close()
		require.NoError(t, err)
		require.Equal(t, totalLogs, lastLog)
	})
	t.Run("Debounce", func(t *testing.T) {
		t.Parallel()
		got := make(chan agentsdk.StartupLog)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			req := agentsdk.PatchStartupLogs{}
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			for _, log := range req.Logs {
				got <- log
			}
		}))
		srvURL, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := agentsdk.New(srvURL)
		sendLog, closer := client.QueueStartupLogs(context.Background(), time.Millisecond)
		sendLog(agentsdk.StartupLog{
			Output: "hello",
		})
		gotLog := <-got
		require.Equal(t, "hello", gotLog.Output)
		err = closer.Close()
		require.NoError(t, err)
	})
	t.Run("RetryOnError", func(t *testing.T) {
		t.Parallel()
		got := make(chan agentsdk.StartupLog)
		first := true
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if first {
				w.WriteHeader(http.StatusInternalServerError)
				first = false
				return
			}
			req := agentsdk.PatchStartupLogs{}
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			for _, log := range req.Logs {
				got <- log
			}
		}))
		srvURL, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := agentsdk.New(srvURL)
		sendLog, closer := client.QueueStartupLogs(context.Background(), time.Millisecond)
		sendLog(agentsdk.StartupLog{
			Output: "hello",
		})
		gotLog := <-got
		require.Equal(t, "hello", gotLog.Output)
		err = closer.Close()
		require.NoError(t, err)
	})
}
