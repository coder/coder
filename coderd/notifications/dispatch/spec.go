package dispatch

import (
	"context"

	"github.com/google/uuid"
)

// DeliveryFunc delivers the notification.
// The first return param indicates whether a retry can be attempted (i.e. a temporary error), and the second returns
// any error that may have arisen.
// If (false, nil) is returned, that is considered a successful dispatch.
type DeliveryFunc func(ctx context.Context, msgID uuid.UUID) (retryable bool, err error)
