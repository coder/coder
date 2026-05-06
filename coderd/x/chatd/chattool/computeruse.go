package chattool

import (
	"context"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	openaicomputeruse "github.com/coder/coder/v2/coderd/x/chatd/chatopenai/computeruse"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

const (
	// ComputerUseProviderAnthropic identifies Anthropic computer use.
	ComputerUseProviderAnthropic = "anthropic"
	// ComputerUseProviderOpenAI identifies OpenAI computer use.
	ComputerUseProviderOpenAI = "openai"
	// ComputerUseModelProviderDefault is the default model provider name for
	// computer use, equal to ComputerUseProviderAnthropic.
	ComputerUseModelProviderDefault = ComputerUseProviderAnthropic
	// ComputerUseAnthropicModelName is the default Anthropic model used for
	// computer use subagents.
	ComputerUseAnthropicModelName = "claude-opus-4-6"
	// ComputerUseOpenAIModelName is the default OpenAI model used for computer use.
	ComputerUseOpenAIModelName = "gpt-5.5"
)

// SupportedComputerUseProviders returns the providers supported by computer use.
// The returned slice is a fresh copy and safe to mutate.
func SupportedComputerUseProviders() []string {
	return []string{
		ComputerUseProviderAnthropic,
		ComputerUseProviderOpenAI,
	}
}

// IsSupportedComputerUseProvider reports whether provider supports computer use.
func IsSupportedComputerUseProvider(provider string) bool {
	return slices.Contains(SupportedComputerUseProviders(), provider)
}

// DefaultComputerUseProvider returns the effective computer use provider.
func DefaultComputerUseProvider(provider string) string {
	if provider == "" {
		return ComputerUseProviderAnthropic
	}
	return provider
}

// DefaultComputerUseModel returns the default model for a computer use provider.
func DefaultComputerUseModel(provider string) (modelProvider, modelName string, ok bool) {
	switch DefaultComputerUseProvider(provider) {
	case ComputerUseProviderAnthropic:
		return ComputerUseModelProviderDefault, ComputerUseAnthropicModelName, true
	case ComputerUseProviderOpenAI:
		// Keep OpenAI isolated here because computer-use models may advance.
		return ComputerUseProviderOpenAI, ComputerUseOpenAIModelName, true
	default:
		return "", "", false
	}
}

// DefaultComputerUseDesktopGeometry returns provider-specific model-facing
// desktop geometry for computer use.
func DefaultComputerUseDesktopGeometry(provider string) workspacesdk.DesktopGeometry {
	switch DefaultComputerUseProvider(provider) {
	case ComputerUseProviderOpenAI:
		return workspacesdk.DefaultOpenAIComputerUseDesktopGeometry()
	default:
		return workspacesdk.DefaultDesktopGeometry()
	}
}

// computerUseTool implements fantasy.AgentTool and chatloop.ToolDefiner.
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

// NewComputerUseTool creates a provider-aware computer use AgentTool that
// delegates to the agent's desktop endpoints. declaredWidth and declaredHeight
// are the model-facing desktop dimensions advertised to providers and requested
// for screenshots.
func NewComputerUseTool(
	provider string,
	declaredWidth, declaredHeight int,
	getWorkspaceConn func(ctx context.Context) (workspacesdk.AgentConn, error),
	storeFile StoreFileFunc,
	clock quartz.Clock,
	logger slog.Logger,
) fantasy.AgentTool {
	return &computerUseTool{
		provider:         DefaultComputerUseProvider(provider),
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
func ComputerUseProviderTool(provider string, declaredWidth, declaredHeight int) (fantasy.Tool, error) {
	switch DefaultComputerUseProvider(provider) {
	case ComputerUseProviderAnthropic:
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
		).Definition(), nil
	case ComputerUseProviderOpenAI:
		// OpenAI's GA computer tool schema does not accept display
		// dimensions. The declared geometry is applied through screenshot
		// sizing and desktop action coordinate scaling.
		return openaicomputeruse.Tool(), nil
	default:
		return nil, xerrors.Errorf("unsupported computer use provider %q, supported providers: %s", provider,
			strings.Join(SupportedComputerUseProviders(), ", "))
	}
}

func (t *computerUseTool) ProviderOptions() fantasy.ProviderOptions {
	return t.providerOptions
}

func (t *computerUseTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	t.providerOptions = opts
}

func (t *computerUseTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	switch DefaultComputerUseProvider(t.provider) {
	case ComputerUseProviderAnthropic:
		return t.runAnthropicComputerUse(ctx, call)
	case ComputerUseProviderOpenAI:
		return t.runOpenAIComputerUse(ctx, call)
	default:
		return fantasy.NewTextErrorResponse(fmt.Sprintf(
			"unsupported computer use provider %q, supported providers: %s",
			t.provider,
			strings.Join(SupportedComputerUseProviders(), ", "),
		)), nil
	}
}

func (t *computerUseTool) runAnthropicComputerUse(
	ctx context.Context,
	call fantasy.ToolCall,
) (fantasy.ToolResponse, error) {
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
		t.wait(ctx, input.Duration)
		return t.captureScreenshot(ctx, conn, declaredWidth, declaredHeight)
	}

	// For screenshot action, use ExecuteDesktopAction.
	if input.Action == fantasyanthropic.ActionScreenshot {
		return t.captureSharedScreenshot(ctx, conn, declaredWidth, declaredHeight)
	}

	// Build the action request.
	action := t.desktopAction(string(input.Action), declaredWidth, declaredHeight)
	if input.Coordinate != ([2]int64{}) {
		coord := coordinateFromInt64(input.Coordinate[0], input.Coordinate[1])
		action.Coordinate = &coord
	}
	if input.StartCoordinate != ([2]int64{}) {
		coord := coordinateFromInt64(input.StartCoordinate[0], input.StartCoordinate[1])
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

	if resp, done := t.executeDesktopAction(ctx, conn, action); done {
		return resp, nil
	}

	// Take a screenshot after every action (Anthropic pattern).
	return t.captureScreenshot(ctx, conn, declaredWidth, declaredHeight)
}

func (t *computerUseTool) runOpenAIComputerUse(
	ctx context.Context,
	call fantasy.ToolCall,
) (fantasy.ToolResponse, error) {
	input, err := openaicomputeruse.ParseInput(call.Input)
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
	actions, err := openaicomputeruse.DesktopActions(
		input,
		declaredWidth,
		declaredHeight,
	)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	for _, action := range actions {
		if action.WaitDurationMillis > 0 {
			t.wait(ctx, action.WaitDurationMillis)
			continue
		}
		if resp, done := t.executeDesktopAction(ctx, conn, action.Action); done {
			if action.ReleaseMouseOnFailure {
				_, err := conn.ExecuteDesktopAction(
					ctx,
					t.desktopAction("left_mouse_up", declaredWidth, declaredHeight),
				)
				if err != nil {
					t.logger.Warn(ctx, "failed to release mouse after OpenAI drag error",
						slog.Error(err),
					)
				}
			}
			t.releaseOpenAIModifierKeys(ctx, conn, action.ReleaseKeysOnFailure)
			return resp, nil
		}
	}
	return t.captureSharedScreenshot(ctx, conn, declaredWidth, declaredHeight)
}

func (t *computerUseTool) releaseOpenAIModifierKeys(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	keys []string,
) {
	for i := len(keys) - 1; i >= 0; i-- {
		key := keys[i]
		action := t.desktopAction("key_up", 0, 0)
		action.Text = &key
		if _, err := conn.ExecuteDesktopAction(ctx, action); err != nil {
			t.logger.Warn(ctx, "failed to release OpenAI modifier key",
				slog.F("key", key),
				slog.Error(err),
			)
		}
	}
}

func (*computerUseTool) executeDesktopAction(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	action workspacesdk.DesktopAction,
) (fantasy.ToolResponse, bool) {
	_, err := conn.ExecuteDesktopAction(ctx, action)
	if err != nil {
		return fantasy.NewTextErrorResponse(
			fmt.Sprintf("action %q failed: %v", action.Action, err),
		), true
	}
	return fantasy.ToolResponse{}, false
}

func (*computerUseTool) desktopAction(
	action string,
	declaredWidth, declaredHeight int,
) workspacesdk.DesktopAction {
	return workspacesdk.DesktopAction{
		Action:       action,
		ScaledWidth:  &declaredWidth,
		ScaledHeight: &declaredHeight,
	}
}

func (t *computerUseTool) wait(ctx context.Context, durationMillis int64) {
	d := durationMillis
	if d <= 0 {
		d = 1000
	}
	timer := t.clock.NewTimer(time.Duration(d)*time.Millisecond, "computeruse", "wait")
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func coordinateFromInt64(x, y int64) [2]int {
	return [2]int{int(x), int(y)}
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
		geometry := DefaultComputerUseDesktopGeometry(t.provider)
		return geometry.DeclaredWidth, geometry.DeclaredHeight
	}
	return t.declaredWidth, t.declaredHeight
}
