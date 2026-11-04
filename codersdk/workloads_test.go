package codersdk_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/codersdk"
)

func TestParseWorkloadResources(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		cpu, ram     string
		cores, bytes int64
	}{
		{"1", "16MiB", 1, 16 << 20},
		{"2", "4GiB", 2, 4 << 30},
		{"8", "512MiB", 8, 512 << 20},
	} {
		t.Run(tt.cpu+"/"+tt.ram, func(t *testing.T) {
			t.Parallel()
			cpu, ram, err := codersdk.ParseWorkloadResources(tt.cpu, tt.ram)
			require.NoError(t, err)
			require.Equal(t, tt.cores, cpu)
			require.Equal(t, tt.bytes, ram)
		})
	}
	for _, cpu := range []string{"", "0", "-1", "+1", "01", "1.5", "500m", "1e2", " 1", "9999999999999999999999999", "9223372036854775807"} {
		t.Run("cpu/"+cpu, func(t *testing.T) {
			t.Parallel()
			_, _, err := codersdk.ParseWorkloadResources(cpu, "1GiB")
			require.ErrorContains(t, err, "--cpu")
		})
	}
	for _, ram := range []string{"", "0GiB", "-1GiB", "+1GiB", "01GiB", "4GB", "4G", "4", "1.5GiB", "15MiB", "1KiB", "4gib", " 4GiB", "4GiB ", "9999999999999999999999999GiB", "9223372036854775807MiB"} {
		t.Run("ram/"+ram, func(t *testing.T) {
			t.Parallel()
			_, _, err := codersdk.ParseWorkloadResources("1", ram)
			require.ErrorContains(t, err, "--ram")
		})
	}
}

func TestWorkloadRequestsDoNotLogPayloads(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req codersdk.CreateWorkloadRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "secret-command-contents", req.Command)
		require.Equal(t, map[string]string{"TOKEN": "secret-env-contents"}, req.Env)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(codersdk.Workload{ID: req.ID, Failure: "secret-output-contents"})
	}))
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	var plainLogs, structuredLogs bytes.Buffer
	client := codersdk.New(u, codersdk.WithLogBodies(), codersdk.WithPlainLogger(&plainLogs),
		codersdk.WithLogger(slog.Make(sloghuman.Sink(&structuredLogs)).Leveled(slog.LevelDebug)))
	_, err = client.CreateWorkload(t.Context(), uuid.New(), codersdk.CreateWorkloadRequest{
		ID: uuid.New(), Command: "secret-command-contents", Env: map[string]string{"TOKEN": "secret-env-contents"},
	})
	require.NoError(t, err)
	for _, logs := range []string{plainLogs.String(), structuredLogs.String()} {
		require.NotContains(t, logs, "secret-command-contents")
		require.NotContains(t, logs, "secret-env-contents")
		require.NotContains(t, logs, "secret-output-contents")
	}
	require.Contains(t, structuredLogs.String(), "sdk request")
	require.Contains(t, structuredLogs.String(), "sdk response")
	require.True(t, client.LogBodies(), "per-request suppression must not mutate the client")
}

func TestWorkloadOutputRetryAfter(t *testing.T) {
	t.Parallel()
	const serverDate = "Wed, 21 Oct 2015 07:28:00 GMT"
	for _, tt := range []struct {
		name       string
		retryAfter string
		status     int
		want       time.Duration
	}{
		{"Seconds", "5", http.StatusTooManyRequests, 5 * time.Second},
		{"Date", "Wed, 21 Oct 2015 07:28:30 GMT", http.StatusTooManyRequests, 30 * time.Second},
		{"Missing", "", http.StatusTooManyRequests, time.Minute},
		{"Invalid", "invalid", http.StatusTooManyRequests, time.Minute},
		{"Fractional", "0.5", http.StatusTooManyRequests, time.Minute},
		{"Zero", "0", http.StatusTooManyRequests, time.Minute},
		{"Negative", "-5", http.StatusTooManyRequests, time.Minute},
		{"Signed", "+5", http.StatusTooManyRequests, time.Minute},
		{"PastDate", "Wed, 21 Oct 2015 07:27:59 GMT", http.StatusTooManyRequests, time.Minute},
		{"PresentDate", serverDate, http.StatusTooManyRequests, time.Minute},
		{"MaxSeconds", "9223372036", http.StatusTooManyRequests, 9223372036 * time.Second},
		{"DurationOverflow", "9223372037", http.StatusTooManyRequests, time.Minute},
		{"IntegerOverflow", "18446744073709551616", http.StatusTooManyRequests, time.Minute},
		{"ServiceUnavailable", "5", http.StatusServiceUnavailable, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Date", serverDate)
				if tt.retryAfter != "" {
					w.Header().Set("Retry-After", tt.retryAfter)
				}
				w.WriteHeader(tt.status)
				_ = json.NewEncoder(w).Encode(codersdk.Response{Message: "request rejected"})
			}))
			t.Cleanup(server.Close)
			u, err := url.Parse(server.URL)
			require.NoError(t, err)
			client := codersdk.New(u)
			_, err = client.WorkloadOutput(t.Context(), uuid.New(), uuid.New(), 0)
			require.Error(t, err)
			require.EqualValues(t, 1, calls.Load())
			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, tt.status, apiErr.StatusCode())
			require.Equal(t, "request rejected", apiErr.Message)
			require.Equal(t, apiErr.Error(), err.Error())
			var retryErr interface{ RetryAfter() time.Duration }
			if tt.status != http.StatusTooManyRequests {
				require.False(t, errors.As(err, &retryErr))
				require.Same(t, apiErr, err)
				return
			}
			require.ErrorAs(t, err, &retryErr)
			require.Equal(t, tt.want, retryErr.RetryAfter())
			require.Same(t, apiErr, errors.Unwrap(err))
		})
	}
}

func TestWorkloadOutputRetryAfterLocalDate(t *testing.T) {
	t.Parallel()
	for _, date := range []string{"", "invalid"} {
		t.Run("Date="+date, func(t *testing.T) {
			t.Parallel()
			retryAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				// Suppress net/http's automatic Date header for the missing case.
				w.Header()["Date"] = nil
				if date != "" {
					w.Header().Set("Date", date)
				}
				w.Header().Set("Retry-After", retryAt.Format(http.TimeFormat))
				w.WriteHeader(http.StatusTooManyRequests)
			}))
			t.Cleanup(server.Close)
			u, err := url.Parse(server.URL)
			require.NoError(t, err)
			client := codersdk.New(u)
			before := time.Now()
			_, err = client.WorkloadOutput(t.Context(), uuid.New(), uuid.New(), 0)
			after := time.Now()
			require.EqualValues(t, 1, calls.Load())
			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode())
			var retryErr interface{ RetryAfter() time.Duration }
			require.ErrorAs(t, err, &retryErr)
			require.GreaterOrEqual(t, retryErr.RetryAfter(), retryAt.Sub(after))
			require.LessOrEqual(t, retryErr.RetryAfter(), retryAt.Sub(before))
		})
	}
}

func TestCreateWorkloadRequestValidate(t *testing.T) {
	t.Parallel()
	valid := codersdk.CreateWorkloadRequest{ID: uuid.New(), TemplateID: uuid.New(), CPU: 2, MemoryBytes: 4 << 30, Command: "pytest -q"}
	require.NoError(t, valid.Validate())
	for _, tt := range []struct {
		name   string
		mutate func(*codersdk.CreateWorkloadRequest)
	}{
		{"id", func(r *codersdk.CreateWorkloadRequest) { r.ID = uuid.Nil }},
		{"template", func(r *codersdk.CreateWorkloadRequest) { r.TemplateID = uuid.Nil }},
		{"cpu_zero", func(r *codersdk.CreateWorkloadRequest) { r.CPU = 0 }},
		{"cpu_negative", func(r *codersdk.CreateWorkloadRequest) { r.CPU = -1 }},
		{"cpu_overflow", func(r *codersdk.CreateWorkloadRequest) { r.CPU = math.MaxInt64 }},
		{"ram_zero", func(r *codersdk.CreateWorkloadRequest) { r.MemoryBytes = 0 }},
		{"ram_negative", func(r *codersdk.CreateWorkloadRequest) { r.MemoryBytes = -1 }},
		{"ram_unaligned", func(r *codersdk.CreateWorkloadRequest) { r.MemoryBytes++ }},
		{"cmd_empty", func(r *codersdk.CreateWorkloadRequest) { r.Command = " \t\n" }},
		{"cmd_nul", func(r *codersdk.CreateWorkloadRequest) { r.Command = "echo\x00oops" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := valid
			tt.mutate(&req)
			require.Error(t, req.Validate())
		})
	}
}
