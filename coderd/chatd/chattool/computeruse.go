package chattool

import (
	"context"
	"fmt"
	"math"
	"time"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"

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
	displayWidth     int
	displayHeight    int
	getWorkspaceConn func(ctx context.Context) (workspacesdk.AgentConn, error)
	providerOptions  fantasy.ProviderOptions
	clock            quartz.Clock
}

// NewComputerUseTool creates a computer use AgentTool that
// delegates to the agent's desktop endpoints.
func NewComputerUseTool(
	displayWidth, displayHeight int,
	getWorkspaceConn func(ctx context.Context) (workspacesdk.AgentConn, error),
	clock quartz.Clock,
) fantasy.AgentTool {
	return &computerUseTool{
		displayWidth:     displayWidth,
		displayHeight:    displayHeight,
		getWorkspaceConn: getWorkspaceConn,
		clock:            clock,
	}
}

func (*computerUseTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{
		Name:        "computer",
		Description: "Control the desktop: take screenshots, move the mouse, click, type, and scroll.",
		Parameters:  map[string]any{},
		Required:    []string{},
	}
}

// ComputerUseProviderTool creates the provider-defined tool
// definition for Anthropic computer use. This is passed via
// ProviderTools so the API receives the correct wire format.
func ComputerUseProviderTool(displayWidth, displayHeight int) fantasy.Tool {
	return fantasyanthropic.NewComputerUseTool(
		fantasyanthropic.ComputerUseToolOptions{
			DisplayWidthPx:  int64(displayWidth),
			DisplayHeightPx: int64(displayHeight),
			ToolVersion:     fantasyanthropic.ComputerUse20251124,
		},
	)
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

	// Compute scaled screenshot size for Anthropic constraints.
	scaledW, scaledH := computeScaledScreenshotSize(
		t.displayWidth, t.displayHeight,
	)

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
		screenshotAction := workspacesdk.DesktopAction{
			Action:       "screenshot",
			ScaledWidth:  &scaledW,
			ScaledHeight: &scaledH,
		}
		screenResp, sErr := conn.ExecuteDesktopAction(ctx, screenshotAction)
		if sErr != nil {
			return fantasy.NewTextErrorResponse(
				fmt.Sprintf("screenshot failed: %v", sErr),
			), nil
		}
		return fantasy.NewImageResponse(
			[]byte(screenResp.ScreenshotData), "image/png",
		), nil
	}

	// For screenshot action, use ExecuteDesktopAction.
	if input.Action == fantasyanthropic.ActionScreenshot {
		screenshotAction := workspacesdk.DesktopAction{
			Action:       "screenshot",
			ScaledWidth:  &scaledW,
			ScaledHeight: &scaledH,
		}
		screenResp, sErr := conn.ExecuteDesktopAction(ctx, screenshotAction)
		if sErr != nil {
			return fantasy.NewTextErrorResponse(
				fmt.Sprintf("screenshot failed: %v", sErr),
			), nil
		}
		return fantasy.NewImageResponse(
			[]byte(screenResp.ScreenshotData), "image/png",
		), nil
	}

	// Build the action request.
	action := workspacesdk.DesktopAction{
		Action:       string(input.Action),
		ScaledWidth:  &scaledW,
		ScaledHeight: &scaledH,
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
	screenshotAction := workspacesdk.DesktopAction{
		Action:       "screenshot",
		ScaledWidth:  &scaledW,
		ScaledHeight: &scaledH,
	}
	screenResp, sErr := conn.ExecuteDesktopAction(ctx, screenshotAction)
	if sErr != nil {
		return fantasy.NewTextErrorResponse(
			fmt.Sprintf("screenshot failed: %v", sErr),
		), nil
	}

	return fantasy.NewImageResponse(
		[]byte(screenResp.ScreenshotData), "image/png",
	), nil
}

// computeScaledScreenshotSize computes the target screenshot
// dimensions to fit within Anthropic's constraints.
func computeScaledScreenshotSize(width, height int) (scaledWidth int, scaledHeight int) {
	const maxLongEdge = 1568
	const maxTotalPixels = 1_150_000

	longEdge := max(width, height)
	totalPixels := width * height
	longEdgeScale := float64(maxLongEdge) / float64(longEdge)
	totalPixelsScale := math.Sqrt(
		float64(maxTotalPixels) / float64(totalPixels),
	)
	scale := min(1.0, longEdgeScale, totalPixelsScale)

	if scale >= 1.0 {
		return width, height
	}
	return max(1, int(float64(width)*scale)),
		max(1, int(float64(height)*scale))
}
