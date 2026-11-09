package chattool_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestDefaultComputerUseDesktopGeometry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		provider       codersdk.ChatComputerUseProvider
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
			provider:       codersdk.ChatComputerUseProviderAnthropic,
			declaredWidth:  1280,
			declaredHeight: 720,
		},
		{
			name:           "OpenAI",
			provider:       codersdk.ChatComputerUseProviderOpenAI,
			declaredWidth:  1600,
			declaredHeight: 900,
		},
		{
			name:           "OpenAICompat",
			provider:       codersdk.ChatComputerUseProviderOpenAICompat,
			declaredWidth:  1280,
			declaredHeight: 720,
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

	tests := []struct {
		version fantasyanthropic.ComputerUseToolVersion
		// wantDisplay reports whether the definition carries the
		// declared display size; the toolset takes none.
		wantDisplay bool
		wantZoom    any
	}{
		{
			version:  fantasyanthropic.ComputerUseToolset20260801,
			wantZoom: false,
		},
		{
			version:     fantasyanthropic.ComputerUse20251124,
			wantDisplay: true,
		},
		{
			version:     fantasyanthropic.ComputerUse20250124,
			wantDisplay: true,
		},
	}
	for _, tt := range tests {
		t.Run(string(tt.version), func(t *testing.T) {
			t.Parallel()

			def, err := chattool.ComputerUseProviderTool(
				codersdk.ChatComputerUseProviderAnthropic,
				tt.version,
				geometry.DeclaredWidth,
				geometry.DeclaredHeight,
			)
			require.NoError(t, err)
			pdt, ok := def.(fantasy.ProviderDefinedTool)
			require.True(t, ok, "ComputerUseProviderTool should return a ProviderDefinedTool")
			assert.True(t, fantasyanthropic.IsComputerUseTool(def))
			assert.Equal(t, "computer", pdt.Name)
			assert.Equal(t, string(tt.version), pdt.Args["tool_version"])
			assert.Equal(t, tt.wantZoom, pdt.Args["enable_zoom"])
			wantWidth, wantHeight := int64(0), int64(0)
			if tt.wantDisplay {
				wantWidth, wantHeight = int64(geometry.DeclaredWidth), int64(geometry.DeclaredHeight)
			}
			assert.Equal(t, wantWidth, pdt.Args["display_width_px"])
			assert.Equal(t, wantHeight, pdt.Args["display_height_px"])
		})
	}

	t.Run("OpenAI", func(t *testing.T) {
		t.Parallel()

		openAITool, err := chattool.ComputerUseProviderTool(
			codersdk.ChatComputerUseProviderOpenAI,
			"",
			geometry.DeclaredWidth,
			geometry.DeclaredHeight,
		)
		require.NoError(t, err)
		assert.True(t, fantasyopenai.IsComputerUseTool(openAITool))
	})

	t.Run("OpenAICompat", func(t *testing.T) {
		t.Parallel()

		def, err := chattool.ComputerUseProviderTool(
			codersdk.ChatComputerUseProviderOpenAICompat,
			"",
			geometry.DeclaredWidth,
			geometry.DeclaredHeight,
		)
		require.NoError(t, err)
		// The chat completions API has no hosted computer tool, so this
		// flavor is an ordinary function tool.
		funcTool, ok := def.(fantasy.FunctionTool)
		require.True(t, ok, "got %T, want a function tool", def)
		assert.Equal(t, "computer", funcTool.Name)
		assert.Contains(t, funcTool.Description,
			fmt.Sprintf("%d by %d pixel", geometry.DeclaredWidth, geometry.DeclaredHeight))

		properties, ok := funcTool.InputSchema["properties"].(map[string]any)
		require.True(t, ok)
		action, ok := properties["action"].(map[string]any)
		require.True(t, ok)
		enum, ok := action["enum"].([]string)
		require.True(t, ok, "the action list must be an enum, got %T", action["enum"])
		assert.Contains(t, enum, "screenshot")
		assert.Contains(t, enum, "left_click_drag")
		// The agent desktop has no zoom action.
		assert.NotContains(t, enum, "zoom")
		// A failed call halts the rest of the step, which would skip a
		// release planned after a press.
		assert.NotContains(t, enum, "left_mouse_down")
		assert.NotContains(t, enum, "left_mouse_up")
		// Every function call answers with a screenshot, which cannot carry
		// the pointer position.
		assert.NotContains(t, enum, "cursor_position")
		assert.Equal(t, []string{"action"}, funcTool.InputSchema["required"])
	})

	t.Run("Unsupported", func(t *testing.T) {
		t.Parallel()

		_, err := chattool.ComputerUseProviderTool(
			codersdk.ChatComputerUseProvider("unsupported"),
			"",
			geometry.DeclaredWidth,
			geometry.DeclaredHeight,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported computer use provider")
	})
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

	tool := chattool.NewComputerUseTool(codersdk.ChatComputerUseProviderAnthropic, fantasyanthropic.ComputerUse20251124, geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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
	tool := chattool.NewComputerUseTool(codersdk.ChatComputerUseProviderAnthropic, fantasyanthropic.ComputerUse20251124, geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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

	tool := chattool.NewComputerUseTool(codersdk.ChatComputerUseProviderAnthropic, fantasyanthropic.ComputerUse20251124, geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
		return mockConn, nil
	}, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
		return chattool.AttachmentMetadata{}, xerrors.New("ETOOMANYFILES")
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

	tool := chattool.NewComputerUseTool(codersdk.ChatComputerUseProviderAnthropic, fantasyanthropic.ComputerUse20251124, geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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

	tool := chattool.NewComputerUseTool(codersdk.ChatComputerUseProviderAnthropic, fantasyanthropic.ComputerUse20251124, geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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

// The function tool answers every action with a screenshot, and only a
// requested screenshot is stored as a chat attachment.
func TestComputerUseTool_Run_OpenAICompat_ScreenshotsAfterEveryAction(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()
	const screenshotPNG = "aW1hZ2UtZGF0YQ=="
	// The click, the screenshot that follows it, and the requested screenshot.
	actions := recordDesktopActions(t, mockConn, geometry, 3, screenshotPNG)

	var storedNames []string
	tool := chattool.NewComputerUseTool(
		codersdk.ChatComputerUseProviderOpenAICompat,
		"",
		geometry.DeclaredWidth,
		geometry.DeclaredHeight,
		func(_ context.Context) (workspacesdk.AgentConn, error) {
			return mockConn, nil
		},
		func(_ context.Context, name string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
			storedNames = append(storedNames, name)
			return chattool.AttachmentMetadata{
				FileID:    uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
				MediaType: "image/png",
				Name:      name,
			}, nil
		},
		quartz.NewReal(),
		slogtest.Make(t, nil),
	)

	ctx := context.Background()
	actionResp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "compat-left-click",
		Name:  "computer",
		Input: `{"action":"left_click","coordinate":[100,200]}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "image", actionResp.Type)
	assert.False(t, actionResp.IsError)
	expectedImage, err := base64.StdEncoding.DecodeString(screenshotPNG)
	require.NoError(t, err)
	assert.Equal(t, expectedImage, actionResp.Data)
	actionAttachments, err := chattool.AttachmentsFromMetadata(actionResp.Metadata)
	require.NoError(t, err)
	assert.Empty(t, actionAttachments, "the screenshot after an action is for the model only")

	requestedResp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "compat-screenshot",
		Name:  "computer",
		Input: `{"action":"screenshot"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "image", requestedResp.Type)
	requestedAttachments, err := chattool.AttachmentsFromMetadata(requestedResp.Metadata)
	require.NoError(t, err)
	require.Len(t, requestedAttachments, 1)

	require.Len(t, *actions, 3)
	assertDesktopActionScaled(t, geometry, (*actions)[0])
	assertDesktopAction(t, (*actions)[0], "left_click", [2]int{100, 200})
	assert.Equal(t, "screenshot", (*actions)[1].Action)
	assert.Equal(t, "screenshot", (*actions)[2].Action)
	require.Len(t, storedNames, 1)
	assert.Contains(t, storedNames[0], "screenshot-")
}

func TestComputerUseTool_Run_Wait(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()
	mClock := quartz.NewMock(t)
	followUpScreenshot := base64.StdEncoding.EncodeToString([]byte("after-wait"))

	mockConn.EXPECT().ExecuteDesktopAction(
		gomock.Any(),
		gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
	).DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
		assert.Equal(t, "screenshot", action.Action)
		assertDesktopActionScaled(t, geometry, action)
		return workspacesdk.DesktopActionResponse{
			Output:           "screenshot",
			ScreenshotData:   followUpScreenshot,
			ScreenshotWidth:  geometry.DeclaredWidth,
			ScreenshotHeight: geometry.DeclaredHeight,
		}, nil
	})

	trap := mClock.Trap().NewTimer("computeruse", "wait")
	defer trap.Close()
	tool := chattool.NewComputerUseTool(codersdk.ChatComputerUseProviderAnthropic, fantasyanthropic.ComputerUse20251124, geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
		return mockConn, nil
	}, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
		t.Fatal("storeFile should not be called for wait screenshots")
		return chattool.AttachmentMetadata{}, nil
	}, mClock, slogtest.Make(t, nil))

	type toolResult struct {
		resp fantasy.ToolResponse
		err  error
	}
	resultCh := make(chan toolResult, 1)
	go func() {
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "test-3",
			Name:  "computer",
			Input: `{"action":"wait","duration":10}`,
		})
		resultCh <- toolResult{resp: resp, err: err}
	}()

	// Anthropic wait durations are in seconds.
	call := trap.MustWait(ctx)
	assert.Equal(t, 10*time.Second, call.Duration)
	call.MustRelease(ctx)
	mClock.Advance(10 * time.Second).MustWait(ctx)

	result := testutil.RequireReceive(ctx, t, resultCh)
	require.NoError(t, result.err)
	resp := result.resp
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

func TestComputerUseTool_Run_AnthropicHoldKeyDurationSeconds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		duration     int64
		wantDuration int
	}{
		{name: "Seconds", duration: 2, wantDuration: 2000},
		{name: "CappedAt300Seconds", duration: 1000, wantDuration: 300_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			gomock.InOrder(
				mockConn.EXPECT().ExecuteDesktopAction(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
						assert.Equal(t, "hold_key", action.Action)
						require.NotNil(t, action.Duration)
						// The agent desktop takes milliseconds.
						assert.Equal(t, tt.wantDuration, *action.Duration)
						return workspacesdk.DesktopActionResponse{Output: "hold_key action performed"}, nil
					}),
				expectScreenshot(mockConn),
			)

			tool := newAnthropicComputerUseTool(t, fantasyanthropic.ComputerUse20251124, mockConn)
			resp, err := tool.Run(context.Background(), anthropicComputerUseCall(
				fmt.Sprintf(`{"action":"hold_key","text":"shift","duration":%d}`, tt.duration),
			))
			require.NoError(t, err)
			assert.False(t, resp.IsError, resp.Content)
		})
	}
}

// The computer_toolset_20260801 toolset answers actions other than screenshot
// with text, and the model asks for a screenshot when it needs one. Older
// versions screenshot after every action.
func TestComputerUseTool_Run_AnthropicStepResults(t *testing.T) {
	t.Parallel()

	t.Run("Toolset", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		gomock.InOrder(
			expectActionOutput(mockConn, "left_click", "left_click action performed"),
			expectActionOutput(mockConn, "type", ""),
			expectActionOutput(mockConn, "cursor_position", "x=10,y=20"),
		)

		tool := newAnthropicComputerUseTool(t, fantasyanthropic.ComputerUseToolset20260801, mockConn)
		for _, tc := range []struct{ input, want string }{
			{input: `{"action":"left_click","coordinate":[100,200]}`, want: "left_click action performed"},
			{input: `{"action":"type","text":"hello"}`, want: "type action performed"},
			{input: `{"action":"cursor_position"}`, want: "x=10,y=20"},
		} {
			resp, err := tool.Run(context.Background(), anthropicComputerUseCall(tc.input))
			require.NoError(t, err)
			assert.Equal(t, fantasy.NewTextResponse(tc.want), resp, tc.input)
		}
	})

	t.Run("ToolsetWait", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		// The screenshot is answered so a regression fails on the result
		// instead of stalling the tool goroutine.
		expectScreenshot(mockConn).Do(func(context.Context, workspacesdk.DesktopAction) {
			t.Error("toolset wait should not take a screenshot")
		}).AnyTimes()
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTimer("computeruse", "wait")
		defer trap.Close()
		geometry := workspacesdk.DefaultDesktopGeometry()
		tool := chattool.NewComputerUseTool(
			codersdk.ChatComputerUseProviderAnthropic,
			fantasyanthropic.ComputerUseToolset20260801,
			geometry.DeclaredWidth,
			geometry.DeclaredHeight,
			func(_ context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			nil,
			mClock,
			slogtest.Make(t, nil),
		)

		respCh := make(chan fantasy.ToolResponse, 1)
		go func() {
			resp, err := tool.Run(ctx, anthropicComputerUseCall(`{"action":"wait","duration":1}`))
			assert.NoError(t, err)
			respCh <- resp
		}()
		trap.MustWait(ctx).MustRelease(ctx)
		mClock.Advance(time.Second).MustWait(ctx)
		assert.Equal(t, fantasy.NewTextResponse("wait action performed"), testutil.RequireReceive(ctx, t, respCh))
	})

	t.Run("Computer20251124", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		gomock.InOrder(
			expectActionOutput(mockConn, "left_click", "left_click action performed"),
			expectScreenshot(mockConn),
			expectActionOutput(mockConn, "type", "type action performed"),
			expectScreenshot(mockConn),
		)

		tool := newAnthropicComputerUseTool(t, fantasyanthropic.ComputerUse20251124, mockConn)
		for _, input := range []string{
			`{"action":"left_click","coordinate":[100,200]}`,
			`{"action":"type","text":"hello"}`,
		} {
			resp, err := tool.Run(context.Background(), anthropicComputerUseCall(input))
			require.NoError(t, err)
			assert.Equal(t, "image", resp.Type, input)
		}
	})
}

// The model plans a step's Anthropic computer calls against one screen, so
// after a call fails or is rejected before it runs, the later computer calls
// of the step must not touch the desktop.
func TestComputerUseTool_AnthropicHaltsStep(t *testing.T) {
	t.Parallel()

	const haltedText = "Not executed: an earlier computer action in this turn failed."
	rejected := fantasy.ToolCallContent{ToolCallID: "toolu_rejected", ToolName: "computer", Input: `{"action":"left_click",`}
	key := fantasy.ToolCallContent{ToolCallID: "toolu_key", ToolName: "computer", Input: `{"action":"key","text":"NotAKey"}`}
	click := fantasy.ToolCallContent{ToolCallID: "toolu_click", ToolName: "computer", Input: `{"action":"left_click","coordinate":[100,200]}`}
	typeText := fantasy.ToolCallContent{ToolCallID: "toolu_type", ToolName: "computer", Input: `{"action":"type","text":"hello"}`}
	screenshot := fantasy.ToolCallContent{ToolCallID: "toolu_screenshot", ToolName: "computer", Input: `{"action":"screenshot"}`}

	// runStep runs the executable calls as chatd does after it rejects the
	// other observed calls.
	runStep := func(t *testing.T, conn workspacesdk.AgentConn, observed, executed []fantasy.ToolCallContent) []fantasy.ToolResultContent {
		t.Helper()
		outcome, err := chatloop.ExecuteLocalTools(context.Background(), chatloop.ExecuteLocalToolsOptions{
			ProviderTools:     []chatloop.ProviderTool{{Runner: newAnthropicComputerUseTool(t, fantasyanthropic.ComputerUseToolset20260801, conn)}},
			ToolCalls:         executed,
			ObservedToolCalls: observed,
			Clock:             quartz.NewReal(),
		})
		require.NoError(t, err)
		var results []fantasy.ToolResultContent
		for _, content := range outcome.Content {
			if result, ok := content.(fantasy.ToolResultContent); ok {
				results = append(results, result)
			}
		}
		require.Len(t, results, len(executed))
		return results
	}
	requireHalted := func(t *testing.T, result fantasy.ToolResultContent) {
		t.Helper()
		halted, ok := result.Result.(fantasy.ToolResultOutputContentError)
		require.True(t, ok, "%T", result.Result)
		require.EqualError(t, halted.Error, haltedText, result.ToolCallID)
	}

	t.Run("AfterFailure", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().ExecuteDesktopAction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
				assert.Equal(t, "key", action.Action)
				return workspacesdk.DesktopActionResponse{}, xerrors.New("unknown key")
			})

		calls := []fantasy.ToolCallContent{key, typeText, screenshot}
		results := runStep(t, mockConn, calls, calls)
		failed, ok := results[0].Result.(fantasy.ToolResultOutputContentError)
		require.True(t, ok, "%T", results[0].Result)
		require.ErrorContains(t, failed.Error, "unknown key")
		for _, result := range results[1:] {
			requireHalted(t, result)
		}
	})

	t.Run("AfterRejectedCall", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		results := runStep(t, mockConn, []fantasy.ToolCallContent{rejected, typeText}, []fantasy.ToolCallContent{typeText})
		requireHalted(t, results[0])
	})

	t.Run("EarlierCallsRun", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		expectActionOutput(mockConn, "left_click", "left_click action performed")

		results := runStep(t, mockConn, []fantasy.ToolCallContent{click, rejected}, []fantasy.ToolCallContent{click})
		require.Equal(t, fantasy.ToolResultOutputContentText{Text: "left_click action performed"}, results[0].Result)
	})
}

// A coordinate of [0,0] is the top-left pixel and a valid target, so the
// runner must send it rather than read it as an omitted field.
func TestComputerUseTool_Run_TopLeftCoordinate(t *testing.T) {
	t.Parallel()
	const screenshotPNG = "aW1hZ2UtZGF0YQ=="

	for _, tc := range []struct {
		name     string
		provider codersdk.ChatComputerUseProvider
		version  fantasyanthropic.ComputerUseToolVersion
	}{
		{"Anthropic", codersdk.ChatComputerUseProviderAnthropic, fantasyanthropic.ComputerUse20251124},
		{"OpenAICompat", codersdk.ChatComputerUseProviderOpenAICompat, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			geometry := workspacesdk.DefaultDesktopGeometry()
			actions := recordDesktopActions(t, mockConn, geometry, 2, screenshotPNG)

			tool := chattool.NewComputerUseTool(
				tc.provider,
				tc.version,
				geometry.DeclaredWidth,
				geometry.DeclaredHeight,
				func(_ context.Context) (workspacesdk.AgentConn, error) { return mockConn, nil },
				nil,
				quartz.NewReal(),
				slogtest.Make(t, nil),
			)

			_, err := tool.Run(context.Background(), fantasy.ToolCall{
				ID:    "top-left-click",
				Name:  "computer",
				Input: `{"action":"left_click","coordinate":[0,0]}`,
			})
			require.NoError(t, err)
			require.NotEmpty(t, *actions)
			assertDesktopAction(t, (*actions)[0], "left_click", [2]int{0, 0})
		})
	}
}

// An omitted or null coordinate stays omitted, so the agent still reports the
// missing field instead of clicking the top-left pixel. Chat completions
// models often send null for an optional field.
func TestComputerUseTool_Run_OmittedCoordinateIsNotTopLeft(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		`{"action":"left_click"}`,
		`{"action":"left_click","coordinate":null,"start_coordinate":null,"scroll_amount":null,"repeat":null}`,
	} {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			mockConn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
			// The click and the screenshot that follows it.
			actions := recordDesktopActions(t, mockConn, workspacesdk.DefaultDesktopGeometry(), 2, "aW1hZ2UtZGF0YQ==")
			resp, err := newOpenAICompatComputerUseTool(t, mockConn).Run(context.Background(), fantasy.ToolCall{
				ID:    "click-without-coordinate",
				Name:  "computer",
				Input: input,
			})
			require.NoError(t, err)
			require.False(t, resp.IsError, resp.Content)
			require.NotEmpty(t, *actions)
			assert.Nil(t, (*actions)[0].Coordinate)
			assert.Nil(t, (*actions)[0].StartCoordinate)
			assert.Nil(t, (*actions)[0].ScrollAmount)
		})
	}
}

// Coordinates are forwarded by key presence, so a short or long array must be
// rejected before it decodes zero-filled into a click at the top-left pixel.
func TestComputerUseTool_Run_RejectsMalformedCoordinate(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		`{"action":"left_click","coordinate":[]}`,
		`{"action":"left_click","coordinate":[5]}`,
		`{"action":"left_click","coordinate":[1,2,3]}`,
		`{"action":"left_click_drag","start_coordinate":[5],"coordinate":[10,10]}`,
	} {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			// No expectations: any desktop call fails the test.
			mockConn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
			resp, err := newOpenAICompatComputerUseTool(t, mockConn).Run(context.Background(), fantasy.ToolCall{
				ID:    "malformed-coordinate",
				Name:  "computer",
				Input: input,
			})
			require.NoError(t, err)
			assert.True(t, resp.IsError)
			assert.Contains(t, resp.Content, "expected 2 elements")
		})
	}
}

// The desktop clamps an off-screen point to the screen edge, so a model that
// aims off the screenshot learns of the miss instead of acting on the edge.
func TestComputerUseTool_Run_RejectsOffScreenCoordinate(t *testing.T) {
	t.Parallel()

	geometry := workspacesdk.DefaultDesktopGeometry()
	right, bottom := geometry.DeclaredWidth, geometry.DeclaredHeight
	for name, newTool := range actionPerCallComputerUseTools() {
		for _, tc := range []struct {
			input string
			want  string
		}{
			{fmt.Sprintf(`{"action":"left_click","coordinate":[%d,10]}`, right), fmt.Sprintf("coordinate [%d, 10] is outside", right)},
			{fmt.Sprintf(`{"action":"mouse_move","coordinate":[10,%d]}`, bottom), fmt.Sprintf("coordinate [10, %d] is outside", bottom)},
			{`{"action":"scroll","coordinate":[-1,10],"scroll_direction":"down","scroll_amount":1}`, "coordinate [-1, 10] is outside"},
			{`{"action":"left_click_drag","start_coordinate":[10,-5],"coordinate":[20,20]}`, "start_coordinate [10, -5] is outside"},
		} {
			t.Run(name+"/"+tc.input, func(t *testing.T) {
				t.Parallel()

				// No expectations: any desktop call fails the test.
				mockConn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
				resp, err := newTool(t, mockConn).Run(context.Background(), fantasy.ToolCall{
					ID:    "off-screen-coordinate",
					Name:  "computer",
					Input: tc.input,
				})
				require.NoError(t, err)
				assert.True(t, resp.IsError)
				assert.Contains(t, resp.Content, tc.want)
			})
		}

		t.Run(name+"/BottomRightPixel", func(t *testing.T) {
			t.Parallel()

			var actions []workspacesdk.DesktopAction
			mockConn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
			mockConn.EXPECT().ExecuteDesktopAction(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
					actions = append(actions, action)
					return workspacesdk.DesktopActionResponse{}, nil
				}).MinTimes(1)
			resp, err := newTool(t, mockConn).Run(context.Background(), fantasy.ToolCall{
				ID:    "bottom-right-click",
				Name:  "computer",
				Input: fmt.Sprintf(`{"action":"left_click","coordinate":[%d,%d]}`, right-1, bottom-1),
			})
			require.NoError(t, err)
			require.False(t, resp.IsError, resp.Content)
			assertDesktopAction(t, actions[0], "left_click", [2]int{right - 1, bottom - 1})
		})
	}
}

// A function tool step plans every call of the step against one screen, so a
// failed action halts the later calls of that step, as for Anthropic.
func TestComputerUseTool_OpenAICompatHaltsStep(t *testing.T) {
	t.Parallel()

	const haltedText = "Not executed: an earlier computer action in this turn failed."
	key := fantasy.ToolCallContent{ToolCallID: "compat_key", ToolName: "computer", Input: `{"action":"key","text":"NotAKey"}`}
	typeText := fantasy.ToolCallContent{ToolCallID: "compat_type", ToolName: "computer", Input: `{"action":"type","text":"hello"}`}
	rejected := fantasy.ToolCallContent{ToolCallID: "compat_rejected", ToolName: "computer", Input: `{"action":"left_click",`}

	runStep := func(t *testing.T, conn workspacesdk.AgentConn, observed, executed []fantasy.ToolCallContent) []fantasy.ToolResultContent {
		t.Helper()
		outcome, err := chatloop.ExecuteLocalTools(context.Background(), chatloop.ExecuteLocalToolsOptions{
			ProviderTools:     []chatloop.ProviderTool{{Runner: newOpenAICompatComputerUseTool(t, conn)}},
			ToolCalls:         executed,
			ObservedToolCalls: observed,
			Clock:             quartz.NewReal(),
		})
		require.NoError(t, err)
		var results []fantasy.ToolResultContent
		for _, content := range outcome.Content {
			if result, ok := content.(fantasy.ToolResultContent); ok {
				results = append(results, result)
			}
		}
		require.Len(t, results, len(executed))
		return results
	}

	t.Run("AfterFailure", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		// Only the failing action may reach the desktop.
		mockConn.EXPECT().ExecuteDesktopAction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
				assert.Equal(t, "key", action.Action)
				return workspacesdk.DesktopActionResponse{}, xerrors.New("unknown key")
			}).Times(1)

		calls := []fantasy.ToolCallContent{key, typeText}
		results := runStep(t, mockConn, calls, calls)
		failed, ok := results[0].Result.(fantasy.ToolResultOutputContentError)
		require.True(t, ok, "%T", results[0].Result)
		require.ErrorContains(t, failed.Error, "unknown key")
		halted, ok := results[1].Result.(fantasy.ToolResultOutputContentError)
		require.True(t, ok, "%T", results[1].Result)
		require.EqualError(t, halted.Error, haltedText)
	})

	t.Run("AfterRejectedCall", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)

		results := runStep(t, mockConn, []fantasy.ToolCallContent{rejected, typeText}, []fantasy.ToolCallContent{typeText})
		halted, ok := results[0].Result.(fantasy.ToolResultOutputContentError)
		require.True(t, ok, "%T", results[0].Result)
		require.EqualError(t, halted.Error, haltedText)
	})
}

// The desktop also performs actions that a flavor does not declare, such as
// key_down, which would leave a key held, so the runner rejects them before
// the desktop.
func TestComputerUseTool_Run_RejectsUnsupportedAction(t *testing.T) {
	t.Parallel()

	undeclared := map[string][]string{
		"Anthropic":    {"key_down", "zoom", ""},
		"OpenAICompat": {"key_down", "zoom", "", "left_mouse_down", "left_mouse_up", "cursor_position"},
	}
	for name, newTool := range actionPerCallComputerUseTools() {
		for _, action := range undeclared[name] {
			t.Run(name+"/"+action, func(t *testing.T) {
				t.Parallel()

				// No expectations: any desktop call fails the test.
				mockConn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
				resp, err := newTool(t, mockConn).Run(context.Background(), fantasy.ToolCall{
					ID:    "unsupported-action",
					Name:  "computer",
					Input: fmt.Sprintf(`{"action":%q,"text":"shift"}`, action),
				})
				require.NoError(t, err)
				assert.True(t, resp.IsError)
				assert.Contains(t, resp.Content, "unsupported action")
			})
		}
	}
}

// actionPerCallComputerUseTools returns constructors for the flavors that
// declare one action per call.
func actionPerCallComputerUseTools() map[string]func(testing.TB, workspacesdk.AgentConn) fantasy.AgentTool {
	return map[string]func(testing.TB, workspacesdk.AgentConn) fantasy.AgentTool{
		"Anthropic": func(t testing.TB, conn workspacesdk.AgentConn) fantasy.AgentTool {
			return newAnthropicComputerUseTool(t, fantasyanthropic.ComputerUseToolset20260801, conn)
		},
		"OpenAICompat": newOpenAICompatComputerUseTool,
	}
}

// The agent scrolls a default amount when scroll_amount is absent, so a zero or
// negative amount must not reach it looking absent.
func TestComputerUseTool_Run_ScrollAmount(t *testing.T) {
	t.Parallel()
	const screenshotPNG = "aW1hZ2UtZGF0YQ=="

	for _, tc := range []struct {
		name     string
		provider codersdk.ChatComputerUseProvider
		version  fantasyanthropic.ComputerUseToolVersion
	}{
		{"Anthropic", codersdk.ChatComputerUseProviderAnthropic, fantasyanthropic.ComputerUse20251124},
		{"OpenAICompat", codersdk.ChatComputerUseProviderOpenAICompat, ""},
	} {
		newTool := func(t *testing.T, conn workspacesdk.AgentConn) fantasy.AgentTool {
			geometry := workspacesdk.DefaultDesktopGeometry()
			return chattool.NewComputerUseTool(
				tc.provider,
				tc.version,
				geometry.DeclaredWidth,
				geometry.DeclaredHeight,
				func(_ context.Context) (workspacesdk.AgentConn, error) { return conn, nil },
				nil,
				quartz.NewReal(),
				slogtest.Make(t, nil),
			)
		}
		scroll := func(amount int) fantasy.ToolCall {
			return fantasy.ToolCall{
				ID:    "scroll",
				Name:  "computer",
				Input: fmt.Sprintf(`{"action":"scroll","coordinate":[10,20],"scroll_direction":"down","scroll_amount":%d}`, amount),
			}
		}

		t.Run(tc.name+"/RejectsNonpositive", func(t *testing.T) {
			t.Parallel()

			for _, amount := range []int{0, -2} {
				// No expectations: any desktop call fails the test.
				mockConn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
				resp, err := newTool(t, mockConn).Run(context.Background(), scroll(amount))
				require.NoError(t, err)
				assert.True(t, resp.IsError, "scroll_amount %d", amount)
				assert.Contains(t, resp.Content, "scroll_amount must be at least 1")
			}
		})

		t.Run(tc.name+"/ForwardsPositive", func(t *testing.T) {
			t.Parallel()

			mockConn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
			// The scroll and the screenshot that follows it.
			actions := recordDesktopActions(t, mockConn, workspacesdk.DefaultDesktopGeometry(), 2, screenshotPNG)
			resp, err := newTool(t, mockConn).Run(context.Background(), scroll(4))
			require.NoError(t, err)
			require.False(t, resp.IsError, resp.Content)
			assertScrollAction(t, (*actions)[0], [2]int{10, 20}, "down", 4)
		})
	}
}

func newOpenAICompatComputerUseTool(t testing.TB, conn workspacesdk.AgentConn) fantasy.AgentTool {
	t.Helper()
	geometry := workspacesdk.DefaultDesktopGeometry()
	return chattool.NewComputerUseTool(
		codersdk.ChatComputerUseProviderOpenAICompat,
		"",
		geometry.DeclaredWidth,
		geometry.DeclaredHeight,
		func(_ context.Context) (workspacesdk.AgentConn, error) {
			return conn, nil
		},
		nil,
		quartz.NewReal(),
		slogtest.Make(t, nil),
	)
}

func TestComputerUseTool_Run_AnthropicKeyRepeat(t *testing.T) {
	t.Parallel()

	t.Run("PressesRepeatTimes", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().ExecuteDesktopAction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, action workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
				assert.Equal(t, "key", action.Action)
				require.NotNil(t, action.Text)
				assert.Equal(t, "Down", *action.Text)
				return workspacesdk.DesktopActionResponse{Output: "key action performed"}, nil
			}).Times(3)

		tool := newAnthropicComputerUseTool(t, fantasyanthropic.ComputerUseToolset20260801, mockConn)
		resp, err := tool.Run(context.Background(), anthropicComputerUseCall(`{"action":"key","text":"Down","repeat":3}`))
		require.NoError(t, err)
		assert.Equal(t, fantasy.NewTextResponse("key action performed"), resp)
	})

	t.Run("RejectsOutOfRange", func(t *testing.T) {
		t.Parallel()

		// Zero must not read as an omitted repeat and press the key once.
		for _, repeat := range []int{0, 101} {
			for name, newTool := range actionPerCallComputerUseTools() {
				// No expectations: any desktop call fails the test.
				mockConn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
				resp, err := newTool(t, mockConn).Run(context.Background(),
					anthropicComputerUseCall(fmt.Sprintf(`{"action":"key","text":"Down","repeat":%d}`, repeat)))
				require.NoError(t, err)
				assert.True(t, resp.IsError, "%s repeat %d", name, repeat)
				assert.Contains(t, resp.Content, "repeat must be between 1 and 100")
			}
		}
	})
}

func newAnthropicComputerUseTool(t testing.TB, version fantasyanthropic.ComputerUseToolVersion, conn workspacesdk.AgentConn) fantasy.AgentTool {
	t.Helper()
	geometry := workspacesdk.DefaultDesktopGeometry()
	return chattool.NewComputerUseTool(
		codersdk.ChatComputerUseProviderAnthropic,
		version,
		geometry.DeclaredWidth,
		geometry.DeclaredHeight,
		func(_ context.Context) (workspacesdk.AgentConn, error) {
			return conn, nil
		},
		nil,
		quartz.NewReal(),
		slogtest.Make(t, nil),
	)
}

func anthropicComputerUseCall(input string) fantasy.ToolCall {
	return fantasy.ToolCall{ID: "anthropic-call", Name: "computer", Input: input}
}

// expectDesktopAction expects one desktop action and answers it with resp.
// A different action fails the call.
func expectDesktopAction(conn *agentconnmock.MockAgentConn, action string, resp workspacesdk.DesktopActionResponse) *gomock.Call {
	return conn.EXPECT().ExecuteDesktopAction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, got workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
			if got.Action != action {
				return workspacesdk.DesktopActionResponse{}, xerrors.Errorf("got action %q, want %q", got.Action, action)
			}
			return resp, nil
		})
}

func expectActionOutput(conn *agentconnmock.MockAgentConn, action, output string) *gomock.Call {
	return expectDesktopAction(conn, action, workspacesdk.DesktopActionResponse{Output: output})
}

func expectScreenshot(conn *agentconnmock.MockAgentConn) *gomock.Call {
	return expectDesktopAction(conn, "screenshot", workspacesdk.DesktopActionResponse{
		Output:         "screenshot",
		ScreenshotData: base64.StdEncoding.EncodeToString([]byte("screenshot")),
	})
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
		codersdk.ChatComputerUseProviderAnthropic,
		fantasyanthropic.ComputerUse20251124,
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
	tool := chattool.NewComputerUseTool(codersdk.ChatComputerUseProviderAnthropic, fantasyanthropic.ComputerUse20251124, geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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
	tool := chattool.NewComputerUseTool(codersdk.ChatComputerUseProviderAnthropic, fantasyanthropic.ComputerUse20251124, geometry.DeclaredWidth, geometry.DeclaredHeight, func(_ context.Context) (workspacesdk.AgentConn, error) {
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
		return chattool.AttachmentMetadata{}, xerrors.New("ETOOMANYFILES")
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
		codersdk.ChatComputerUseProviderOpenAI,
		"",
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
