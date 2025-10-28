package agentsocket

import (
	"context"
	"net"
)

// AuthMiddleware defines the interface for authentication middleware
type AuthMiddleware interface {
	// Authenticate authenticates a connection and returns a context with auth info
	Authenticate(ctx context.Context, conn net.Conn) (context.Context, error)
}

// NoAuthMiddleware is a no-op authentication middleware
type NoAuthMiddleware struct{}

// Authenticate implements AuthMiddleware but performs no authentication
func (*NoAuthMiddleware) Authenticate(ctx context.Context, conn net.Conn) (context.Context, error) {
	return ctx, nil
}

// Ensure NoAuthMiddleware implements AuthMiddleware
var _ AuthMiddleware = (*NoAuthMiddleware)(nil)
