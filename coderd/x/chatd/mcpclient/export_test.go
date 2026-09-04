package mcpclient

import (
	"context"
	"net/netip"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
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
	configs []database.MCPServerConfig,
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
		ctx, logger, configs, nil, uuid.Nil, nil, nil, httpClient,
		timeout, connectHooks{reaperDone: reaperDone},
	)
}

// BuildAuthHeadersForTest exposes buildAuthHeaders for external
// tests.
var BuildAuthHeadersForTest = buildAuthHeaders

// SummaryErrorForTest exposes summaryError for external tests.
var SummaryErrorForTest = summaryError

// MaxSummaryErrorLenForTest exposes the persisted-error byte cap for
// external tests.
const MaxSummaryErrorLenForTest = maxSummaryErrorLen
