package agentsocket

import (
	"time"

	"cdr.dev/slog"
)

// AgentInfo represents information about the agent
type AgentInfo struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	Uptime    string    `json:"uptime"`
}

// PingResponse represents a ping response
type PingResponse struct {
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// HealthResponse represents a health check response
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Uptime    string    `json:"uptime"`
}

// HandlerContext provides context for handlers
type HandlerContext struct {
	AgentID   string
	Version   string
	Status    string
	StartedAt time.Time
	Logger    slog.Logger
}

// NewHandlers creates the default set of handlers
func NewHandlers(handlerCtx HandlerContext) map[string]Handler {
	handlers := make(map[string]Handler)

	// Ping handler
	handlers["ping"] = func(_ Context, req *Request) (*Response, error) {
		resp := PingResponse{
			Message:   "pong",
			Timestamp: time.Now(),
		}
		return NewResponse(req.ID, resp)
	}

	// Health check handler
	handlers["health"] = func(_ Context, req *Request) (*Response, error) {
		uptime := time.Since(handlerCtx.StartedAt)
		resp := HealthResponse{
			Status:    handlerCtx.Status,
			Timestamp: time.Now(),
			Uptime:    uptime.String(),
		}
		return NewResponse(req.ID, resp)
	}

	// Agent info handler
	handlers["agent.info"] = func(_ Context, req *Request) (*Response, error) {
		uptime := time.Since(handlerCtx.StartedAt)
		resp := AgentInfo{
			ID:        handlerCtx.AgentID,
			Version:   handlerCtx.Version,
			Status:    handlerCtx.Status,
			StartedAt: handlerCtx.StartedAt,
			Uptime:    uptime.String(),
		}
		return NewResponse(req.ID, resp)
	}

	// List methods handler
	handlers["methods.list"] = func(_ Context, req *Request) (*Response, error) {
		methods := []string{
			"ping",
			"health",
			"agent.info",
			"methods.list",
		}
		return NewResponse(req.ID, methods)
	}

	return handlers
}

// RegisterDefaultHandlers registers the default set of handlers with a server
func RegisterDefaultHandlers(server *Server, ctx HandlerContext) {
	handlers := NewHandlers(ctx)
	for method, handler := range handlers {
		server.RegisterHandler(method, handler)
	}
}

// CreateHandlerContext creates a handler context from agent information
func CreateHandlerContext(agentID, version, status string, startedAt time.Time, logger slog.Logger) HandlerContext {
	return HandlerContext{
		AgentID:   agentID,
		Version:   version,
		Status:    status,
		StartedAt: startedAt,
		Logger:    logger,
	}
}
