package circuitbreaker

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/sony/gobreaker/v2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/metrics"
)

// ErrCircuitOpen is returned by Execute when the circuit breaker is open
// and the request was rejected without calling the handler.
var ErrCircuitOpen = xerrors.New("circuit breaker is open")

// DefaultIsFailure returns true for standard HTTP status codes that typically
// indicate upstream overload.
func DefaultIsFailure(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, // 429
		http.StatusServiceUnavailable, // 503
		http.StatusGatewayTimeout:     // 504
		return true
	default:
		return false
	}
}

// ProviderCircuitBreakers manages per-endpoint/model circuit breakers for a single provider.
type ProviderCircuitBreakers struct {
	provider string
	config   config.CircuitBreaker
	breakers sync.Map // "endpoint:model" -> *gobreaker.CircuitBreaker[struct{}]
	onChange func(endpoint, model string, from, to gobreaker.State)
	metrics  *metrics.Metrics
}

// NewProviderCircuitBreakers creates circuit breakers for a single provider.
// Returns nil if cfg is nil (no circuit breaker protection).
// onChange is called when circuit state changes.
// metrics is used to record circuit breaker reject counts (can be nil).
func NewProviderCircuitBreakers(provider string, cfg *config.CircuitBreaker, onChange func(endpoint, model string, from, to gobreaker.State), m *metrics.Metrics) *ProviderCircuitBreakers {
	if cfg == nil {
		return nil
	}
	return &ProviderCircuitBreakers{
		provider: provider,
		config:   *cfg,
		onChange: onChange,
		metrics:  m,
	}
}

// isFailure checks if the status code should count as a failure.
// Falls back to DefaultIsFailure if no custom function is configured.
func (p *ProviderCircuitBreakers) isFailure(statusCode int) bool {
	if p.config.IsFailure != nil {
		return p.config.IsFailure(statusCode)
	}
	return DefaultIsFailure(statusCode)
}

// openErrBody returns the error response body when the circuit is open.
func (p *ProviderCircuitBreakers) openErrBody() []byte {
	if p.config.OpenErrorResponse != nil {
		return p.config.OpenErrorResponse()
	}
	return []byte(`{"error":"circuit breaker is open"}`)
}

// Get returns the circuit breaker for an endpoint/model tuple, creating it if needed.
func (p *ProviderCircuitBreakers) Get(endpoint, model string) *gobreaker.CircuitBreaker[struct{}] {
	key := endpoint + ":" + model
	if v, ok := p.breakers.Load(key); ok {
		return v.(*gobreaker.CircuitBreaker[struct{}]) //nolint:forcetypeassert // sync.Map always stores this type
	}

	settings := gobreaker.Settings{
		Name:        p.provider + ":" + key,
		MaxRequests: p.config.MaxRequests,
		Interval:    p.config.Interval,
		Timeout:     p.config.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= p.config.FailureThreshold
		},
		OnStateChange: func(_ string, from, to gobreaker.State) {
			if p.onChange != nil {
				p.onChange(endpoint, model, from, to)
			}
		},
	}

	cb := gobreaker.NewCircuitBreaker[struct{}](settings)
	actual, _ := p.breakers.LoadOrStore(key, cb)
	return actual.(*gobreaker.CircuitBreaker[struct{}]) //nolint:forcetypeassert // sync.Map always stores this type
}

// statusCapturingWriter wraps http.ResponseWriter to capture the status code.
// It implements http.Flusher to support streaming and http.Hijacker to
// satisfy the FullResponseWriter lint rule.
type statusCapturingWriter struct {
	http.ResponseWriter
	statusCode    int
	headerWritten bool
}

func (w *statusCapturingWriter) WriteHeader(code int) {
	if !w.headerWritten {
		w.statusCode = code
		w.headerWritten = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusCapturingWriter) Write(b []byte) (int, error) {
	if !w.headerWritten {
		w.statusCode = http.StatusOK
		w.headerWritten = true
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusCapturingWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusCapturingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, xerrors.New("upstream ResponseWriter does not support hijacking")
	}
	return h.Hijack()
}

// Unwrap returns the underlying ResponseWriter for interface checks.
func (w *statusCapturingWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Execute runs the given handler function within circuit breaker protection.
// If the circuit is open, the request is rejected with a 503 response, metrics are recorded,
// and ErrCircuitOpen is returned.
// Otherwise, it returns the handler's error (or nil on success).
// The handler receives a wrapped ResponseWriter that captures the status code.
// If the receiver is nil (no circuit breaker configured), the handler is called directly.
func (p *ProviderCircuitBreakers) Execute(endpoint, model string, w http.ResponseWriter, handler func(http.ResponseWriter) error) error {
	if p == nil {
		return handler(w)
	}

	cb := p.Get(endpoint, model)

	// Wrap response writer to capture status code
	sw := &statusCapturingWriter{ResponseWriter: w, statusCode: http.StatusOK}

	var handlerErr error
	_, err := cb.Execute(func() (struct{}, error) {
		handlerErr = handler(sw)
		if p.isFailure(sw.statusCode) {
			return struct{}{}, xerrors.Errorf("upstream error: %d", sw.statusCode)
		}
		return struct{}{}, nil
	})

	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		if p.metrics != nil {
			p.metrics.CircuitBreakerRejects.WithLabelValues(p.provider, endpoint, model).Inc()
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int64(p.config.Timeout.Seconds())))
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write(p.openErrBody())
		return ErrCircuitOpen
	}

	return handlerErr
}

// Timeout returns the configured timeout duration for this circuit breaker.
func (p *ProviderCircuitBreakers) Timeout() time.Duration {
	return p.config.Timeout
}

// Provider returns the provider name for this circuit breaker.
func (p *ProviderCircuitBreakers) Provider() string {
	return p.provider
}

// OpenErrorResponse returns the error response body when the circuit is open.
// This is exposed for handlers to use when responding to rejected requests.
func (p *ProviderCircuitBreakers) OpenErrorResponse() []byte {
	return p.openErrBody()
}

// StateToGaugeValue converts gobreaker.State to a gauge value.
// closed=0, half-open=0.5, open=1
func StateToGaugeValue(s gobreaker.State) float64 {
	switch s {
	case gobreaker.StateClosed:
		return 0
	case gobreaker.StateHalfOpen:
		return 0.5
	case gobreaker.StateOpen:
		return 1
	default:
		return 0
	}
}
