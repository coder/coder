package chattool

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

var SupportedComputerUseProviders = []string{"anthropic", "openai"}

func SupportsComputerUse(provider string) bool {
	_, ok := DefaultComputerUseModel(provider)
	return ok
}

func DefaultComputerUseModel(provider string) (string, bool) {
	switch normalizeComputerUseProvider(provider) {
	case "anthropic":
		return "claude-opus-4-6", true
	case "openai":
		return "computer-use-preview-2025-03-11", true
	default:
		return "", false
	}
}

// computerUseTool implements fantasy.AgentTool and
// chatloop.ToolDefiner for provider-aware computer use.
type computerUseTool struct {
	provider         string
	declaredWidth    int
	declaredHeight   int
	getWorkspaceConn func(ctx context.Context) (workspacesdk.AgentConn, error)
	storeFile        StoreFileFunc
	providerOptions  fantasy.ProviderOptions
	clock            quartz.Clock
	logger           slog.Logger
}

// NewComputerUseTool creates a computer-use AgentTool that delegates to the
// agent's desktop endpoints. declaredWidth and declaredHeight are the
// model-facing desktop dimensions advertised to the provider and requested for
// screenshots.
func NewComputerUseTool(
	provider string,
	declaredWidth, declaredHeight int,
	getWorkspaceConn func(ctx context.Context) (workspacesdk.AgentConn, error),
	storeFile StoreFileFunc,
	clock quartz.Clock,
	logger slog.Logger,
) fantasy.AgentTool {
	return &computerUseTool{
		provider:         normalizeComputerUseProvider(provider),
		declaredWidth:    declaredWidth,
		declaredHeight:   declaredHeight,
		getWorkspaceConn: getWorkspaceConn,
		storeFile:        storeFile,
		clock:            clock,
		logger:           logger,
	}
}

func (*computerUseTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{
		Name: "computer",
		Description: "Control the desktop: take screenshots, move the mouse, click, type, and scroll. " +
			"Use an explicit screenshot action when you want to share a screenshot with the user; " +
			"those screenshots are also attached to the chat.",
		Parameters: map[string]any{},
		Required:   []string{},
	}
}

// ComputerUseProviderTool creates the provider-defined computer-use tool
// definition using the declared model-facing desktop geometry.
func ComputerUseProviderTool(provider string, declaredWidth, declaredHeight int) fantasy.Tool {
	switch normalizeComputerUseProvider(provider) {
	case "anthropic":
		return fantasyanthropic.NewComputerUseTool(
			fantasyanthropic.ComputerUseToolOptions{
				DisplayWidthPx:  int64(declaredWidth),
				DisplayHeightPx: int64(declaredHeight),
				ToolVersion:     fantasyanthropic.ComputerUse20251124,
			},
			nil,
		).Definition()
	case "openai":
		return fantasyopenai.NewComputerUseTool(
			fantasyopenai.ComputerUseToolOptions{
				DisplayWidthPx:  int64(declaredWidth),
				DisplayHeightPx: int64(declaredHeight),
				Environment:     "ubuntu",
			},
			nil,
		).Definition()
	default:
		panic("unsupported computer use provider: " + provider)
	}
}

func (t *computerUseTool) ProviderOptions() fantasy.ProviderOptions {
	return t.providerOptions
}

func (t *computerUseTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	t.providerOptions = opts
}

func (t *computerUseTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	actions, err := t.parseActions(call.Input)
	if err != nil {
		return t.errorResponse(call.ID, xerrors.Errorf("invalid computer use input: %w", err)), nil
	}

	conn, err := t.getWorkspaceConn(ctx)
	if err != nil {
		return t.errorResponse(call.ID, xerrors.Errorf("failed to connect to workspace: %w", err)), nil
	}

	declaredWidth, declaredHeight := t.declaredActionDimensions()
	captureSharedScreenshot := false
	for _, action := range actions {
		switch action.kind {
		case computerUseActionKindWait:
			timer := t.clock.NewTimer(action.waitDuration, "computeruse", "wait")
			select {
			case <-ctx.Done():
			case <-timer.C:
			}
			timer.Stop()
		case computerUseActionKindScreenshot:
			captureSharedScreenshot = true
		case computerUseActionKindExecute:
			action.desktopAction.ScaledWidth = &declaredWidth
			action.desktopAction.ScaledHeight = &declaredHeight
			_, err := conn.ExecuteDesktopAction(ctx, action.desktopAction)
			if err != nil {
				return t.errorResponse(call.ID, xerrors.Errorf("action %q failed: %w", action.desktopAction.Action, err)), nil
			}
		default:
			return t.errorResponse(call.ID, xerrors.Errorf("unsupported computer use action kind %q", action.kind)), nil
		}
	}

	if captureSharedScreenshot {
		return t.captureSharedScreenshot(ctx, conn, call.ID, declaredWidth, declaredHeight)
	}
	return t.captureScreenshot(ctx, conn, call.ID, declaredWidth, declaredHeight)
}

func (t *computerUseTool) captureScreenshot(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	toolCallID string,
	declaredWidth, declaredHeight int,
) (fantasy.ToolResponse, error) {
	screenResp, err := executeScreenshotAction(ctx, conn, declaredWidth, declaredHeight)
	if err != nil {
		return t.errorResponse(toolCallID, xerrors.Errorf("screenshot failed: %w", err)), nil
	}
	return t.screenshotResponse(toolCallID, screenResp.ScreenshotData)
}

func (t *computerUseTool) captureSharedScreenshot(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	toolCallID string,
	declaredWidth, declaredHeight int,
) (fantasy.ToolResponse, error) {
	screenResp, err := executeScreenshotAction(ctx, conn, declaredWidth, declaredHeight)
	if err != nil {
		return t.errorResponse(toolCallID, xerrors.Errorf("screenshot failed: %w", err)), nil
	}

	response, err := t.screenshotResponse(toolCallID, screenResp.ScreenshotData)
	if err != nil {
		return t.errorResponse(toolCallID, xerrors.Errorf("screenshot failed: %w", err)), nil
	}

	attachmentName := fmt.Sprintf(
		"screenshot-%s.png",
		t.clock.Now().UTC().Format("2006-01-02T15-04-05Z"),
	)
	if t.storeFile == nil {
		t.logger.Warn(ctx, "screenshot attachment storage is not configured")
		return response, nil
	}

	attachment, err := storeScreenshotAttachment(
		ctx,
		t.storeFile,
		attachmentName,
		screenResp.ScreenshotData,
	)
	if err != nil {
		t.logger.Warn(ctx, "failed to persist screenshot attachment",
			slog.F("attachment_name", attachmentName),
			slog.Error(err),
		)
		return response, nil
	}
	return WithAttachments(response, attachment), nil
}

func executeScreenshotAction(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	declaredWidth, declaredHeight int,
) (workspacesdk.DesktopActionResponse, error) {
	screenshotAction := workspacesdk.DesktopAction{
		Action:       "screenshot",
		ScaledWidth:  &declaredWidth,
		ScaledHeight: &declaredHeight,
	}
	return conn.ExecuteDesktopAction(ctx, screenshotAction)
}

func (t *computerUseTool) declaredActionDimensions() (declaredWidth, declaredHeight int) {
	if t.declaredWidth <= 0 || t.declaredHeight <= 0 {
		geometry := workspacesdk.DefaultDesktopGeometry()
		return geometry.DeclaredWidth, geometry.DeclaredHeight
	}
	return t.declaredWidth, t.declaredHeight
}

type computerUseActionKind string

const (
	computerUseActionKindExecute    computerUseActionKind = "execute"
	computerUseActionKindScreenshot computerUseActionKind = "screenshot"
	computerUseActionKindWait       computerUseActionKind = "wait"
)

type normalizedComputerUseAction struct {
	kind          computerUseActionKind
	desktopAction workspacesdk.DesktopAction
	waitDuration  time.Duration
}

func (t *computerUseTool) parseActions(input string) ([]normalizedComputerUseAction, error) {
	switch t.provider {
	case "anthropic":
		parsed, err := fantasyanthropic.ParseComputerUseInput(input)
		if err != nil {
			return nil, err
		}
		return normalizeAnthropicComputerUseInput(parsed)
	case "openai":
		parsed, err := fantasyopenai.ParseComputerUseInput([]byte(input))
		if err != nil {
			return nil, err
		}
		return normalizeOpenAIComputerUseInput(parsed)
	default:
		return nil, xerrors.Errorf("unsupported computer use provider %q", t.provider)
	}
}

func normalizeAnthropicComputerUseInput(
	input fantasyanthropic.ComputerUseInput,
) ([]normalizedComputerUseAction, error) {
	action := normalizedComputerUseAction{}
	switch input.Action {
	case fantasyanthropic.ActionWait:
		duration := input.Duration
		if duration <= 0 {
			duration = 1000
		}
		action.kind = computerUseActionKindWait
		action.waitDuration = time.Duration(duration) * time.Millisecond
	case fantasyanthropic.ActionScreenshot:
		action.kind = computerUseActionKindScreenshot
	default:
		action.kind = computerUseActionKindExecute
		action.desktopAction = workspacesdk.DesktopAction{Action: string(input.Action)}
		if input.Coordinate != ([2]int64{}) {
			action.desktopAction.Coordinate = coordinateFromInt64(input.Coordinate[0], input.Coordinate[1])
		}
		if input.StartCoordinate != ([2]int64{}) {
			action.desktopAction.StartCoordinate = coordinateFromInt64(input.StartCoordinate[0], input.StartCoordinate[1])
		}
		if input.Text != "" {
			text := input.Text
			action.desktopAction.Text = &text
		}
		if input.Duration > 0 {
			duration := int(input.Duration)
			action.desktopAction.Duration = &duration
		}
		if input.ScrollAmount > 0 {
			scrollAmount := int(input.ScrollAmount)
			action.desktopAction.ScrollAmount = &scrollAmount
		}
		if input.ScrollDirection != "" {
			scrollDirection := input.ScrollDirection
			action.desktopAction.ScrollDirection = &scrollDirection
		}
	}
	return []normalizedComputerUseAction{action}, nil
}

func normalizeOpenAIComputerUseInput(
	input fantasyopenai.ComputerUseInput,
) ([]normalizedComputerUseAction, error) {
	if input.Action != nil {
		action, err := normalizeOpenAIComputerUseAction(input.Action)
		if err != nil {
			return nil, err
		}
		return []normalizedComputerUseAction{action}, nil
	}
	if len(input.Actions) == 0 {
		return nil, xerrors.New("computer use input is empty")
	}
	actions := make([]normalizedComputerUseAction, 0, len(input.Actions))
	for _, action := range input.Actions {
		normalized, err := normalizeOpenAIComputerUseAction(action)
		if err != nil {
			return nil, err
		}
		actions = append(actions, normalized)
	}
	return actions, nil
}

func normalizeOpenAIComputerUseAction(
	action fantasyopenai.ComputerUseAction,
) (normalizedComputerUseAction, error) {
	switch typed := action.(type) {
	case fantasyopenai.ComputerUseClickAction:
		desktopAction, err := openAIClickActionName(typed.Button)
		if err != nil {
			return normalizedComputerUseAction{}, err
		}
		return normalizedComputerUseAction{
			kind: computerUseActionKindExecute,
			desktopAction: workspacesdk.DesktopAction{
				Action:     desktopAction,
				Coordinate: coordinateFromInt64(typed.X, typed.Y),
			},
		}, nil
	case fantasyopenai.ComputerUseDoubleClickAction:
		return normalizedComputerUseAction{
			kind: computerUseActionKindExecute,
			desktopAction: workspacesdk.DesktopAction{
				Action:     "double_click",
				Coordinate: coordinateFromInt64(typed.X, typed.Y),
			},
		}, nil
	case fantasyopenai.ComputerUseDragAction:
		start := typed.Path[0]
		end := typed.Path[len(typed.Path)-1]
		return normalizedComputerUseAction{
			kind: computerUseActionKindExecute,
			desktopAction: workspacesdk.DesktopAction{
				Action:          "left_click_drag",
				StartCoordinate: coordinateFromInt64(start.X, start.Y),
				Coordinate:      coordinateFromInt64(end.X, end.Y),
			},
		}, nil
	case fantasyopenai.ComputerUseKeypressAction:
		keys := strings.Join(typed.Keys, "+")
		return normalizedComputerUseAction{
			kind: computerUseActionKindExecute,
			desktopAction: workspacesdk.DesktopAction{
				Action: "key",
				Text:   &keys,
			},
		}, nil
	case fantasyopenai.ComputerUseMoveAction:
		return normalizedComputerUseAction{
			kind: computerUseActionKindExecute,
			desktopAction: workspacesdk.DesktopAction{
				Action:     "mouse_move",
				Coordinate: coordinateFromInt64(typed.X, typed.Y),
			},
		}, nil
	case fantasyopenai.ComputerUseScreenshotAction:
		return normalizedComputerUseAction{kind: computerUseActionKindScreenshot}, nil
	case fantasyopenai.ComputerUseScrollAction:
		direction, amount, err := openAIScrollDirectionAndAmount(typed.ScrollX, typed.ScrollY)
		if err != nil {
			return normalizedComputerUseAction{}, err
		}
		return normalizedComputerUseAction{
			kind: computerUseActionKindExecute,
			desktopAction: workspacesdk.DesktopAction{
				Action:          "scroll",
				Coordinate:      coordinateFromInt64(typed.X, typed.Y),
				ScrollAmount:    intPtr(amount),
				ScrollDirection: stringPtr(direction),
			},
		}, nil
	case fantasyopenai.ComputerUseTypeAction:
		text := typed.Text
		return normalizedComputerUseAction{
			kind: computerUseActionKindExecute,
			desktopAction: workspacesdk.DesktopAction{
				Action: "type",
				Text:   &text,
			},
		}, nil
	case fantasyopenai.ComputerUseWaitAction:
		return normalizedComputerUseAction{
			kind:         computerUseActionKindWait,
			waitDuration: time.Second,
		}, nil
	default:
		return normalizedComputerUseAction{}, xerrors.Errorf("unsupported computer use action type %q", action.Type())
	}
}

func openAIClickActionName(button string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(button)) {
	case "", "left":
		return "left_click", nil
	case "right":
		return "right_click", nil
	case "middle":
		return "middle_click", nil
	default:
		return "", xerrors.Errorf("unsupported computer use click button %q", button)
	}
}

func openAIScrollDirectionAndAmount(scrollX, scrollY int64) (string, int, error) {
	absX := absInt64(scrollX)
	absY := absInt64(scrollY)
	if absX == 0 && absY == 0 {
		return "", 0, xerrors.New("computer use scroll action requires non-zero scroll_x or scroll_y")
	}
	if absY >= absX && absY > 0 {
		if scrollY > 0 {
			return "down", int(absY), nil
		}
		return "up", int(absY), nil
	}
	if scrollX > 0 {
		return "right", int(absX), nil
	}
	return "left", int(absX), nil
}

func coordinateFromInt64(x, y int64) *[2]int {
	coord := [2]int{int(x), int(y)}
	return &coord
}

func intPtr(v int) *int {
	return &v
}

func stringPtr(v string) *string {
	return &v
}

func absInt64(v int64) int64 {
	return int64(math.Abs(float64(v)))
}

func normalizeComputerUseProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func (t *computerUseTool) screenshotResponse(toolCallID string, encodedPNG string) (fantasy.ToolResponse, error) {
	switch t.provider {
	case "openai":
		pngData, err := base64.StdEncoding.DecodeString(encodedPNG)
		if err != nil {
			return fantasy.ToolResponse{}, err
		}
		return toolResponseFromOpenAIComputerUseResult(
			fantasyopenai.NewComputerUseScreenshotResult(toolCallID, pngData),
		), nil
	default:
		return fantasy.NewImageResponse([]byte(encodedPNG), "image/png"), nil
	}
}

func (t *computerUseTool) errorResponse(toolCallID string, err error) fantasy.ToolResponse {
	if t.provider == "openai" {
		return toolResponseFromOpenAIComputerUseResult(
			fantasyopenai.NewComputerUseErrorResult(toolCallID, err),
		)
	}
	return fantasy.NewTextErrorResponse(err.Error())
}

func toolResponseFromOpenAIComputerUseResult(result fantasy.ToolResultPart) fantasy.ToolResponse {
	switch output := result.Output.(type) {
	case fantasy.ToolResultOutputContentMedia:
		return fantasy.NewImageResponse([]byte(output.Data), output.MediaType)
	case fantasy.ToolResultOutputContentText:
		return fantasy.NewTextResponse(output.Text)
	case fantasy.ToolResultOutputContentError:
		return fantasy.NewTextErrorResponse(output.Error.Error())
	default:
		return fantasy.NewTextErrorResponse("unsupported openai computer use result type")
	}
}
