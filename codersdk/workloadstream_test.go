package codersdk_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestStreamWorkloadOutput(t *testing.T) {
	t.Parallel()
	for _, debug := range []bool{false, true} {
		t.Run(fmt.Sprintf("Debug=%t", debug), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			defer cancel()
			org, id := uuid.New(), uuid.New()
			const offset = int64(42)
			const secret = "private-stream-output"
			first := codersdk.WorkloadOutput{
				Workload:   codersdk.Workload{ID: id, State: "running"},
				Events:     []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte(secret)}},
				NextOffset: offset + 1,
			}
			last := codersdk.WorkloadOutput{
				Workload:   codersdk.Workload{ID: id, State: "succeeded"},
				Events:     []codersdk.WorkloadEvent{{Stream: "stderr", Data: []byte("last chunk")}},
				NextOffset: offset + 2,
			}
			terminal := codersdk.WorkloadOutput{Workload: last.Workload, NextOffset: last.NextOffset}
			sendFirst, sendLast := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, fmt.Sprintf("/api/v2/organizations/%s/workloads/%s", org, id), r.URL.Path)
				assert.Equal(t, url.Values{"offset": {"42"}, "follow": {"true"}}, r.URL.Query())
				assert.Equal(t, "session-token", r.Header.Get(codersdk.SessionTokenHeader))
				w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				if !assert.NoError(t, http.NewResponseController(w).Flush()) {
					return
				}
				select {
				case <-sendFirst:
				case <-r.Context().Done():
					return
				}
				if !assert.NoError(t, json.NewEncoder(w).Encode(first)) || !assert.NoError(t, http.NewResponseController(w).Flush()) {
					return
				}
				select {
				case <-sendLast:
				case <-r.Context().Done():
					return
				}
				_, _ = io.WriteString(w, "\n")
				assert.NoError(t, json.NewEncoder(w).Encode(last))
				assert.NoError(t, json.NewEncoder(w).Encode(terminal))
			}))
			t.Cleanup(server.Close)
			u, err := url.Parse(server.URL)
			require.NoError(t, err)
			var plainLogs, structuredLogs bytes.Buffer
			options := []codersdk.ClientOption{codersdk.WithSessionToken("session-token")}
			if debug {
				options = append(options, codersdk.WithLogBodies(), codersdk.WithPlainLogger(&plainLogs),
					codersdk.WithLogger(slog.Make(sloghuman.Sink(&structuredLogs)).Leveled(slog.LevelDebug)))
			}
			client := codersdk.New(u, options...)
			// Only the headers are available until this call returns, even when
			// debug HTTP dumping is enabled.
			body, err := client.StreamWorkloadOutput(ctx, org, id, offset)
			require.NoError(t, err)
			defer body.Close()
			close(sendFirst)
			reader := bufio.NewReader(body)
			line, err := reader.ReadBytes('\n')
			require.NoError(t, err)
			var got codersdk.WorkloadOutput
			require.NoError(t, json.Unmarshal(line, &got))
			require.Equal(t, first, got)
			// Later output and exit are withheld until the first bytes reach
			// the caller, proving that the response is not buffered.
			close(sendLast)
			heartbeat, err := reader.ReadString('\n')
			require.NoError(t, err)
			require.Equal(t, "\n", heartbeat)
			decoder := json.NewDecoder(reader)
			require.NoError(t, decoder.Decode(&got))
			require.Equal(t, last, got)
			require.NoError(t, decoder.Decode(&got))
			require.Equal(t, terminal, got)
			require.ErrorIs(t, decoder.Decode(&got), io.EOF)
			require.Equal(t, debug, client.LogBodies())
			for _, logs := range []string{plainLogs.String(), structuredLogs.String()} {
				require.NotContains(t, logs, secret)
				require.NotContains(t, logs, base64.StdEncoding.EncodeToString([]byte(secret)))
				require.NotContains(t, logs, "events")
				require.NotContains(t, logs, "body=")
			}
			if debug {
				require.Contains(t, plainLogs.String(), "http <--")
				require.Contains(t, structuredLogs.String(), "sdk response")
			}
		})
	}
}

func TestStreamWorkloadOutputCancellation(t *testing.T) {
	t.Parallel()
	for _, closeBody := range []bool{false, true} {
		t.Run(fmt.Sprintf("Close=%t", closeBody), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			defer cancel()
			streamCtx, cancelStream := context.WithCancel(ctx)
			defer cancelStream()
			disconnected := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/x-ndjson")
				assert.NoError(t, http.NewResponseController(w).Flush())
				<-r.Context().Done()
				close(disconnected)
			}))
			t.Cleanup(server.Close)
			u, err := url.Parse(server.URL)
			require.NoError(t, err)
			var requestCtx context.Context
			client := codersdk.New(u, codersdk.WithHTTPClient(&http.Client{
				Transport: workloadStreamRoundTripper(func(req *http.Request) (*http.Response, error) {
					requestCtx = req.Context()
					return http.DefaultTransport.RoundTrip(req)
				}),
			}))
			body, err := client.StreamWorkloadOutput(streamCtx, uuid.New(), uuid.New(), 0)
			require.NoError(t, err)
			defer body.Close()
			readDone := make(chan error, 1)
			go func() {
				_, err := body.Read(make([]byte, 1))
				readDone <- err
			}()
			if closeBody {
				require.NoError(t, body.Close())
				require.NoError(t, streamCtx.Err(), "closing the body must not cancel the caller")
			} else {
				cancelStream()
			}
			require.ErrorIs(t, requestCtx.Err(), context.Canceled)
			select {
			case err := <-readDone:
				require.Error(t, err)
			case <-ctx.Done():
				t.Fatal("body read did not unblock")
			}
			select {
			case <-disconnected:
			case <-ctx.Done():
				t.Fatal("stream did not disconnect")
			}
			require.NoError(t, body.Close(), "close is idempotent")
		})
	}
}

func TestStreamWorkloadOutputErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		status     int
		retryAfter string
		wantRetry  time.Duration
	}{
		{"Forbidden", http.StatusForbidden, "", 0},
		{"Unavailable", http.StatusServiceUnavailable, "5", 0},
		{"RateLimitedSeconds", http.StatusTooManyRequests, "5", 5 * time.Second},
		{"RateLimitedDate", http.StatusTooManyRequests, "Wed, 21 Oct 2015 07:28:30 GMT", 30 * time.Second},
		{"RateLimitedMissing", http.StatusTooManyRequests, "", time.Minute},
		{"RateLimitedInvalid", http.StatusTooManyRequests, "invalid", time.Minute},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Date", "Wed, 21 Oct 2015 07:28:00 GMT")
				w.Header().Set("Retry-After", tt.retryAfter)
				w.WriteHeader(tt.status)
				_ = json.NewEncoder(w).Encode(codersdk.Response{Message: "private-error-message"})
			}))
			t.Cleanup(server.Close)
			u, err := url.Parse(server.URL)
			require.NoError(t, err)
			var plainLogs, structuredLogs bytes.Buffer
			client := codersdk.New(u, codersdk.WithLogBodies(), codersdk.WithPlainLogger(&plainLogs),
				codersdk.WithLogger(slog.Make(sloghuman.Sink(&structuredLogs)).Leveled(slog.LevelDebug)))
			body, err := client.StreamWorkloadOutput(t.Context(), uuid.New(), uuid.New(), 0)
			require.Nil(t, body)
			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, tt.status, apiErr.StatusCode())
			require.Equal(t, "private-error-message", apiErr.Message)
			require.EqualValues(t, 1, calls.Load(), "the SDK must not retry")
			var retryErr interface{ RetryAfter() time.Duration }
			if tt.wantRetry > 0 {
				require.ErrorAs(t, err, &retryErr)
				require.Equal(t, tt.wantRetry, retryErr.RetryAfter())
			} else {
				require.False(t, errors.As(err, &retryErr))
			}
			require.NotContains(t, plainLogs.String(), "private-error-message")
			require.NotContains(t, structuredLogs.String(), "private-error-message")
		})
	}
}

func TestStreamWorkloadOutputWrongContentType(t *testing.T) {
	t.Parallel()
	for _, contentType := range []string{"", "application/json", "text/html", "application/x-ndjson; broken"} {
		t.Run(contentType, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			defer cancel()
			disconnected := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header()["Content-Type"] = []string{contentType}
				w.WriteHeader(http.StatusOK)
				assert.NoError(t, http.NewResponseController(w).Flush())
				<-r.Context().Done()
				close(disconnected)
			}))
			t.Cleanup(server.Close)
			u, err := url.Parse(server.URL)
			require.NoError(t, err)
			var logs bytes.Buffer
			client := codersdk.New(u, codersdk.WithLogBodies(), codersdk.WithPlainLogger(&logs))
			body, err := client.StreamWorkloadOutput(ctx, uuid.New(), uuid.New(), 0)
			require.Nil(t, body)
			require.ErrorContains(t, err, "unexpected workload output content type")
			select {
			case <-disconnected:
			case <-ctx.Done():
				t.Fatal("rejected body was not closed")
			}
		})
	}
}

func TestStreamWorkloadOutputEstablishmentTimeout(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u, err := url.Parse("http://workload.test")
		require.NoError(t, err)
		var calls atomic.Int32
		client := codersdk.New(u, codersdk.WithHTTPClient(&http.Client{
			Transport: workloadStreamRoundTripper(func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				<-req.Context().Done()
				return nil, req.Context().Err()
			}),
		}))
		started := time.Now()
		body, err := client.StreamWorkloadOutput(t.Context(), uuid.New(), uuid.New(), 0)
		require.Nil(t, body)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Equal(t, 10*time.Second, time.Since(started))
		require.EqualValues(t, 1, calls.Load())
	})
}

func TestStreamWorkloadOutputBodyOutlivesEstablishmentTimeout(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u, err := url.Parse("http://workload.test")
		require.NoError(t, err)
		reader, writer := io.Pipe()
		defer writer.Close()
		var requestCtx context.Context
		httpClient := &http.Client{
			Timeout: time.Second,
			Transport: workloadStreamRoundTripper(func(req *http.Request) (*http.Response, error) {
				requestCtx = req.Context()
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {"application/x-ndjson"}},
					Body:       reader,
					Request:    req,
				}, nil
			}),
		}
		client := codersdk.New(u, codersdk.WithHTTPClient(httpClient))
		body, err := client.StreamWorkloadOutput(t.Context(), uuid.New(), uuid.New(), 0)
		require.NoError(t, err)
		defer body.Close()
		// synctest advances virtual time without waiting on wall time.
		<-time.After(11 * time.Second)
		require.NoError(t, requestCtx.Err())
		require.Equal(t, time.Second, httpClient.Timeout, "the shared HTTP client must not be mutated")
		go func() { _, _ = io.WriteString(writer, "\n") }()
		buf := make([]byte, 1)
		_, err = io.ReadFull(body, buf)
		require.NoError(t, err)
		require.Equal(t, []byte("\n"), buf)
		require.NoError(t, body.Close())
		require.ErrorIs(t, requestCtx.Err(), context.Canceled)
		_, err = writer.Write([]byte("closed"))
		require.ErrorIs(t, err, io.ErrClosedPipe)
	})
}

type workloadStreamRoundTripper func(*http.Request) (*http.Response, error)

func (f workloadStreamRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
