package testutil

import (
	"context"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/coder/coder/v2/aibridge/mcp"
)

// MockServerProxier is a test [mcp.ServerProxier] that injects a fixed set of
// tools.
type MockServerProxier struct {
	Tools []*mcp.Tool
}

func (*MockServerProxier) Init(context.Context) error {
	return nil
}

func (*MockServerProxier) Shutdown(context.Context) error {
	return nil
}

func (m *MockServerProxier) ListTools() []*mcp.Tool {
	return m.Tools
}

func (m *MockServerProxier) GetTool(id string) *mcp.Tool {
	for _, t := range m.Tools {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (*MockServerProxier) CallTool(context.Context, string, any) (*mcpgo.CallToolResult, error) {
	return nil, nil //nolint:nilnil // mock: no-op implementation
}

// StubToolCaller is a minimal tool client that returns a fixed text result.
type StubToolCaller struct{}

func (StubToolCaller) CallTool(_ context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return mcpgo.NewToolResultText("tool result"), nil
}
