package chattool_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/quartz"
)

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
			ScreenshotData:   "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg==",
			ScreenshotWidth:  geometry.DeclaredWidth,
			ScreenshotHeight: geometry.DeclaredHeight,
		}, nil
	})

	tool := chattool.NewComputerUseTool(geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
		return mockConn, nil
	}, nil, quartz.NewReal(), slogtest.Make(t, nil))

	call := fantasy.ToolCall{
		ID:    "test-1",
		Name:  "computer",
		Input: `{"action":"screenshot"}`,
	}

	resp, err := tool.Run(context.Background(), call)
	require.NoError(t, err)
	assert.Equal(t, "image", resp.Type)
	assert.Equal(t, "image/png", resp.MediaType)
	assert.Equal(t, []byte("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg=="), resp.Data)
	assert.False(t, resp.IsError)
}

func TestComputerUseTool_Run_Screenshot_PersistsAttachment(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()
	const screenshotPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg=="

	mockConn.EXPECT().ExecuteDesktopAction(
		gomock.Any(),
		gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
	).DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
		require.Equal(t, "screenshot", action.Action)
		return workspacesdk.DesktopActionResponse{
			Output:           "screenshot",
			ScreenshotData:   screenshotPNG,
			ScreenshotWidth:  geometry.DeclaredWidth,
			ScreenshotHeight: geometry.DeclaredHeight,
		}, nil
	})

	var storedName string
	var storedType string
	var storedData []byte
	tool := chattool.NewComputerUseTool(geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
		return mockConn, nil
	}, func(_ context.Context, name string, detectName string, data []byte) (chattool.AttachmentMetadata, error) {
		storedName = name
		require.Equal(t, name, detectName)
		storedType = "image/png"
		storedData = append([]byte(nil), data...)
		return chattool.AttachmentMetadata{
			FileID:    uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
			MediaType: storedType,
			Name:      name,
		}, nil
	}, quartz.NewReal(), slogtest.Make(t, nil))

	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID: "test-screenshot-persist", Name: "computer", Input: `{"action":"screenshot"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "image", resp.Type)
	assert.Equal(t, "image/png", resp.MediaType)
	assert.Equal(t, []byte(screenshotPNG), resp.Data)
	assert.Contains(t, storedName, "screenshot-")
	assert.Equal(t, "image/png", storedType)
	expectedPNG, decodeErr := base64.StdEncoding.DecodeString(screenshotPNG)
	require.NoError(t, decodeErr)
	require.Equal(t, expectedPNG, storedData)

	attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
	require.NoError(t, err)
	require.Len(t, attachments, 1)
	assert.Equal(t, uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"), attachments[0].FileID)
	assert.Equal(t, "image/png", attachments[0].MediaType)
}

func TestComputerUseTool_Run_Screenshot_StoreErrorFallsBackToImage(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()
	const screenshotPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg=="

	mockConn.EXPECT().ExecuteDesktopAction(
		gomock.Any(),
		gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
	).Return(workspacesdk.DesktopActionResponse{
		Output:           "screenshot",
		ScreenshotData:   screenshotPNG,
		ScreenshotWidth:  geometry.DeclaredWidth,
		ScreenshotHeight: geometry.DeclaredHeight,
	}, nil)

	tool := chattool.NewComputerUseTool(geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
		return mockConn, nil
	}, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
		return chattool.AttachmentMetadata{}, xerrors.New("chat already has the maximum of 20 linked files")
	}, quartz.NewReal(), slogtest.Make(t, nil))

	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID: "test-screenshot-store-error", Name: "computer", Input: `{"action":"screenshot"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "image", resp.Type)
	assert.Equal(t, "image/png", resp.MediaType)
	assert.False(t, resp.IsError)
	attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
	require.NoError(t, err)
	assert.Empty(t, attachments)
}

func TestComputerUseTool_Run_Screenshot_OversizedAttachmentFallsBackToImage(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()
	oversizedScreenshot := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0xAB}, 10<<20+1))

	mockConn.EXPECT().ExecuteDesktopAction(
		gomock.Any(),
		gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
	).Return(workspacesdk.DesktopActionResponse{
		Output:           "screenshot",
		ScreenshotData:   oversizedScreenshot,
		ScreenshotWidth:  geometry.DeclaredWidth,
		ScreenshotHeight: geometry.DeclaredHeight,
	}, nil)

	tool := chattool.NewComputerUseTool(geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
		return mockConn, nil
	}, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
		t.Fatal("storeFile should not be called for oversized screenshots")
		return chattool.AttachmentMetadata{}, nil
	}, quartz.NewReal(), slogtest.Make(t, nil))

	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID: "test-screenshot-oversized", Name: "computer", Input: `{"action":"screenshot"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "image", resp.Type)
	assert.Equal(t, "image/png", resp.MediaType)
	assert.False(t, resp.IsError)
	require.Len(t, resp.Data, len(oversizedScreenshot))
	attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
	require.NoError(t, err)
	assert.Empty(t, attachments)
}

func TestComputerUseTool_Run_LeftClick(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()
	followUpScreenshot := base64.StdEncoding.EncodeToString([]byte("after-click"))

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
			ScreenshotData:   followUpScreenshot,
			ScreenshotWidth:  geometry.DeclaredWidth,
			ScreenshotHeight: geometry.DeclaredHeight,
		}, nil
	})

	tool := chattool.NewComputerUseTool(geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
		return mockConn, nil
	}, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
		t.Fatal("storeFile should not be called for left_click follow-up screenshots")
		return chattool.AttachmentMetadata{}, nil
	}, quartz.NewReal(), slogtest.Make(t, nil))

	call := fantasy.ToolCall{
		ID:    "test-2",
		Name:  "computer",
		Input: `{"action":"left_click","coordinate":[100,200]}`,
	}

	resp, err := tool.Run(context.Background(), call)
	require.NoError(t, err)
	assert.Equal(t, "image", resp.Type)
	assert.Equal(t, []byte(followUpScreenshot), resp.Data)
	attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
	require.NoError(t, err)
	assert.Empty(t, attachments)
}

func TestComputerUseTool_Run_Wait(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()
	followUpScreenshot := base64.StdEncoding.EncodeToString([]byte("after-wait"))

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
			ScreenshotData:   followUpScreenshot,
			ScreenshotWidth:  geometry.DeclaredWidth,
			ScreenshotHeight: geometry.DeclaredHeight,
		}, nil
	})

	tool := chattool.NewComputerUseTool(geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
		return mockConn, nil
	}, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
		t.Fatal("storeFile should not be called for wait screenshots")
		return chattool.AttachmentMetadata{}, nil
	}, quartz.NewReal(), slogtest.Make(t, nil))

	call := fantasy.ToolCall{
		ID:    "test-3",
		Name:  "computer",
		Input: `{"action":"wait","duration":10}`,
	}

	resp, err := tool.Run(context.Background(), call)
	require.NoError(t, err)
	assert.Equal(t, "image", resp.Type)
	assert.Equal(t, "image/png", resp.MediaType)
	assert.Equal(t, []byte(followUpScreenshot), resp.Data)
	assert.False(t, resp.IsError)
	attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
	require.NoError(t, err)
	assert.Empty(t, attachments)
}

func TestComputerUseTool_Run_ConnError(t *testing.T) {
	t.Parallel()

	geometry := workspacesdk.DefaultDesktopGeometry()
	tool := chattool.NewComputerUseTool(geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
		return nil, xerrors.New("workspace not available")
	}, nil, quartz.NewReal(), slogtest.Make(t, nil))

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
	}, nil, quartz.NewReal(), slogtest.Make(t, nil))

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
