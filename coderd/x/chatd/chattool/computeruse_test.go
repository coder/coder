package chattool_test

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/quartz"
)

func TestComputerUseTool_Info(t *testing.T) {
	t.Parallel()

	geometry := workspacesdk.DefaultDesktopGeometry()
	tool := chattool.NewComputerUseTool(geometry.DeclaredWidth, geometry.DeclaredHeight, nil, quartz.NewReal())
	info := tool.Info()
	assert.Equal(t, "computer", info.Name)
	assert.NotEmpty(t, info.Description)
}

func TestComputerUseProviderTool(t *testing.T) {
	t.Parallel()

	geometry := workspacesdk.DefaultDesktopGeometry()
	def := chattool.ComputerUseProviderTool(geometry.DeclaredWidth, geometry.DeclaredHeight)
	pdt, ok := def.(fantasy.ProviderDefinedTool)
	require.True(t, ok, "ComputerUseProviderTool should return a ProviderDefinedTool")
	assert.Contains(t, pdt.ID, "computer")
	assert.Equal(t, "computer", pdt.Name)
	assert.Equal(t, int64(geometry.DeclaredWidth), pdt.Args["display_width_px"])
	assert.Equal(t, int64(geometry.DeclaredHeight), pdt.Args["display_height_px"])
}

func TestComputerUseProviderTool_PrefersDeclaredGeometry(t *testing.T) {
	t.Parallel()

	geometry := workspacesdk.NewDesktopGeometry(1920, 1080)
	def := chattool.ComputerUseProviderTool(geometry.DeclaredWidth, geometry.DeclaredHeight)
	pdt, ok := def.(fantasy.ProviderDefinedTool)
	require.True(t, ok, "ComputerUseProviderTool should return a ProviderDefinedTool")
	assert.Equal(t, int64(1280), pdt.Args["display_width_px"])
	assert.Equal(t, int64(720), pdt.Args["display_height_px"])
}

func TestComputerUseTool_Run_Screenshot(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()

	mockConn.EXPECT().ExecuteDesktopAction(
		gomock.Any(),
		gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
	).DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
		require.NotNil(t, action.ScaledWidth)
		require.NotNil(t, action.ScaledHeight)
		assert.Equal(t, geometry.DeclaredWidth, *action.ScaledWidth)
		assert.Equal(t, geometry.DeclaredHeight, *action.ScaledHeight)
		return workspacesdk.DesktopActionResponse{
			Output:           "screenshot",
			ScreenshotData:   "base64png",
			ScreenshotWidth:  geometry.DeclaredWidth,
			ScreenshotHeight: geometry.DeclaredHeight,
		}, nil
	})

	tool := chattool.NewComputerUseTool(geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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
	geometry := workspacesdk.DefaultDesktopGeometry()

	mockConn.EXPECT().ExecuteDesktopAction(
		gomock.Any(),
		gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
	).DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
		require.NotNil(t, action.Coordinate)
		assert.Equal(t, [2]int{100, 200}, *action.Coordinate)
		require.NotNil(t, action.ScaledWidth)
		require.NotNil(t, action.ScaledHeight)
		assert.Equal(t, geometry.DeclaredWidth, *action.ScaledWidth)
		assert.Equal(t, geometry.DeclaredHeight, *action.ScaledHeight)
		return workspacesdk.DesktopActionResponse{Output: "left_click performed"}, nil
	})

	mockConn.EXPECT().ExecuteDesktopAction(
		gomock.Any(),
		gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
	).DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
		assert.Equal(t, "screenshot", action.Action)
		require.NotNil(t, action.ScaledWidth)
		require.NotNil(t, action.ScaledHeight)
		assert.Equal(t, geometry.DeclaredWidth, *action.ScaledWidth)
		assert.Equal(t, geometry.DeclaredHeight, *action.ScaledHeight)
		return workspacesdk.DesktopActionResponse{
			Output:           "screenshot",
			ScreenshotData:   "after-click",
			ScreenshotWidth:  geometry.DeclaredWidth,
			ScreenshotHeight: geometry.DeclaredHeight,
		}, nil
	})

	tool := chattool.NewComputerUseTool(geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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
	geometry := workspacesdk.DefaultDesktopGeometry()

	mockConn.EXPECT().ExecuteDesktopAction(
		gomock.Any(),
		gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
	).DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
		require.NotNil(t, action.ScaledWidth)
		require.NotNil(t, action.ScaledHeight)
		assert.Equal(t, geometry.DeclaredWidth, *action.ScaledWidth)
		assert.Equal(t, geometry.DeclaredHeight, *action.ScaledHeight)
		return workspacesdk.DesktopActionResponse{
			Output:           "screenshot",
			ScreenshotData:   "after-wait",
			ScreenshotWidth:  geometry.DeclaredWidth,
			ScreenshotHeight: geometry.DeclaredHeight,
		}, nil
	})

	tool := chattool.NewComputerUseTool(geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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

	geometry := workspacesdk.DefaultDesktopGeometry()
	tool := chattool.NewComputerUseTool(geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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

	geometry := workspacesdk.DefaultDesktopGeometry()
	tool := chattool.NewComputerUseTool(geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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
