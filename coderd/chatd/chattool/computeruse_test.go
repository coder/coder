package chattool_test

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/quartz"
)

func TestComputerUseTool_Info(t *testing.T) {
	t.Parallel()

	tool := chattool.NewComputerUseTool(workspacesdk.DesktopDisplayWidth, workspacesdk.DesktopDisplayHeight, nil, quartz.NewReal())
	info := tool.Info()
	assert.Equal(t, "computer", info.Name)
	assert.NotEmpty(t, info.Description)
}

func TestComputerUseProviderTool(t *testing.T) {
	t.Parallel()

	def := chattool.ComputerUseProviderTool(workspacesdk.DesktopDisplayWidth, workspacesdk.DesktopDisplayHeight)
	pdt, ok := def.(fantasy.ProviderDefinedTool)
	require.True(t, ok, "ComputerUseProviderTool should return a ProviderDefinedTool")
	assert.Contains(t, pdt.ID, "computer")
	assert.Equal(t, "computer", pdt.Name)
	// Verify display dimensions are passed through.
	assert.Equal(t, int64(workspacesdk.DesktopDisplayWidth), pdt.Args["display_width_px"])
	assert.Equal(t, int64(workspacesdk.DesktopDisplayHeight), pdt.Args["display_height_px"])
}

func TestComputerUseTool_Run_Screenshot(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	mockConn.EXPECT().ExecuteDesktopAction(
		gomock.Any(),
		gomock.Any(),
	).Return(workspacesdk.DesktopActionResponse{
		Output:           "screenshot",
		ScreenshotData:   "base64png",
		ScreenshotWidth:  1024,
		ScreenshotHeight: 768,
	}, nil)

	tool := chattool.NewComputerUseTool(workspacesdk.DesktopDisplayWidth, workspacesdk.DesktopDisplayHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
		return mockConn, nil
	}, quartz.NewReal())

	call := fantasy.ToolCall{
		ID:    "test-1",
		Name:  "computer",
		Input: `{"action":"screenshot"}`,
	}

	resp, err := tool.Run(context.Background(), call)
	require.NoError(t, err)
	assert.Equal(t, "image", resp.Type)
	assert.Equal(t, "image/png", resp.MediaType)
	assert.Equal(t, []byte("base64png"), resp.Data)
	assert.False(t, resp.IsError)
}

func TestComputerUseTool_Run_LeftClick(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)

	// Expect the action call first.
	mockConn.EXPECT().ExecuteDesktopAction(
		gomock.Any(),
		gomock.Any(),
	).Return(workspacesdk.DesktopActionResponse{
		Output: "left_click performed",
	}, nil)

	// Then expect a screenshot (auto-screenshot after action).
	mockConn.EXPECT().ExecuteDesktopAction(
		gomock.Any(),
		gomock.Any(),
	).Return(workspacesdk.DesktopActionResponse{
		Output:           "screenshot",
		ScreenshotData:   "after-click",
		ScreenshotWidth:  1024,
		ScreenshotHeight: 768,
	}, nil)

	tool := chattool.NewComputerUseTool(workspacesdk.DesktopDisplayWidth, workspacesdk.DesktopDisplayHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
		return mockConn, nil
	}, quartz.NewReal())

	call := fantasy.ToolCall{
		ID:    "test-2",
		Name:  "computer",
		Input: `{"action":"left_click","coordinate":[100,200]}`,
	}

	resp, err := tool.Run(context.Background(), call)
	require.NoError(t, err)
	assert.Equal(t, "image", resp.Type)
	assert.Equal(t, []byte("after-click"), resp.Data)
}

func TestComputerUseTool_Run_Wait(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	// Expect a screenshot after the wait completes.
	mockConn.EXPECT().ExecuteDesktopAction(
		gomock.Any(),
		gomock.Any(),
	).Return(workspacesdk.DesktopActionResponse{
		Output:           "screenshot",
		ScreenshotData:   "after-wait",
		ScreenshotWidth:  1024,
		ScreenshotHeight: 768,
	}, nil)

	tool := chattool.NewComputerUseTool(workspacesdk.DesktopDisplayWidth, workspacesdk.DesktopDisplayHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
		return mockConn, nil
	}, quartz.NewReal())

	call := fantasy.ToolCall{
		ID:    "test-3",
		Name:  "computer",
		Input: `{"action":"wait","duration":10}`,
	}

	resp, err := tool.Run(context.Background(), call)
	require.NoError(t, err)
	assert.Equal(t, "image", resp.Type)
	assert.Equal(t, "image/png", resp.MediaType)
	assert.Equal(t, []byte("after-wait"), resp.Data)
	assert.False(t, resp.IsError)
}

func TestComputerUseTool_Run_ConnError(t *testing.T) {
	t.Parallel()

	tool := chattool.NewComputerUseTool(workspacesdk.DesktopDisplayWidth, workspacesdk.DesktopDisplayHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
		return nil, xerrors.New("workspace not available")
	}, quartz.NewReal())

	call := fantasy.ToolCall{
		ID:    "test-4",
		Name:  "computer",
		Input: `{"action":"screenshot"}`,
	}

	resp, err := tool.Run(context.Background(), call)
	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "workspace not available")
}

func TestComputerUseTool_Run_InvalidInput(t *testing.T) {
	t.Parallel()

	tool := chattool.NewComputerUseTool(workspacesdk.DesktopDisplayWidth, workspacesdk.DesktopDisplayHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
		return nil, xerrors.New("should not be called")
	}, quartz.NewReal())

	call := fantasy.ToolCall{
		ID:    "test-5",
		Name:  "computer",
		Input: `{invalid json`,
	}

	resp, err := tool.Run(context.Background(), call)
	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "invalid computer use input")
}
