package mcpclient

import (
	"context"
	"net/netip"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/safedial"
)

// ConvertCallResultForTest exposes convertCallResult for external
// tests.
var ConvertCallResultForTest = convertCallResult

// ConnectAllForTest exposes connectAll with an injectable connect
// timeout and a reaperDone hook that fires after an abandoned
// connect goroutine has been drained and its late session closed.
func ConnectAllForTest(
	ctx context.Context,
	logger slog.Logger,
	servers []Server,
	timeout time.Duration,
	reaperDone func(),
) ([]fantasy.AgentTool, []ConnectSummary, func()) {
	// Connect-budget tests serve from loopback, which the guard
	// blocks by default, so allow it explicitly.
	httpClient := NewHTTPClient(nil, safedial.WithAllowedPrefixes(
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	))
	return connectAllWithHooks(
		ctx, logger, servers, nil, uuid.Nil, nil, nil,
		connectOptions{
			httpClient: httpClient,
			timeout:    timeout,
			hooks:      connectHooks{reaperDone: reaperDone},
			kind:       connectionKindOrg,
		},
	)
}

// ConnectInlineForTest exposes the inline connect path
// with an injectable connect timeout and a loopback-permitting client.
func ConnectInlineForTest(
	ctx context.Context,
	logger slog.Logger,
	servers []Server,
	coderHeaders map[string]string,
	timeout time.Duration,
) ([]fantasy.AgentTool, []ConnectSummary, func()) {
	httpClient := NewHTTPClient(nil, safedial.WithAllowedPrefixes(
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	))
	return connectAllWithHooks(
		ctx, logger, servers, nil, uuid.Nil, nil, coderHeaders,
		connectOptions{
			httpClient: inlineHTTPClient(httpClient, InternalServers{}),
			timeout:    timeout,
			kind:       connectionKindInline,
		},
	)
}

// ToolCallIDMetaKeyForTest exposes the _meta key for external tests.
const ToolCallIDMetaKeyForTest = toolCallIDMetaKey

// MaxInlineToolResultBytesForTest exposes the result cap.
const MaxInlineToolResultBytesForTest = maxInlineToolResultBytes

// MaxInlineHTTPResponseBytesForTest exposes the body cap.
const MaxInlineHTTPResponseBytesForTest = maxInlineHTTPResponseBytes

// BuildAuthHeadersForTest exposes buildAuthHeaders for external
// tests.
var BuildAuthHeadersForTest = buildAuthHeaders

// SummaryErrorForTest exposes summaryError for external tests.
var SummaryErrorForTest = summaryError

// MaxSummaryErrorLenForTest exposes the persisted-error byte cap for
// external tests.
const MaxSummaryErrorLenForTest = maxSummaryErrorLen
