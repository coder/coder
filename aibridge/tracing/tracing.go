package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type (
	traceInterceptionAttrsContextKey  struct{}
	traceRequestBridgeAttrsContextKey struct{}
)

const (
	// trace attribute key constants
	RequestPath = "request_path"

	InterceptionID = "interception_id"
	InitiatorID    = "user_id"
	Provider       = "provider"
	Model          = "model"
	Streaming      = "streaming"
	IsBedrock      = "aws_bedrock"

	PassthroughURL         = "passthrough_url"
	PassthroughUpstreamURL = "passthrough_upstream_url"
	PassthroughMethod      = "passthrough_method"

	MCPInput      = "mcp_input"
	MCPProxyName  = "mcp_proxy_name"
	MCPToolName   = "mcp_tool_name"
	MCPServerName = "mcp_server_name"
	MCPServerURL  = "mcp_server_url"
	MCPToolCount  = "mcp_tool_count"

	APIKeyID = "api_key_id"
)

// EndSpanErr ends given span and sets Error status if error is not nil
// uses pointer to error because defer evaluates function arguments
// when defer statement is executed not when deferred function is called
//
// example usage:
//
//	func Example() (result any, outErr error) {
//	    _, span := tracer.Start(...)
//	    defer tracing.EndSpanErr(span, &outErr)
//
// }
func EndSpanErr(span trace.Span, err *error) {
	if span == nil {
		return
	}

	if err != nil && *err != nil {
		span.SetStatus(codes.Error, (*err).Error())
	}
	span.End()
}

func WithInterceptionAttributesInContext(ctx context.Context, traceAttrs []attribute.KeyValue) context.Context {
	return context.WithValue(ctx, traceInterceptionAttrsContextKey{}, traceAttrs)
}

func InterceptionAttributesFromContext(ctx context.Context) []attribute.KeyValue {
	attrs, ok := ctx.Value(traceInterceptionAttrsContextKey{}).([]attribute.KeyValue)
	if !ok {
		return nil
	}

	return attrs
}

func WithRequestBridgeAttributesInContext(ctx context.Context, traceAttrs []attribute.KeyValue) context.Context {
	return context.WithValue(ctx, traceRequestBridgeAttrsContextKey{}, traceAttrs)
}

func RequestBridgeAttributesFromContext(ctx context.Context) []attribute.KeyValue {
	attrs, ok := ctx.Value(traceRequestBridgeAttrsContextKey{}).([]attribute.KeyValue)
	if !ok {
		return nil
	}

	return attrs
}
