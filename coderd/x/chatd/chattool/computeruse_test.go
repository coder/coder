package chattool_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"reflect"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
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

func newComputerUseToolForTest(
	t *testing.T,
	provider string,
	getConn func(context.Context) (workspacesdk.AgentConn, error),
	storeFile chattool.StoreFileFunc,
) fantasy.AgentTool {
	t.Helper()
	geometry := workspacesdk.DefaultDesktopGeometry()
	clock := quartz.NewReal()
	logger := slogtest.Make(t, nil)
	constructor := reflect.ValueOf(chattool.NewComputerUseTool)
	typ := constructor.Type()

	var args []reflect.Value
	switch typ.NumIn() {
	case 6:
		if provider != "anthropic" {
			t.Fatalf("NewComputerUseTool must accept a provider parameter to test %q", provider)
		}
		args = []reflect.Value{
			reflect.ValueOf(geometry.DeclaredWidth),
			reflect.ValueOf(geometry.DeclaredHeight),
			reflect.ValueOf(getConn),
			reflect.Zero(typ.In(3)),
			reflect.ValueOf(clock),
			reflect.ValueOf(logger),
		}
		if storeFile != nil {
			args[3] = reflect.ValueOf(storeFile)
		}
	case 7:
		args = []reflect.Value{
			reflect.ValueOf(provider),
			reflect.ValueOf(geometry.DeclaredWidth),
			reflect.ValueOf(geometry.DeclaredHeight),
			reflect.ValueOf(getConn),
			reflect.Zero(typ.In(4)),
			reflect.ValueOf(clock),
			reflect.ValueOf(logger),
		}
		if storeFile != nil {
			args[4] = reflect.ValueOf(storeFile)
		}
	default:
		t.Fatalf("unexpected NewComputerUseTool signature with %d parameters", typ.NumIn())
	}

	results := constructor.Call(args)
	tool, ok := results[0].Interface().(fantasy.AgentTool)
	require.True(t, ok, "NewComputerUseTool should return a fantasy.AgentTool")
	return tool
}

func computerUseProviderToolForTest(
	t *testing.T,
	provider string,
	declaredWidth int,
	declaredHeight int,
) fantasy.Tool {
	t.Helper()
	constructor := reflect.ValueOf(chattool.ComputerUseProviderTool)
	typ := constructor.Type()

	var args []reflect.Value
	switch typ.NumIn() {
	case 2:
		if provider != "anthropic" {
			t.Fatalf("ComputerUseProviderTool must accept a provider parameter to test %q", provider)
		}
		args = []reflect.Value{reflect.ValueOf(declaredWidth), reflect.ValueOf(declaredHeight)}
	case 3:
		args = []reflect.Value{reflect.ValueOf(provider), reflect.ValueOf(declaredWidth), reflect.ValueOf(declaredHeight)}
	default:
		t.Fatalf("unexpected ComputerUseProviderTool signature with %d parameters", typ.NumIn())
	}

	results := constructor.Call(args)
	tool, ok := results[0].Interface().(fantasy.Tool)
	require.True(t, ok, "ComputerUseProviderTool should return a fantasy.Tool")
	return tool
}

func requireDesktopActionEqual(
	t *testing.T,
	got workspacesdk.DesktopAction,
	want workspacesdk.DesktopAction,
	geometry workspacesdk.DesktopGeometry,
) {
	t.Helper()

	require.Equal(t, want.Action, got.Action)
	require.NotNil(t, got.ScaledWidth)
	require.NotNil(t, got.ScaledHeight)
	assert.Equal(t, geometry.DeclaredWidth, *got.ScaledWidth)
	assert.Equal(t, geometry.DeclaredHeight, *got.ScaledHeight)

	if want.Coordinate != nil {
		require.NotNil(t, got.Coordinate)
		assert.Equal(t, *want.Coordinate, *got.Coordinate)
	} else {
		assert.Nil(t, got.Coordinate)
	}
	if want.StartCoordinate != nil {
		require.NotNil(t, got.StartCoordinate)
		assert.Equal(t, *want.StartCoordinate, *got.StartCoordinate)
	} else {
		assert.Nil(t, got.StartCoordinate)
	}
	if want.Text != nil {
		require.NotNil(t, got.Text)
		assert.Equal(t, *want.Text, *got.Text)
	} else {
		assert.Nil(t, got.Text)
	}
	if want.Duration != nil {
		require.NotNil(t, got.Duration)
		assert.Equal(t, *want.Duration, *got.Duration)
	} else {
		assert.Nil(t, got.Duration)
	}
	if want.ScrollAmount != nil {
		require.NotNil(t, got.ScrollAmount)
		assert.Equal(t, *want.ScrollAmount, *got.ScrollAmount)
	} else {
		assert.Nil(t, got.ScrollAmount)
	}
	if want.ScrollDirection != nil {
		require.NotNil(t, got.ScrollDirection)
		assert.Equal(t, *want.ScrollDirection, *got.ScrollDirection)
	} else {
		assert.Nil(t, got.ScrollDirection)
	}
}

func expectDesktopActionSequence(
	t *testing.T,
	mockConn *agentconnmock.MockAgentConn,
	expected []workspacesdk.DesktopAction,
	screenshotData string,
) {
	t.Helper()

	geometry := workspacesdk.DefaultDesktopGeometry()
	for _, want := range expected {
		want := want
		mockConn.EXPECT().ExecuteDesktopAction(
			gomock.Any(),
			gomock.AssignableToTypeOf(workspacesdk.DesktopAction{}),
		).DoAndReturn(func(_ context.Context, got workspacesdk.DesktopAction) (workspacesdk.DesktopActionResponse, error) {
			requireDesktopActionEqual(t, got, want, geometry)
			if want.Action == "screenshot" {
				return workspacesdk.DesktopActionResponse{
					Output:           "screenshot",
					ScreenshotData:   screenshotData,
					ScreenshotWidth:  geometry.DeclaredWidth,
					ScreenshotHeight: geometry.DeclaredHeight,
				}, nil
			}
			return workspacesdk.DesktopActionResponse{Output: want.Action + " performed"}, nil
		})
	}
}

func intPtr(v int) *int { return &v }

func coordPtr(x, y int) *[2]int {
	coord := [2]int{x, y}
	return &coord
}

func strPtr(v string) *string { return &v }

func TestComputerUseProviderTool(t *testing.T) {
	t.Parallel()

	geometry := workspacesdk.DefaultDesktopGeometry()

	t.Run("Anthropic", func(t *testing.T) {
		t.Parallel()

		def := computerUseProviderToolForTest(t, "anthropic", geometry.DeclaredWidth, geometry.DeclaredHeight)
		pdt, ok := def.(fantasy.ProviderDefinedTool)
		require.True(t, ok, "ComputerUseProviderTool should return a ProviderDefinedTool")
		require.Equal(t, "anthropic.computer", pdt.ID)
		require.Equal(t, "computer", pdt.Name)
		assert.Equal(t, int64(geometry.DeclaredWidth), pdt.Args["display_width_px"])
		assert.Equal(t, int64(geometry.DeclaredHeight), pdt.Args["display_height_px"])
	})

	t.Run("OpenAI", func(t *testing.T) {
		t.Parallel()

		def := computerUseProviderToolForTest(t, "openai", geometry.DeclaredWidth, geometry.DeclaredHeight)
		pdt, ok := def.(fantasy.ProviderDefinedTool)
		require.True(t, ok, "ComputerUseProviderTool should return a ProviderDefinedTool")
		require.Equal(t, "openai.computer", pdt.ID)
		require.Equal(t, "computer", pdt.Name)
		assert.Equal(t, int64(geometry.DeclaredWidth), pdt.Args["display_width_px"])
		assert.Equal(t, int64(geometry.DeclaredHeight), pdt.Args["display_height_px"])
		environment, ok := pdt.Args["environment"]
		require.True(t, ok, "openai computer tool should declare an environment")
		assert.NotEmpty(t, environment)
	})
}

func TestComputerUseTool_Run_Screenshot_Anthropic(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	geometry := workspacesdk.DefaultDesktopGeometry()

	expectDesktopActionSequence(t, mockConn, []workspacesdk.DesktopAction{{
		Action: "screenshot",
	}}, "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg==")

	tool := newComputerUseToolForTest(t, "anthropic", func(_ context.Context) (workspacesdk.AgentConn, error) {
		return mockConn, nil
	}, nil)

	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "anthropic-screenshot",
		Name:  "computer",
		Input: `{"action":"screenshot"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "image", resp.Type)
	assert.Equal(t, "image/png", resp.MediaType)
	assert.Equal(t, []byte("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg=="), resp.Data)
	assert.False(t, resp.IsError)
	require.Equal(t, geometry.DeclaredWidth, geometry.DeclaredWidth)
}

func TestComputerUseTool_Run_Screenshot_PersistsAttachment_OpenAI(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	const screenshotPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg=="

	expectDesktopActionSequence(t, mockConn, []workspacesdk.DesktopAction{{
		Action: "screenshot",
	}}, screenshotPNG)

	var storedName string
	var storedType string
	var storedData []byte
	tool := newComputerUseToolForTest(t, "openai", func(_ context.Context) (workspacesdk.AgentConn, error) {
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
	})

	payload := []byte(`{"type":"screenshot"}`)
	parsed, err := fantasyopenai.ParseComputerUseInput(payload)
	require.NoError(t, err)
	require.NotNil(t, parsed.Action)

	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID: "openai-screenshot", Name: "computer", Input: string(payload),
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

func TestComputerUseTool_Run_OpenAIBatchedActions(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	followUpScreenshot := base64.StdEncoding.EncodeToString([]byte("after-openai-batch"))

	expectDesktopActionSequence(t, mockConn, []workspacesdk.DesktopAction{
		{Action: "mouse_move", Coordinate: coordPtr(10, 20)},
		{Action: "left_click", Coordinate: coordPtr(10, 20)},
		{Action: "screenshot"},
	}, followUpScreenshot)

	tool := newComputerUseToolForTest(t, "openai", func(_ context.Context) (workspacesdk.AgentConn, error) {
		return mockConn, nil
	}, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
		t.Fatal("storeFile should not be called for openai follow-up screenshots")
		return chattool.AttachmentMetadata{}, nil
	})

	payload := []byte(`[{"type":"move","x":10,"y":20},{"type":"click","button":"left","x":10,"y":20}]`)
	parsed, err := fantasyopenai.ParseComputerUseInput(payload)
	require.NoError(t, err)
	require.Len(t, parsed.Actions, 2)

	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID: "openai-batch", Name: "computer", Input: string(payload),
	})
	require.NoError(t, err)
	assert.Equal(t, "image", resp.Type)
	assert.Equal(t, []byte(followUpScreenshot), resp.Data)
	attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
	require.NoError(t, err)
	assert.Empty(t, attachments)
}

func TestComputerUseTool_Run_FollowUpScreenshotsDoNotPersistAttachments(t *testing.T) {
	t.Parallel()

	followUpScreenshot := base64.StdEncoding.EncodeToString([]byte("after-action"))
	tests := []struct {
		name     string
		provider string
		input    string
		expected []workspacesdk.DesktopAction
	}{
		{
			name:     "AnthropicClick",
			provider: "anthropic",
			input:    `{"action":"left_click","coordinate":[100,200]}`,
			expected: []workspacesdk.DesktopAction{
				{Action: "left_click", Coordinate: coordPtr(100, 200)},
				{Action: "screenshot"},
			},
		},
		{
			name:     "AnthropicWait",
			provider: "anthropic",
			input:    `{"action":"wait","duration":10}`,
			expected: []workspacesdk.DesktopAction{{Action: "screenshot"}},
		},
		{
			name:     "OpenAIClick",
			provider: "openai",
			input:    `{"type":"click","button":"left","x":100,"y":200}`,
			expected: []workspacesdk.DesktopAction{
				{Action: "left_click", Coordinate: coordPtr(100, 200)},
				{Action: "screenshot"},
			},
		},
		{
			name:     "OpenAIType",
			provider: "openai",
			input:    `{"type":"type","text":"hello"}`,
			expected: []workspacesdk.DesktopAction{
				{Action: "type", Text: strPtr("hello")},
				{Action: "screenshot"},
			},
		},
		{
			name:     "OpenAIScroll",
			provider: "openai",
			input:    `{"type":"scroll","x":10,"y":20,"scroll_x":0,"scroll_y":600}`,
			expected: []workspacesdk.DesktopAction{
				{Action: "scroll", Coordinate: coordPtr(10, 20), ScrollAmount: intPtr(600), ScrollDirection: strPtr("down")},
				{Action: "screenshot"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			expectDesktopActionSequence(t, mockConn, tt.expected, followUpScreenshot)

			tool := newComputerUseToolForTest(t, tt.provider, func(_ context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			}, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
				t.Fatal("storeFile should not be called for follow-up screenshots")
				return chattool.AttachmentMetadata{}, nil
			})

			resp, err := tool.Run(context.Background(), fantasy.ToolCall{
				ID: tt.name, Name: "computer", Input: tt.input,
			})
			require.NoError(t, err)
			assert.Equal(t, "image", resp.Type)
			assert.Equal(t, []byte(followUpScreenshot), resp.Data)
			attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
			require.NoError(t, err)
			assert.Empty(t, attachments)
		})
	}
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

	tool := newComputerUseToolForTest(t, "anthropic", func(_ context.Context) (workspacesdk.AgentConn, error) {
		return mockConn, nil
	}, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
		return chattool.AttachmentMetadata{}, xerrors.New("chat already has the maximum of 20 linked files")
	})

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

	tool := newComputerUseToolForTest(t, "anthropic", func(_ context.Context) (workspacesdk.AgentConn, error) {
		return mockConn, nil
	}, func(_ context.Context, _ string, _ string, _ []byte) (chattool.AttachmentMetadata, error) {
		t.Fatal("storeFile should not be called for oversized screenshots")
		return chattool.AttachmentMetadata{}, nil
	})

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

func TestComputerUseTool_Run_ConnError(t *testing.T) {
	t.Parallel()

	tool := newComputerUseToolForTest(t, "anthropic", func(_ context.Context) (workspacesdk.AgentConn, error) {
		return nil, xerrors.New("workspace not available")
	}, nil)

	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "test-conn-error",
		Name:  "computer",
		Input: `{"action":"screenshot"}`,
	})
	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "workspace not available")
}

func TestComputerUseTool_Run_InvalidInput_OpenAI(t *testing.T) {
	t.Parallel()

	tool := newComputerUseToolForTest(t, "openai", func(_ context.Context) (workspacesdk.AgentConn, error) {
		return nil, xerrors.New("should not be called")
	}, nil)

	payload := []byte(`{"type":"future_action"}`)
	_, err := fantasyopenai.ParseComputerUseInput(payload)
	require.Error(t, err)

	resp, runErr := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "test-openai-invalid-input",
		Name:  "computer",
		Input: string(payload),
	})
	require.NoError(t, runErr)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "invalid computer use input")
	assert.Contains(t, resp.Content, "future_action")
}
