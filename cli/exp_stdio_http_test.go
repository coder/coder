package cli_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/testutil"
)

func TestExpStdioHTTP(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("Skipping cat test on Windows")
		}

		port := testutil.RandomPort(t)
		ctx := testutil.Context(t, testutil.WaitMedium)

		// Use cat command which stays running and echoes stdin to stdout
		inv, _ := clitest.New(t, "exp", "stdio-http", "--port", strconv.Itoa(port), "tee")
		inv = inv.WithContext(ctx)

		cmdDone := make(chan struct{})
		var cmdErr error
		go func() {
			defer close(cmdDone)
			cmdErr = inv.Run()
		}()

		// Wait for server to start
		waitForHTTPServer(ctx, t, port)

		// Start reading stdout stream
		respOut := getSSEStream(t, fmt.Sprintf("http://localhost:%d", port))
		defer respOut.Body.Close()

		// Send input via stdin endpoint
		inputText := "hello world"
		respIn := postToStdin(t, fmt.Sprintf("http://localhost:%d", port), inputText+"\n")
		defer respIn.Body.Close()

		require.Equal(t, http.StatusOK, respIn.StatusCode)

		// Verify stdin response
		var stdinResp map[string]interface{}
		err := json.NewDecoder(respIn.Body).Decode(&stdinResp)
		require.NoError(t, err)
		require.Equal(t, "ok", stdinResp["status"])
		require.Equal(t, float64(len(inputText)+1), stdinResp["bytes_written"]) // +1 for newline

		// Read stdout stream
		scanner := bufio.NewScanner(respOut.Body)
		var outputLines []string
		timeout := time.After(3 * time.Second)

		done := make(chan struct{})
		go func() {
			defer close(done)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data: ") {
					outputLines = append(outputLines, strings.TrimPrefix(line, "data: "))
					return // Got our expected line
				}
			}
		}()

		select {
		case <-done:
			// Success
		case <-timeout:
			t.Fatal("Timeout waiting for stdout response")
		}

		require.Len(t, outputLines, 1)
		require.Equal(t, inputText, outputLines[0])

		<-cmdDone
		if cmdErr != nil && !strings.Contains(cmdErr.Error(), "context canceled") {
			t.Errorf("Command failed: %v", cmdErr)
		}
	})
}

func waitForHTTPServer(ctx context.Context, t *testing.T, port int) {
	t.Helper()
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		client := &http.Client{Timeout: 100 * time.Millisecond}
		req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://localhost:%d", port), nil)
		if !assert.NoError(t, err) {
			return false
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			return true
		}
		return false
	}, testutil.IntervalFast)
}

func getSSEStream(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	return resp
}

func postToStdin(t *testing.T, url, data string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("POST", url, strings.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "text/plain")

	client := &http.Client{Timeout: time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	return resp
}
