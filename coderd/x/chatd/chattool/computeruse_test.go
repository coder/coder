package chattool_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"
	"time"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	openaicomputeruse "github.com/coder/coder/v2/coderd/x/chatd/chatopenai/computeruse"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestDefaultComputerUseModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		provider          string
		wantModelProvider string
		wantModelName     string
		wantOK            bool
	}{
		{
			name:              "empty defaults to Anthropic",
			provider:          "",
			wantModelProvider: chattool.ComputerUseModelProviderDefault,
			wantModelName:     chattool.ComputerUseAnthropicModelName,
			wantOK:            true,
		},
		{
			name:              "Anthropic",
			provider:          chattool.ComputerUseProviderAnthropic,
			wantModelProvider: chattool.ComputerUseModelProviderDefault,
			wantModelName:     chattool.ComputerUseAnthropicModelName,
			wantOK:            true,
		},
		{
			name:              "OpenAI",
			provider:          chattool.ComputerUseProviderOpenAI,
			wantModelProvider: chattool.ComputerUseProviderOpenAI,
			wantModelName:     chattool.ComputerUseOpenAIModelName,
			wantOK:            true,
		},
		{
			name:     "unsupported",
			provider: "unsupported",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			modelProvider, modelName, ok := chattool.DefaultComputerUseModel(tt.provider)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantModelProvider, modelProvider)
			assert.Equal(t, tt.wantModelName, modelName)
		})
	}
}

func TestDefaultComputerUseDesktopGeometry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		provider       string
		declaredWidth  int
		declaredHeight int
	}{
		{
			name:           "empty defaults to Anthropic geometry",
			provider:       "",
			declaredWidth:  1280,
			declaredHeight: 720,
		},
		{
			name:           "Anthropic",
			provider:       chattool.ComputerUseProviderAnthropic,
			declaredWidth:  1280,
			declaredHeight: 720,
		},
		{
			name:           "OpenAI",
			provider:       chattool.ComputerUseProviderOpenAI,
			declaredWidth:  1600,
			declaredHeight: 900,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			geometry := chattool.DefaultComputerUseDesktopGeometry(tt.provider)
			assert.Equal(t, tt.declaredWidth, geometry.DeclaredWidth)
			assert.Equal(t, tt.declaredHeight, geometry.DeclaredHeight)
		})
	}
}

func TestComputerUseProviderTool(t *testing.T) {
	t.Parallel()

	geometry := workspacesdk.DefaultDesktopGeometry()
	def, err := chattool.ComputerUseProviderTool(
		chattool.ComputerUseProviderAnthropic,
		geometry.DeclaredWidth,
		geometry.DeclaredHeight,
	)
	require.NoError(t, err)
	pdt, ok := def.(fantasy.ProviderDefinedTool)
	require.True(t, ok, "ComputerUseProviderTool should return a ProviderDefinedTool")
	assert.True(t, fantasyanthropic.IsComputerUseTool(def))
	assert.Contains(t, pdt.ID, "computer")
	assert.Equal(t, "computer", pdt.Name)
	assert.Equal(t, int64(geometry.DeclaredWidth), pdt.Args["display_width_px"])
	assert.Equal(t, int64(geometry.DeclaredHeight), pdt.Args["display_height_px"])

	openAITool, err := chattool.ComputerUseProviderTool(
		chattool.ComputerUseProviderOpenAI,
		geometry.DeclaredWidth,
		geometry.DeclaredHeight,
	)
	require.NoError(t, err)
	assert.True(t, openaicomputeruse.IsTool(openAITool))

	_, err = chattool.ComputerUseProviderTool(
		"unsupported",
		geometry.DeclaredWidth,
		geometry.DeclaredHeight,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported computer use provider")
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

	tool := chattool.NewComputerUseTool(chattool.ComputerUseProviderAnthropic, geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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
	expectedBinary, decErr := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg==")
	require.NoError(t, decErr)
	assert.Equal(t, expectedBinary, resp.Data)
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
	tool := chattool.NewComputerUseTool(chattool.ComputerUseProviderAnthropic, geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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
	expectedBinary, decErr := base64.StdEncoding.DecodeString(screenshotPNG)
	require.NoError(t, decErr)
	assert.Equal(t, expectedBinary, resp.Data)
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

	tool := chattool.NewComputerUseTool(chattool.ComputerUseProviderAnthropic, geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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

	tool := chattool.NewComputerUseTool(chattool.ComputerUseProviderAnthropic, geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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
	expectedOversized, decErr := base64.StdEncoding.DecodeString(oversizedScreenshot)
	require.NoError(t, decErr)
	require.Len(t, resp.Data, len(expectedOversized))
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

	tool := chattool.NewComputerUseTool(chattool.ComputerUseProviderAnthropic, geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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
	expectedBinary, decErr := base64.StdEncoding.DecodeString(followUpScreenshot)
	require.NoError(t, decErr)
	assert.Equal(t, expectedBinary, resp.Data)
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

	tool := chattool.NewComputerUseTool(chattool.ComputerUseProviderAnthropic, geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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
	expectedBinary, decErr := base64.StdEncoding.DecodeString(followUpScreenshot)
	require.NoError(t, decErr)
	assert.Equal(t, expectedBinary, resp.Data)
	assert.False(t, resp.IsError)
	attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
	require.NoError(t, err)
	assert.Empty(t, attachments)
}

func TestComputerUseTool_Run_ScreenshotDataIsDecodedBinary(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()

	// A known base64 string (1x1 red PNG).
	const screenshotBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8BQDwAEgAF/pooBPQAAAABJRU5ErkJggg=="

	mockConn.EXPECT().ExecuteDesktopAction(
		gomock.Any(),
		gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
	).Return(workspacesdk.DesktopActionResponse{
		Output:           "screenshot",
		ScreenshotData:   screenshotBase64,
		ScreenshotWidth:  geometry.DeclaredWidth,
		ScreenshotHeight: geometry.DeclaredHeight,
	}, nil)

	tool := chattool.NewComputerUseTool(
		chattool.ComputerUseProviderAnthropic,
		geometry.DeclaredWidth,
		geometry.DeclaredHeight,
		func(_ context.Context) (workspacesdk.AgentConn, error) {
			return mockConn, nil
		},
		nil,
		quartz.NewReal(),
		slogtest.Make(t, nil),
	)

	call := fantasy.ToolCall{
		ID:    "test-decode-1",
		Name:  "computer",
		Input: `{"action":"screenshot"}`,
	}

	resp, err := tool.Run(context.Background(), call)
	require.NoError(t, err)

	assert.Equal(t, "image", resp.Type)
	assert.Equal(t, "image/png", resp.MediaType)

	// Data must contain decoded binary, not the base64 string
	// reinterpreted as bytes.
	expectedBinary, err := base64.StdEncoding.DecodeString(screenshotBase64)
	require.NoError(t, err)
	assert.Equal(t, expectedBinary, resp.Data,
		"ToolResponse.Data should contain decoded binary, not base64-as-bytes")

	// Verify that re-encoding produces the original base64 string.
	// This is the round-trip that the chat loop performs when
	// building the API response.
	reEncoded := base64.StdEncoding.EncodeToString(resp.Data)
	assert.Equal(t, screenshotBase64, reEncoded,
		"re-encoding Data should produce the original base64 string (no double-encode)")
}

func TestComputerUseTool_Run_ConnError(t *testing.T) {
	t.Parallel()

	geometry := workspacesdk.DefaultDesktopGeometry()
	tool := chattool.NewComputerUseTool(chattool.ComputerUseProviderAnthropic, geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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
	tool := chattool.NewComputerUseTool(chattool.ComputerUseProviderAnthropic, geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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

func TestComputerUseTool_Run_OpenAI_BatchedActions(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()
	const screenshotPNG = "aW1hZ2UtZGF0YQ=="
	actions := recordDesktopActions(t, mockConn, geometry, 16, screenshotPNG)

	tool := newOpenAIComputerUseTool(t, geometry, mockConn, nil, quartz.NewReal())
	resp, err := tool.Run(context.Background(), openAIComputerUseCall(`{
		"call_id":"call_batch",
		"actions":[
			{"type":"screenshot"},
			{"type":"move","x":10,"y":20},
			{"type":"click","button":"left","x":30,"y":40},
			{"type":"click","button":"right","x":31,"y":41},
			{"type":"click","button":"middle","x":32,"y":42},
			{"type":"double_click","x":50,"y":60},
			{"type":"drag","path":[{"x":1,"y":2},{"x":3,"y":4},{"x":5,"y":6}]},
			{"type":"keypress","keys":["ctrl","s"]},
			{"type":"type","text":"hello"},
			{"type":"scroll","x":70,"y":80,"scroll_y":500,"scroll_x":-200}
		]
	}`))
	require.NoError(t, err)
	assert.Equal(t, "image", resp.Type)
	assert.Equal(t, "image/png", resp.MediaType)
	assert.False(t, resp.IsError)
	expectedImage, err := base64.StdEncoding.DecodeString(screenshotPNG)
	require.NoError(t, err)
	assert.Equal(t, expectedImage, resp.Data)
	attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
	require.NoError(t, err)
	assert.Empty(t, attachments)

	require.Len(t, *actions, 16)
	for _, action := range *actions {
		assertDesktopActionScaled(t, geometry, action)
	}
	assertDesktopAction(t, (*actions)[0], "mouse_move", [2]int{10, 20})
	assertDesktopAction(t, (*actions)[1], "left_click", [2]int{30, 40})
	assertDesktopAction(t, (*actions)[2], "right_click", [2]int{31, 41})
	assertDesktopAction(t, (*actions)[3], "middle_click", [2]int{32, 42})
	assertDesktopAction(t, (*actions)[4], "double_click", [2]int{50, 60})
	assertDesktopAction(t, (*actions)[5], "mouse_move", [2]int{1, 2})
	assert.Equal(t, "left_mouse_down", (*actions)[6].Action)
	assert.Nil(t, (*actions)[6].Coordinate)
	assertDesktopAction(t, (*actions)[7], "mouse_move", [2]int{3, 4})
	assertDesktopAction(t, (*actions)[8], "mouse_move", [2]int{5, 6})
	assert.Equal(t, "left_mouse_up", (*actions)[9].Action)
	assert.Nil(t, (*actions)[9].Coordinate)
	assertTextAction(t, (*actions)[10], "key", "ctrl+s")
	assertTextAction(t, (*actions)[11], "type", "hello")
	assertDesktopAction(t, (*actions)[12], "mouse_move", [2]int{70, 80})
	assertScrollAction(t, (*actions)[13], [2]int{70, 80}, "down", 5)
	assertScrollAction(t, (*actions)[14], [2]int{70, 80}, "left", 2)
	assert.Equal(t, "screenshot", (*actions)[15].Action)
	assert.Nil(t, (*actions)[15].Coordinate)
}

func TestComputerUseTool_Run_OpenAI_EmptyActionsCapturesScreenshotAndStoresAttachment(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()
	const screenshotPNG = "ZmluYWwtc2NyZWVuc2hvdA=="
	actions := recordDesktopActions(t, mockConn, geometry, 1, screenshotPNG)

	var storedName string
	var storedData []byte
	tool := newOpenAIComputerUseTool(t, geometry, mockConn, func(_ context.Context, name string, detectName string, data []byte) (chattool.AttachmentMetadata, error) {
		storedName = name
		require.Equal(t, name, detectName)
		storedData = append([]byte(nil), data...)
		return chattool.AttachmentMetadata{
			FileID:    uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
			MediaType: "image/png",
			Name:      name,
		}, nil
	}, quartz.NewReal())

	resp, err := tool.Run(context.Background(), openAIComputerUseCall(`{
		"call_id":"call_empty",
		"actions":[]
	}`))
	require.NoError(t, err)
	assert.Equal(t, "image", resp.Type)
	require.Len(t, *actions, 1)
	assert.Equal(t, "screenshot", (*actions)[0].Action)
	assert.Contains(t, storedName, "screenshot-")
	expectedData, err := base64.StdEncoding.DecodeString(screenshotPNG)
	require.NoError(t, err)
	assert.Equal(t, expectedData, storedData)

	attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
	require.NoError(t, err)
	require.Len(t, attachments, 1)
	assert.Equal(t, uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"), attachments[0].FileID)
	assert.Equal(t, "image/png", attachments[0].MediaType)
}

func TestComputerUseTool_Run_OpenAI_FinalScreenshotStoreErrorFallsBackToImage(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()
	const screenshotPNG = "ZmluYWwtc2NyZWVuc2hvdA=="
	recordDesktopActions(t, mockConn, geometry, 1, screenshotPNG)

	tool := newOpenAIComputerUseTool(t, geometry, mockConn, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
		return chattool.AttachmentMetadata{}, xerrors.New("chat already has the maximum of 20 linked files")
	}, quartz.NewReal())

	resp, err := tool.Run(context.Background(), openAIComputerUseCall(`{
		"call_id":"call_store_error",
		"actions":[{"type":"screenshot"}]
	}`))
	require.NoError(t, err)
	assert.Equal(t, "image", resp.Type)
	assert.Equal(t, "image/png", resp.MediaType)
	assert.False(t, resp.IsError)
	attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
	require.NoError(t, err)
	assert.Empty(t, attachments)
}

func TestComputerUseTool_Run_OpenAI_DragReleaseFailureRetriesMouseUp(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()

	gomock.InOrder(
		mockConn.EXPECT().ExecuteDesktopAction(
			gomock.Any(),
			gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
		).DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
			assertDesktopAction(t, action, "mouse_move", [2]int{1, 2})
			assertDesktopActionScaled(t, geometry, action)
			return workspacesdk.DesktopActionResponse{Output: "mouse_move performed"}, nil
		}),
		mockConn.EXPECT().ExecuteDesktopAction(
			gomock.Any(),
			gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
		).DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
			assert.Equal(t, "left_mouse_down", action.Action)
			assertDesktopActionScaled(t, geometry, action)
			return workspacesdk.DesktopActionResponse{Output: "mouse_down performed"}, nil
		}),
		mockConn.EXPECT().ExecuteDesktopAction(
			gomock.Any(),
			gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
		).DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
			assertDesktopAction(t, action, "mouse_move", [2]int{3, 4})
			assertDesktopActionScaled(t, geometry, action)
			return workspacesdk.DesktopActionResponse{Output: "mouse_move performed"}, nil
		}),
		mockConn.EXPECT().ExecuteDesktopAction(
			gomock.Any(),
			gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
		).DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
			assert.Equal(t, "left_mouse_up", action.Action)
			assertDesktopActionScaled(t, geometry, action)
			return workspacesdk.DesktopActionResponse{}, xerrors.New("release failed")
		}),
		mockConn.EXPECT().ExecuteDesktopAction(
			gomock.Any(),
			gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
		).DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
			assert.Equal(t, "left_mouse_up", action.Action)
			assertDesktopActionScaled(t, geometry, action)
			return workspacesdk.DesktopActionResponse{Output: "mouse_up performed"}, nil
		}),
	)

	tool := newOpenAIComputerUseTool(t, geometry, mockConn, nil, quartz.NewReal())
	resp, err := tool.Run(context.Background(), openAIComputerUseCall(`{
		"call_id":"call_release_failure",
		"actions":[{"type":"drag","path":[{"x":1,"y":2},{"x":3,"y":4}]}]
	}`))
	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, `action "left_mouse_up" failed`)
}

func TestComputerUseTool_Run_OpenAI_ActionFailureSkipsFinalScreenshot(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()

	gomock.InOrder(
		mockConn.EXPECT().ExecuteDesktopAction(
			gomock.Any(),
			gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
		).DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
			assertDesktopAction(t, action, "mouse_move", [2]int{10, 20})
			assertDesktopActionScaled(t, geometry, action)
			return workspacesdk.DesktopActionResponse{Output: "mouse_move performed"}, nil
		}),
		mockConn.EXPECT().ExecuteDesktopAction(
			gomock.Any(),
			gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
		).DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
			assertTextAction(t, action, "type", "fail")
			assertDesktopActionScaled(t, geometry, action)
			return workspacesdk.DesktopActionResponse{}, xerrors.New("desktop failed")
		}),
	)

	tool := newOpenAIComputerUseTool(t, geometry, mockConn, nil, quartz.NewReal())
	resp, err := tool.Run(context.Background(), openAIComputerUseCall(`{
		"call_id":"call_failure",
		"actions":[
			{"type":"move","x":10,"y":20},
			{"type":"type","text":"fail"}
		]
	}`))
	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, `action "type" failed`)
}

func TestComputerUseTool_Run_OpenAI_UnsupportedClickButtons(t *testing.T) {
	t.Parallel()

	for _, button := range []string{"extra"} {
		t.Run(button, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			geometry := workspacesdk.DefaultDesktopGeometry()
			tool := newOpenAIComputerUseTool(t, geometry, mockConn, nil, quartz.NewReal())

			resp, err := tool.Run(context.Background(), openAIComputerUseCall(`{
				"call_id":"call_unsupported_button",
				"actions":[{"type":"click","button":"`+button+`","x":10,"y":20}]
			}`))
			require.NoError(t, err)
			assert.True(t, resp.IsError)
			assert.Contains(t, resp.Content, "unsupported OpenAI click button")
		})
	}
}

func TestComputerUseTool_Run_OpenAI_WheelClickIsMiddle(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()
	actions := recordDesktopActions(t, mockConn, geometry, 2, "d2hlZWwtY2xpY2s=")
	tool := newOpenAIComputerUseTool(t, geometry, mockConn, nil, quartz.NewReal())

	resp, err := tool.Run(context.Background(), openAIComputerUseCall(`{
		"call_id":"call_wheel_click",
		"actions":[{"type":"click","button":"wheel","x":10,"y":20}]
	}`))
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	require.Len(t, *actions, 2)
	assertDesktopAction(t, (*actions)[0], "middle_click", [2]int{10, 20})
	assert.Equal(t, "screenshot", (*actions)[1].Action)
}

func TestComputerUseTool_Run_OpenAI_UnsupportedActionType(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()
	tool := newOpenAIComputerUseTool(t, geometry, mockConn, nil, quartz.NewReal())

	resp, err := tool.Run(context.Background(), openAIComputerUseCall(`{
		"call_id":"call_unknown_action",
		"actions":[{"type":"hover","x":10,"y":20}]
	}`))
	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, `unsupported OpenAI computer action type "hover"`)
}

func TestComputerUseTool_Run_OpenAI_InvalidInput(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()
	tool := newOpenAIComputerUseTool(t, geometry, mockConn, nil, quartz.NewReal())

	resp, err := tool.Run(context.Background(), openAIComputerUseCall(`{invalid json`))
	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "invalid")
}

func TestComputerUseTool_Run_OpenAI_DragRequiresTwoPoints(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()
	tool := newOpenAIComputerUseTool(t, geometry, mockConn, nil, quartz.NewReal())

	resp, err := tool.Run(context.Background(), openAIComputerUseCall(`{
		"call_id":"call_short_drag",
		"actions":[{"type":"drag","path":[{"x":10,"y":20}]}]
	}`))
	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "requires at least two path points")
}

func TestComputerUseTool_Run_OpenAI_KeyNormalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		keysJSON string
		wantText string
	}{
		{name: "ctrl s", keysJSON: `["ctrl","s"]`, wantText: "ctrl+s"},
		{name: "modifier aliases", keysJSON: `["control","shift","alt","command","A"]`, wantText: "ctrl+shift+alt+meta+a"},
		{name: "special keys", keysJSON: `["enter","escape","tab","space","backspace","delete"]`, wantText: "Return+Escape+Tab+space+BackSpace+Delete"},
		{name: "arrows", keysJSON: `["ArrowUp","arrowdown","left","Right"]`, wantText: "Up+Down+Left+Right"},
		{name: "function letters digits", keysJSON: `["f1","F12","5","Z"]`, wantText: "F1+F12+5+z"},
		{name: "minus key", keysJSON: `["-"]`, wantText: "-"},
		{name: "equals key", keysJSON: `["="]`, wantText: "="},
		{name: "slash key", keysJSON: `["/"]`, wantText: "/"},
		{name: "period key", keysJSON: `["."]`, wantText: "."},
		{name: "left bracket key", keysJSON: `["["]`, wantText: "["},
		{name: "right bracket key", keysJSON: `["]"]`, wantText: "]"},
		{name: "semicolon key", keysJSON: `[";"]`, wantText: ";"},
		{name: "apostrophe key", keysJSON: `["'"]`, wantText: "'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			geometry := workspacesdk.DefaultDesktopGeometry()
			actions := recordDesktopActions(t, mockConn, geometry, 2, "a2V5LWltYWdl")
			tool := newOpenAIComputerUseTool(t, geometry, mockConn, nil, quartz.NewReal())

			resp, err := tool.Run(context.Background(), openAIComputerUseCall(`{
				"call_id":"call_key",
				"actions":[{"type":"keypress","keys":`+tt.keysJSON+`}]
			}`))
			require.NoError(t, err)
			assert.False(t, resp.IsError)
			require.Len(t, *actions, 2)
			assertTextAction(t, (*actions)[0], "key", tt.wantText)
			assert.Equal(t, "screenshot", (*actions)[1].Action)
		})
	}
}

func TestComputerUseTool_Run_OpenAI_KeyNormalizationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		keysJSON string
		want     string
	}{
		{name: "empty array", keysJSON: `[]`, want: "requires at least one key"},
		{name: "empty token", keysJSON: `["ctrl",""]`, want: "contains an empty key"},
		{name: "unsupported multi-rune", keysJSON: `["ab"]`, want: `unsupported OpenAI keypress "ab"`},
		{name: "unsupported function key", keysJSON: `["f99"]`, want: `unsupported OpenAI keypress "f99"`},
		{name: "unsupported named key", keysJSON: `["PageDown"]`, want: `unsupported OpenAI keypress "PageDown"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			geometry := workspacesdk.DefaultDesktopGeometry()
			tool := newOpenAIComputerUseTool(t, geometry, mockConn, nil, quartz.NewReal())

			resp, err := tool.Run(context.Background(), openAIComputerUseCall(`{
				"call_id":"call_key_error",
				"actions":[{"type":"keypress","keys":`+tt.keysJSON+`}]
			}`))
			require.NoError(t, err)
			assert.True(t, resp.IsError)
			assert.Contains(t, resp.Content, tt.want)
		})
	}
}

func TestComputerUseTool_Run_OpenAI_WaitUsesMockClock(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()
	mClock := quartz.NewMock(t)
	const screenshotPNG = "d2FpdC1zY3JlZW5zaG90"

	mockConn.EXPECT().ExecuteDesktopAction(
		gomock.Any(),
		gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
	).DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
		assert.Equal(t, "screenshot", action.Action)
		assertDesktopActionScaled(t, geometry, action)
		return workspacesdk.DesktopActionResponse{
			Output:           "screenshot",
			ScreenshotData:   screenshotPNG,
			ScreenshotWidth:  geometry.DeclaredWidth,
			ScreenshotHeight: geometry.DeclaredHeight,
		}, nil
	}).Times(1)

	trap := mClock.Trap().NewTimer("computeruse", "wait")
	tool := newOpenAIComputerUseTool(t, geometry, mockConn, nil, mClock)

	type toolResult struct {
		resp fantasy.ToolResponse
		err  error
	}
	resultCh := make(chan toolResult, 1)
	go func() {
		resp, err := tool.Run(ctx, openAIComputerUseCall(`{
			"call_id":"call_wait",
			"actions":[{"type":"wait"}]
		}`))
		resultCh <- toolResult{resp: resp, err: err}
	}()

	trap.MustWait(ctx).MustRelease(ctx)
	trap.Close()
	mClock.Advance(time.Second).MustWait(ctx)

	result := testutil.RequireReceive(ctx, t, resultCh)
	require.NoError(t, result.err)
	assert.Equal(t, "image", result.resp.Type)
	assert.Equal(t, "image/png", result.resp.MediaType)
	assert.False(t, result.resp.IsError)
}

func newOpenAIComputerUseTool(
	t testing.TB,
	geometry workspacesdk.DesktopGeometry,
	conn workspacesdk.AgentConn,
	storeFile chattool.StoreFileFunc,
	clock quartz.Clock,
) fantasy.AgentTool {
	t.Helper()
	return chattool.NewComputerUseTool(
		chattool.ComputerUseProviderOpenAI,
		geometry.DeclaredWidth,
		geometry.DeclaredHeight,
		func(_ context.Context) (workspacesdk.AgentConn, error) {
			return conn, nil
		},
		storeFile,
		clock,
		slogtest.Make(t, nil),
	)
}

func openAIComputerUseCall(input string) fantasy.ToolCall {
	return fantasy.ToolCall{
		ID:    "openai-call",
		Name:  "computer",
		Input: input,
	}
}

func recordDesktopActions(
	t testing.TB,
	mockConn *agentconnmock.MockAgentConn,
	geometry workspacesdk.DesktopGeometry,
	times int,
	screenshotPNG string,
) *[]workspacesdk.DesktopAction {
	t.Helper()
	actions := make([]workspacesdk.DesktopAction, 0, times)
	mockConn.EXPECT().ExecuteDesktopAction(
		gomock.Any(),
		gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
	).DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
		actions = append(actions, action)
		if action.Action == "screenshot" {
			return workspacesdk.DesktopActionResponse{
				Output:           "screenshot",
				ScreenshotData:   screenshotPNG,
				ScreenshotWidth:  geometry.DeclaredWidth,
				ScreenshotHeight: geometry.DeclaredHeight,
			}, nil
		}
		return workspacesdk.DesktopActionResponse{Output: action.Action + " performed"}, nil
	}).Times(times)
	return &actions
}

func assertDesktopActionScaled(
	t testing.TB,
	geometry workspacesdk.DesktopGeometry,
	action workspacesdk.DesktopAction,
) {
	t.Helper()
	require.NotNil(t, action.ScaledWidth)
	require.NotNil(t, action.ScaledHeight)
	assert.Equal(t, geometry.DeclaredWidth, *action.ScaledWidth)
	assert.Equal(t, geometry.DeclaredHeight, *action.ScaledHeight)
}

func assertDesktopAction(
	t testing.TB,
	action workspacesdk.DesktopAction,
	actionName string,
	coordinate [2]int,
) {
	t.Helper()
	assert.Equal(t, actionName, action.Action)
	require.NotNil(t, action.Coordinate)
	assert.Equal(t, coordinate, *action.Coordinate)
}

func assertTextAction(
	t testing.TB,
	action workspacesdk.DesktopAction,
	actionName string,
	text string,
) {
	t.Helper()
	assert.Equal(t, actionName, action.Action)
	require.NotNil(t, action.Text)
	assert.Equal(t, text, *action.Text)
}

func assertScrollAction(
	t testing.TB,
	action workspacesdk.DesktopAction,
	coordinate [2]int,
	direction string,
	amount int,
) {
	t.Helper()
	assertDesktopAction(t, action, "scroll", coordinate)
	require.NotNil(t, action.ScrollDirection)
	require.NotNil(t, action.ScrollAmount)
	assert.Equal(t, direction, *action.ScrollDirection)
	assert.Equal(t, amount, *action.ScrollAmount)
}
