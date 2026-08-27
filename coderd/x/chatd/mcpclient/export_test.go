package mcpclient

import (
	"context"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
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
	return connectAllWithHooks(
		ctx, logger, configs, nil, uuid.Nil, nil, nil,
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
