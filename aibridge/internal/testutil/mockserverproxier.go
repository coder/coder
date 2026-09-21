package testutil

import (
	"context"

	mcpgo "github.com/modelcontextprotocol/go-sdk/mcp"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/mcp"
)

// MockServerProxier is a test [mcp.ServerProxier] that injects a fixed set of
// tools. When ResolveAnyTool is set, ListTools reports a stub tool and GetTool
// resolves any unregistered tool to a stub, so callers that only need the tool
// loop to proceed need not register each tool the fixture might call.
type MockServerProxier struct {
	Tools []*mcp.Tool
	// ResolveAnyTool makes GetTool return a stub tool, backed by a
	// StubToolCaller, for any id not present in Tools, and makes ListTools
	// report one stub when Tools is empty so the interceptor injects tools.
	// Use it to exercise injected-tool agentic loops where the test does not
	// need to validate which tool was called.
	ResolveAnyTool bool
}

func (*MockServerProxier) Init(context.Context) error {
	return nil
}

func (*MockServerProxier) Shutdown(context.Context) error {
	return nil
}

func (m *MockServerProxier) ListTools() []*mcp.Tool {
	if len(m.Tools) == 0 && m.ResolveAnyTool {
		return []*mcp.Tool{stubTool("stub")}
	}
	return m.Tools
}

func (m *MockServerProxier) GetTool(id string) *mcp.Tool {
	for _, t := range m.Tools {
		if t.ID == id {
			return t
		}
	}
	if m.ResolveAnyTool {
		return stubTool(id)
	}
	return nil
}

func stubTool(id string) *mcp.Tool {
	return &mcp.Tool{
		Client:     StubToolCaller{},
		ID:         id,
		Name:       id,
		ServerName: "coder",
		Logger:     slog.Make(),
	}
}

func (*MockServerProxier) CallTool(context.Context, string, any) (*mcpgo.CallToolResult, error) {
	return nil, nil //nolint:nilnil // mock: no-op implementation
}

// StubToolCaller is a minimal tool client that returns a fixed text result.
type StubToolCaller struct{}

func (StubToolCaller) CallTool(_ context.Context, _ *mcpgo.CallToolParams) (*mcpgo.CallToolResult, error) {
	return &mcpgo.CallToolResult{
		Content: []mcpgo.Content{&mcpgo.TextContent{Text: "tool result"}},
	}, nil
}
