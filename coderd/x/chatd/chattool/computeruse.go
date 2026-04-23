package chattool

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

const (
	// ComputerUseModelProvider is the provider for the computer
	// use model.
	ComputerUseModelProvider = "anthropic"
	// ComputerUseModelName is the model used for computer use
	// subagents.
	ComputerUseModelName = "claude-opus-4-6"
)

// computerUseTool implements fantasy.AgentTool and
// chatloop.ToolDefiner for Anthropic computer use.
type computerUseTool struct {
	declaredWidth    int
	declaredHeight   int
	getWorkspaceConn func(ctx context.Context) (workspacesdk.AgentConn, error)
	storeFile        StoreFileFunc
	providerOptions  fantasy.ProviderOptions
	clock            quartz.Clock
	logger           slog.Logger
}

// NewComputerUseTool creates a computer use AgentTool that delegates to the
// agent's desktop endpoints. declaredWidth and declaredHeight are the
// model-facing desktop dimensions advertised to Anthropic and requested for
// screenshots.
func NewComputerUseTool(
	declaredWidth, declaredHeight int,
	getWorkspaceConn func(ctx context.Context) (workspacesdk.AgentConn, error),
	storeFile StoreFileFunc,
	clock quartz.Clock,
	logger slog.Logger,
) fantasy.AgentTool {
	return &computerUseTool{
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

// ComputerUseProviderTool creates the provider-defined Anthropic computer-use
// tool definition using the declared model-facing desktop geometry.
func ComputerUseProviderTool(declaredWidth, declaredHeight int) fantasy.Tool {
	// The run callback is nil because execution is handled separately
	// by the AgentTool runner in the chatloop. We extract just the
	// provider-defined tool definition.
	return fantasyanthropic.NewComputerUseTool(
		fantasyanthropic.ComputerUseToolOptions{
			DisplayWidthPx:  int64(declaredWidth),
			DisplayHeightPx: int64(declaredHeight),
			ToolVersion:     fantasyanthropic.ComputerUse20251124,
		},
		nil,
	).Definition()
}

func (t *computerUseTool) ProviderOptions() fantasy.ProviderOptions {
	return t.providerOptions
}

func (t *computerUseTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	t.providerOptions = opts
}

func (t *computerUseTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	input, err := fantasyanthropic.ParseComputerUseInput(call.Input)
	if err != nil {
		return fantasy.NewTextErrorResponse(
			fmt.Sprintf("invalid computer use input: %v", err),
		), nil
	}

	conn, err := t.getWorkspaceConn(ctx)
	if err != nil {
		return fantasy.NewTextErrorResponse(
			fmt.Sprintf("failed to connect to workspace: %v", err),
		), nil
	}

	declaredWidth, declaredHeight := t.declaredActionDimensions()

	// For wait actions, sleep then return a screenshot.
	if input.Action == fantasyanthropic.ActionWait {
		d := input.Duration
		if d <= 0 {
			d = 1000
		}
		timer := t.clock.NewTimer(time.Duration(d)*time.Millisecond, "computeruse", "wait")
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
		return t.captureScreenshot(ctx, conn, declaredWidth, declaredHeight)
	}

	// For screenshot action, use ExecuteDesktopAction.
	if input.Action == fantasyanthropic.ActionScreenshot {
		return t.captureSharedScreenshot(ctx, conn, declaredWidth, declaredHeight)
	}

	// Build the action request.
	action := workspacesdk.DesktopAction{
		Action:       string(input.Action),
		ScaledWidth:  &declaredWidth,
		ScaledHeight: &declaredHeight,
	}
	if input.Coordinate != ([2]int64{}) {
		coord := [2]int{int(input.Coordinate[0]), int(input.Coordinate[1])}
		action.Coordinate = &coord
	}
	if input.StartCoordinate != ([2]int64{}) {
		coord := [2]int{int(input.StartCoordinate[0]), int(input.StartCoordinate[1])}
		action.StartCoordinate = &coord
	}
	if input.Text != "" {
		action.Text = &input.Text
	}
	if input.Duration > 0 {
		d := int(input.Duration)
		action.Duration = &d
	}
	if input.ScrollAmount > 0 {
		s := int(input.ScrollAmount)
		action.ScrollAmount = &s
	}
	if input.ScrollDirection != "" {
		action.ScrollDirection = &input.ScrollDirection
	}

	// Execute the action.
	_, err = conn.ExecuteDesktopAction(ctx, action)
	if err != nil {
		return fantasy.NewTextErrorResponse(
			fmt.Sprintf("action %q failed: %v", input.Action, err),
		), nil
	}

	// Take a screenshot after every action (Anthropic pattern).
	return t.captureScreenshot(ctx, conn, declaredWidth, declaredHeight)
}

func (t *computerUseTool) captureScreenshot(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	declaredWidth, declaredHeight int,
) (fantasy.ToolResponse, error) {
	screenResp, err := executeScreenshotAction(ctx, conn, declaredWidth, declaredHeight)
	if err != nil {
		return fantasy.NewTextErrorResponse(
			fmt.Sprintf("screenshot failed: %v", err),
		), nil
	}
	screenData, err := base64.StdEncoding.DecodeString(screenResp.ScreenshotData)
	if err != nil {
		t.logger.Error(ctx, "failed to decode screenshot base64 in captureScreenshot",
			slog.Error(err),
		)
		return fantasy.NewTextErrorResponse(
			fmt.Sprintf("failed to decode screenshot data: %v", err),
		), nil
	}
	return fantasy.NewImageResponse(screenData, "image/png"), nil
}

func (t *computerUseTool) captureSharedScreenshot(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	declaredWidth, declaredHeight int,
) (fantasy.ToolResponse, error) {
	screenResp, err := executeScreenshotAction(ctx, conn, declaredWidth, declaredHeight)
	if err != nil {
		return fantasy.NewTextErrorResponse(
			fmt.Sprintf("screenshot failed: %v", err),
		), nil
	}

	screenData, err := base64.StdEncoding.DecodeString(screenResp.ScreenshotData)
	if err != nil {
		t.logger.Error(ctx, "failed to decode screenshot base64 in captureSharedScreenshot",
			slog.Error(err),
		)
		return fantasy.NewTextErrorResponse(
			fmt.Sprintf("failed to decode screenshot data: %v", err),
		), nil
	}

	attachmentName := fmt.Sprintf(
		"screenshot-%s.png",
		t.clock.Now().UTC().Format("2006-01-02T15-04-05Z"),
	)
	if t.storeFile == nil {
		t.logger.Warn(ctx, "screenshot attachment storage is not configured")
		return fantasy.NewImageResponse(screenData, "image/png"), nil
	}

	response := fantasy.NewImageResponse(screenData, "image/png")

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
