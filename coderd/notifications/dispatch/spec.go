package dispatch

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/runtimeconfig"
)

// DeliveryFunc delivers the notification.
// The first return param indicates whether a retry can be attempted (i.e. a temporary error), and the second returns
// any error that may have arisen.
// If (false, nil) is returned, that is considered a successful dispatch.
type DeliveryFunc func(ctx context.Context, cfgResolver runtimeconfig.Resolver, msgID uuid.UUID) (retryable bool, err error)
