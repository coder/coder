package circuitbreaker_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/metrics"
)

func TestExecute_PerModelIsolation(t *testing.T) {
	t.Parallel()

	sonnetCalls := atomic.Int32{}
	haikuCalls := atomic.Int32{}

	cbs := circuitbreaker.NewProviderCircuitBreakers("test", &config.CircuitBreaker{
		FailureThreshold: 1,
		Interval:         time.Minute,
		Timeout:          time.Minute,
		MaxRequests:      1,
	}, slogtest.Make(t, nil), nil)

	endpoint := "/v1/messages"
	sonnetModel := "claude-sonnet-4-20250514"
	haikuModel := "claude-3-5-haiku-20241022"

	// Trip circuit on sonnet model (returns 503)
	w := httptest.NewRecorder()
	err := cbs.Execute(endpoint, sonnetModel, w, func(rw http.ResponseWriter) error {
		sonnetCalls.Add(1)
		rw.WriteHeader(http.StatusServiceUnavailable)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, int32(1), sonnetCalls.Load())

	// Second sonnet request should be blocked by circuit breaker
	w = httptest.NewRecorder()
	err = cbs.Execute(endpoint, sonnetModel, w, func(rw http.ResponseWriter) error {
		sonnetCalls.Add(1)
		rw.WriteHeader(http.StatusOK)
		return nil
	})
	assert.True(t, errors.Is(err, circuitbreaker.ErrCircuitOpen))
	assert.Equal(t, int32(1), sonnetCalls.Load()) // No new call
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	// Haiku model on same endpoint should still work (independent circuit)
	w = httptest.NewRecorder()
	err = cbs.Execute(endpoint, haikuModel, w, func(rw http.ResponseWriter) error {
		haikuCalls.Add(1)
		rw.WriteHeader(http.StatusOK)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, int32(1), haikuCalls.Load())
}

func TestExecute_PerEndpointIsolation(t *testing.T) {
	t.Parallel()

	messagesCalls := atomic.Int32{}
	completionsCalls := atomic.Int32{}

	cbs := circuitbreaker.NewProviderCircuitBreakers("test", &config.CircuitBreaker{
		FailureThreshold: 1,
		Interval:         time.Minute,
		Timeout:          time.Minute,
		MaxRequests:      1,
	}, slogtest.Make(t, nil), nil)

	model := "test-model"

	// Trip circuit on /v1/messages endpoint (returns 503)
	w := httptest.NewRecorder()
	err := cbs.Execute("/v1/messages", model, w, func(rw http.ResponseWriter) error {
		messagesCalls.Add(1)
		rw.WriteHeader(http.StatusServiceUnavailable)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, int32(1), messagesCalls.Load())

	// Second /v1/messages request should be blocked
	w = httptest.NewRecorder()
	err = cbs.Execute("/v1/messages", model, w, func(rw http.ResponseWriter) error {
		messagesCalls.Add(1)
		rw.WriteHeader(http.StatusOK)
		return nil
	})
	assert.True(t, errors.Is(err, circuitbreaker.ErrCircuitOpen))
	assert.Equal(t, int32(1), messagesCalls.Load()) // No new call
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	// /v1/chat/completions on same model should still work (different endpoint)
	w = httptest.NewRecorder()
	err = cbs.Execute("/v1/chat/completions", model, w, func(rw http.ResponseWriter) error {
		completionsCalls.Add(1)
		rw.WriteHeader(http.StatusOK)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, int32(1), completionsCalls.Load())
}

func TestExecute_CustomIsFailure(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	// Custom IsFailure that treats 502 as failure
	cbs := circuitbreaker.NewProviderCircuitBreakers("test", &config.CircuitBreaker{
		FailureThreshold: 1,
		Interval:         time.Minute,
		Timeout:          time.Minute,
		MaxRequests:      1,
		IsFailure: func(statusCode int) bool {
			return statusCode == http.StatusBadGateway
		},
	}, slogtest.Make(t, nil), nil)

	// First request returns 502, trips circuit
	w := httptest.NewRecorder()
	err := cbs.Execute("/v1/messages", "test-model", w, func(rw http.ResponseWriter) error {
		calls.Add(1)
		rw.WriteHeader(http.StatusBadGateway)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())

	// Second request should be blocked
	w = httptest.NewRecorder()
	err = cbs.Execute("/v1/messages", "test-model", w, func(rw http.ResponseWriter) error {
		calls.Add(1)
		rw.WriteHeader(http.StatusOK)
		return nil
	})
	assert.True(t, errors.Is(err, circuitbreaker.ErrCircuitOpen))
	assert.Equal(t, int32(1), calls.Load()) // No new call
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestExecute_OnStateChange(t *testing.T) {
	t.Parallel()

	m := metrics.NewMetrics(prometheus.NewRegistry())
	cbs := circuitbreaker.NewProviderCircuitBreakers("test", &config.CircuitBreaker{
		FailureThreshold: 1,
		Interval:         time.Minute,
		Timeout:          time.Minute,
		MaxRequests:      1,
	}, slogtest.Make(t, nil), m)

	endpoint := "/v1/messages"
	model := "claude-sonnet-4-20250514"

	// Trip circuit
	w := httptest.NewRecorder()
	err := cbs.Execute(endpoint, model, w, func(rw http.ResponseWriter) error {
		rw.WriteHeader(http.StatusServiceUnavailable)
		return nil
	})
	assert.NoError(t, err)

	// Verify the state change was recorded with the correct labels.
	assert.Equal(t, circuitbreaker.StateToGaugeValue(gobreaker.StateOpen), promtest.ToFloat64(m.CircuitBreakerState.WithLabelValues("test", endpoint, model)))
	assert.Equal(t, float64(1), promtest.ToFloat64(m.CircuitBreakerTrips.WithLabelValues("test", endpoint, model)))
}

func TestDefaultIsFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		statusCode int
		isFailure  bool
	}{
		{http.StatusOK, false},
		{http.StatusBadRequest, false},
		{http.StatusUnauthorized, false},
		{http.StatusTooManyRequests, false}, // 429: handled by key failover, not circuit breaker
		{http.StatusInternalServerError, false},
		{http.StatusBadGateway, false},
		{http.StatusServiceUnavailable, true}, // 503
		{http.StatusGatewayTimeout, true},     // 504
	}

	for _, tt := range tests {
		assert.Equal(t, tt.isFailure, circuitbreaker.DefaultIsFailure(tt.statusCode), "status code %d", tt.statusCode)
	}
}

func TestStateToGaugeValue(t *testing.T) {
	t.Parallel()

	assert.Equal(t, float64(0), circuitbreaker.StateToGaugeValue(gobreaker.StateClosed))
	assert.Equal(t, float64(0.5), circuitbreaker.StateToGaugeValue(gobreaker.StateHalfOpen))
	assert.Equal(t, float64(1), circuitbreaker.StateToGaugeValue(gobreaker.StateOpen))
}
