package cli_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/testutil"
)

func TestExpStdioHTTP(t *testing.T) {
	t.Parallel()

	t.Run("NoCommand", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "exp", "stdio-http")
		err := inv.Run()
		require.ErrorContains(t, err, "command is required")
	})

	t.Run("BasicStdinStdout", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("Skipping cat test on Windows")
		}

		port := getFreePort(t)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		// Use cat command which stays running and echoes stdin to stdout
		inv, _ := clitest.New(t, "exp", "stdio-http", "--port", port, "cat")
		inv = inv.WithContext(ctx)

		cmdDone := make(chan struct{})
		var cmdErr error
		go func() {
			defer close(cmdDone)
			cmdErr = inv.Run()
		}()

		// Wait for server to start
		waitForHTTPServer(t, "localhost:"+port)

		// Start reading stdout stream
		respOut := getSSEStream(t, fmt.Sprintf("http://localhost:%s/stdout", port))
		defer respOut.Body.Close()

		// Send input via stdin endpoint
		inputText := "hello world"
		respIn := postToStdin(t, fmt.Sprintf("http://localhost:%s/stdin", port), inputText+"\n")
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

		cancel()
		<-cmdDone
		if cmdErr != nil && !isContextCanceled(cmdErr) {
			t.Errorf("Command failed: %v", cmdErr)
		}
	})

	t.Run("StderrOutput", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("Skipping stderr test on Windows (shell differences)")
		}

		port := getFreePort(t)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		// Use a command that writes to stderr and then stays running
		inv, _ := clitest.New(t, "exp", "stdio-http", "--port", port, "sh", "-c", "echo 'error message' >&2; cat")
		inv = inv.WithContext(ctx)

		cmdDone := make(chan struct{})
		var cmdErr error
		go func() {
			defer close(cmdDone)
			cmdErr = inv.Run()
		}()

		waitForHTTPServer(t, "localhost:"+port)

		// Test stderr endpoint
		resp := getSSEStream(t, fmt.Sprintf("http://localhost:%s/stderr", port))
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		var lines []string
		timeout := time.After(3 * time.Second)

		done := make(chan struct{})
		go func() {
			defer close(done)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data: ") {
					lines = append(lines, strings.TrimPrefix(line, "data: "))
					return
				}
			}
		}()

		select {
		case <-done:
		case <-timeout:
			t.Fatal("Timeout waiting for stderr response")
		}

		require.Len(t, lines, 1)
		require.Equal(t, "error message", lines[0])

		cancel()
		<-cmdDone
		if cmdErr != nil && !isContextCanceled(cmdErr) {
			t.Errorf("Command failed: %v", cmdErr)
		}
	})

	t.Run("CustomHostPort", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("Skipping cat test on Windows")
		}

		port := getFreePort(t)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		inv, _ := clitest.New(t, "exp", "stdio-http", "--host", "127.0.0.1", "--port", port, "cat")
		inv = inv.WithContext(ctx)

		cmdDone := make(chan struct{})
		var cmdErr error
		go func() {
			defer close(cmdDone)
			cmdErr = inv.Run()
		}()

		waitForHTTPServer(t, "127.0.0.1:"+port)

		// Verify server is accessible on the custom host:port
		resp := getSSEStream(t, fmt.Sprintf("http://127.0.0.1:%s/stdout", port))
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		cancel()
		<-cmdDone
		if cmdErr != nil && !isContextCanceled(cmdErr) {
			t.Errorf("Command failed: %v", cmdErr)
		}
	})

	t.Run("CommandTimeout", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("Skipping timeout test on Windows (sleep command differences)")
		}

		port := getFreePort(t)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		// Use a short timeout and a command that would run longer
		inv, _ := clitest.New(t, "exp", "stdio-http", "--port", port, "--timeout", "100ms", "sleep", "5")
		inv = inv.WithContext(ctx)

		cmdDone := make(chan struct{})
		go func() {
			defer close(cmdDone)
			_ = inv.Run() // Ignore error as timeout is expected
		}()

		// The command should timeout quickly before we even try to connect
		select {
		case <-cmdDone:
			// Command completed (likely due to timeout)
		case <-time.After(testutil.WaitShort):
			t.Fatal("Command should have timed out but didn't")
		}

		cancel()
	})

	t.Run("InvalidMethod", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("Skipping cat test on Windows")
		}

		port := getFreePort(t)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		inv, _ := clitest.New(t, "exp", "stdio-http", "--port", port, "cat")
		inv = inv.WithContext(ctx)

		cmdDone := make(chan struct{})
		var cmdErr error
		go func() {
			defer close(cmdDone)
			cmdErr = inv.Run()
		}()

		waitForHTTPServer(t, "localhost:"+port)

		// Test invalid methods on endpoints
		client := &http.Client{Timeout: time.Second}

		// POST to stdout (should be GET)
		req, _ := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://localhost:%s/stdout", port), nil)
		resp, err := client.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		resp.Body.Close()

		// GET to stdin (should be POST)
		req, _ = http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://localhost:%s/stdin", port), nil)
		resp, err = client.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		resp.Body.Close()

		cancel()
		<-cmdDone
		if cmdErr != nil && !isContextCanceled(cmdErr) {
			t.Errorf("Command failed: %v", cmdErr)
		}
	})
}

// Helper functions

func getFreePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return fmt.Sprintf("%d", port)
}

func waitForHTTPServer(t *testing.T, addr string) {
	t.Helper()
	client := &http.Client{Timeout: 100 * time.Millisecond}

	for i := 0; i < 50; i++ { // Try for up to 5 seconds
		resp, err := client.Get(fmt.Sprintf("http://%s/stdout", addr))
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("HTTP server at %s did not start within timeout", addr)
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

func isContextCanceled(err error) bool {
	return err != nil && strings.Contains(err.Error(), "context canceled")
}
