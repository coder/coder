package mcp

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/tracing"
)

const (
	maxSpanInputAttrLen   = 100    // truncates tool.Call span input attribute to first `maxSpanInputAttrLen` letters
	injectedToolPrefix    = "bmcp" // "bridged MCP"
	injectedToolDelimiter = "_"
)

// ToolCaller is the narrowest interface which describes the behavior required from [mcp.Client],
// which will normally be passed into [Tool] for interaction with an MCP server.
// TODO: don't expose github.com/mark3labs/mcp-go outside this package.
type ToolCaller interface {
	CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

type Tool struct {
	Client ToolCaller

	ID          string
	Name        string
	ServerName  string
	ServerURL   string
	Description string
	Params      map[string]any
	Required    []string
	Logger      slog.Logger
}

func (t *Tool) Call(ctx context.Context, input any, tracer trace.Tracer) (_ *mcp.CallToolResult, outErr error) {
	if t == nil {
		return nil, xerrors.New("nil tool")
	}
	if t.Client == nil {
		return nil, xerrors.New("nil client")
	}

	spanAttrs := append(
		tracing.InterceptionAttributesFromContext(ctx),
		attribute.String(tracing.MCPToolName, t.Name),
		attribute.String(tracing.MCPServerName, t.ServerName),
		attribute.String(tracing.MCPServerURL, t.ServerURL),
	)
	ctx, span := tracer.Start(ctx, "Intercept.ProcessRequest.ToolCall", trace.WithAttributes(spanAttrs...))
	defer tracing.EndSpanErr(span, &outErr)

	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Logger.Warn(ctx, "failed to marshal tool input, will be omitted from span attrs", slog.Error(err))
	} else {
		strJSON := string(inputJSON)
		if len(strJSON) > maxSpanInputAttrLen {
			strJSON = strJSON[:maxSpanInputAttrLen]
		}
		span.SetAttributes(attribute.String(tracing.MCPInput, strJSON))
	}

	start := time.Now()
	var res *mcp.CallToolResult
	res, outErr = t.Client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      t.Name,
			Arguments: input,
		},
	})

	logFn := t.Logger.Debug
	if outErr != nil {
		logFn = t.Logger.Warn
	}

	// We don't log MCP results because they could be large or contain sensitive information.
	logFn(ctx, "injected tool invoked",
		slog.F("name", t.Name),
		slog.F("server", t.ServerName),
		slog.F("input", inputJSON),
		slog.F("duration_sec", time.Since(start).Seconds()),
		slog.Error(outErr),
	)

	return res, outErr
}

// EncodeToolID namespaces the given tool name with a prefix to identify tools injected by this library.
// Claude Code, for example, prefixes the tools it includes from defined MCP servers with the "mcp__" prefix.
// We have to namespace the tools we inject to prevent clashes.
//
// We stick to 5 prefix chars ("bmcp_") like "mcp__" since names can only be up to 64 chars:
//
// See:
// - https://community.openai.com/t/function-call-description-max-length/529902
// - https://github.com/anthropics/claude-code/issues/2326
func EncodeToolID(server, tool string) string {
	// strings.Builder writes to in-memory storage and never return errors.
	var sb strings.Builder
	_, _ = sb.WriteString(injectedToolPrefix)
	_, _ = sb.WriteString(injectedToolDelimiter)
	_, _ = sb.WriteString(server)
	_, _ = sb.WriteString(injectedToolDelimiter)
	_, _ = sb.WriteString(tool)
	return sb.String()
}

// FilterAllowedTools filters tools based on the given allow/denylists.
// Filtering acts on tool names, and uses tool IDs for tracking.
// The denylist supersedes the allowlist in the case of any conflicts.
// If an allowlist is provided, tools must match it to be allowed.
// If only a denylist is provided, tools are allowed unless explicitly denied.
func FilterAllowedTools(logger slog.Logger, tools map[string]*Tool, allowlist *regexp.Regexp, denylist *regexp.Regexp) map[string]*Tool {
	if len(tools) == 0 {
		return tools
	}

	if allowlist == nil && denylist == nil {
		return tools
	}

	allowed := make(map[string]*Tool, len(tools))
	for id, tool := range tools {
		if tool == nil {
			continue
		}

		// Check denylist first since it can override allowlist.
		if denylist != nil && denylist.MatchString(tool.Name) {
			// Log conflict if also in allowlist.
			if allowlist != nil && allowlist.MatchString(tool.Name) {
				logger.Warn(context.Background(), "tool filtering conflict; marking tool disallowed", slog.F("name", tool.Name))
			}
			continue // Not allowed.
		}

		// Check allowlist if present.
		if allowlist != nil {
			if !allowlist.MatchString(tool.Name) {
				continue // Not allowed.
			}
		}

		// Tool is allowed.
		allowed[id] = tool
	}

	return allowed
}
