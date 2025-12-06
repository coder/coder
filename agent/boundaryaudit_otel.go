package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

const (
	// OTELDefaultBatchSize is the maximum number of events to batch before sending.
	OTELDefaultBatchSize = 100
	// OTELDefaultFlushInterval is the maximum time to wait before sending a batch.
	OTELDefaultFlushInterval = 5 * time.Second
	// OTELDefaultTimeout is the HTTP request timeout.
	OTELDefaultTimeout = 30 * time.Second
)

// OTELExporter exports boundary audit logs to an OTEL collector via OTLP/HTTP.
type OTELExporter struct {
	logger        slog.Logger
	endpoint      string
	headers       map[string]string
	httpClient    *http.Client
	batchSize     int
	flushInterval time.Duration

	mu      sync.Mutex
	batch   []BoundaryAuditEvent
	timer   *time.Timer
	ctx     context.Context
	cancel  context.CancelFunc
	closed  bool
	wg      sync.WaitGroup
}

// OTELExporterConfig holds configuration for OTELExporter.
type OTELExporterConfig struct {
	Logger        slog.Logger
	Endpoint      string            // OTLP/HTTP endpoint (e.g., "https://otel.example.com:4318/v1/logs")
	Headers       map[string]string // Optional headers for authentication
	BatchSize     int
	FlushInterval time.Duration
	Timeout       time.Duration
	// InsecureSkipVerify skips TLS verification (for testing only).
	InsecureSkipVerify bool
}

// NewOTELExporter creates a new OTEL exporter.
func NewOTELExporter(config OTELExporterConfig) (*OTELExporter, error) {
	if config.Endpoint == "" {
		return nil, xerrors.New("OTEL endpoint is required")
	}

	batchSize := config.BatchSize
	if batchSize <= 0 {
		batchSize = OTELDefaultBatchSize
	}

	flushInterval := config.FlushInterval
	if flushInterval <= 0 {
		flushInterval = OTELDefaultFlushInterval
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = OTELDefaultTimeout
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: config.InsecureSkipVerify,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &OTELExporter{
		logger:        config.Logger.Named("otel-exporter"),
		endpoint:      config.Endpoint,
		headers:       config.Headers,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		batch:         make([]BoundaryAuditEvent, 0, batchSize),
		ctx:           ctx,
		cancel:        cancel,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}, nil
}

// Export adds events to the batch and sends when full or on interval.
func (e *OTELExporter) Export(events []BoundaryAuditEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return
	}

	e.batch = append(e.batch, events...)

	// Start timer on first event in batch.
	if len(e.batch) == len(events) && e.timer == nil {
		e.timer = time.AfterFunc(e.flushInterval, func() {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.flushLocked()
		})
	}

	// Flush if batch is full.
	if len(e.batch) >= e.batchSize {
		e.flushLocked()
	}
}

// flushLocked sends the current batch. Must be called with mu held.
func (e *OTELExporter) flushLocked() {
	if len(e.batch) == 0 {
		return
	}

	if e.timer != nil {
		e.timer.Stop()
		e.timer = nil
	}

	// Copy and clear batch.
	events := make([]BoundaryAuditEvent, len(e.batch))
	copy(events, e.batch)
	e.batch = e.batch[:0]

	// Send in background.
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.sendEvents(events)
	}()
}

// sendEvents sends events to the OTEL endpoint.
func (e *OTELExporter) sendEvents(events []BoundaryAuditEvent) {
	payload := e.buildOTLPPayload(events)

	body, err := json.Marshal(payload)
	if err != nil {
		e.logger.Warn(e.ctx, "failed to marshal OTLP payload", slog.Error(err))
		return
	}

	req, err := http.NewRequestWithContext(e.ctx, http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		e.logger.Warn(e.ctx, "failed to create OTLP request", slog.Error(err))
		return
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range e.headers {
		req.Header.Set(k, v)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		e.logger.Warn(e.ctx, "failed to send OTLP request", slog.Error(err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		e.logger.Warn(e.ctx, "OTLP request failed",
			slog.F("status", resp.StatusCode),
			slog.F("body", string(respBody)),
		)
		return
	}

	e.logger.Debug(e.ctx, "sent boundary audit logs to OTEL", slog.F("count", len(events)))
}

// buildOTLPPayload converts boundary audit events to OTLP log format.
// See: https://opentelemetry.io/docs/specs/otlp/#otlphttp
func (e *OTELExporter) buildOTLPPayload(events []BoundaryAuditEvent) map[string]any {
	logRecords := make([]map[string]any, len(events))

	for i, event := range events {
		// Convert timestamp to nanoseconds since epoch.
		timeUnixNano := fmt.Sprintf("%d", event.Timestamp.UnixNano())

		// Build attributes.
		attributes := []map[string]any{
			{"key": "boundary.resource_type", "value": map[string]any{"stringValue": event.ResourceType}},
			{"key": "boundary.resource", "value": map[string]any{"stringValue": event.Resource}},
			{"key": "boundary.operation", "value": map[string]any{"stringValue": event.Operation}},
			{"key": "boundary.decision", "value": map[string]any{"stringValue": event.Decision}},
		}

		// Determine severity based on decision.
		severityNumber := 9  // INFO
		severityText := "INFO"
		if strings.EqualFold(event.Decision, "deny") {
			severityNumber = 13 // WARN
			severityText = "WARN"
		}

		logRecords[i] = map[string]any{
			"timeUnixNano":   timeUnixNano,
			"severityNumber": severityNumber,
			"severityText":   severityText,
			"body": map[string]any{
				"stringValue": fmt.Sprintf("%s %s %s", event.Decision, event.Operation, event.Resource),
			},
			"attributes": attributes,
		}
	}

	return map[string]any{
		"resourceLogs": []map[string]any{
			{
				"resource": map[string]any{
					"attributes": []map[string]any{
						{"key": "service.name", "value": map[string]any{"stringValue": "coder-boundary"}},
					},
				},
				"scopeLogs": []map[string]any{
					{
						"scope": map[string]any{
							"name":    "boundary.audit",
							"version": "1.0.0",
						},
						"logRecords": logRecords,
					},
				},
			},
		},
	}
}

// Close flushes remaining events and stops the exporter.
func (e *OTELExporter) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	e.cancel()

	// Flush remaining events synchronously.
	if len(e.batch) > 0 {
		events := make([]BoundaryAuditEvent, len(e.batch))
		copy(events, e.batch)
		e.batch = nil
		e.mu.Unlock()

		e.sendEvents(events)
	} else {
		e.mu.Unlock()
	}

	e.wg.Wait()
	return nil
}
