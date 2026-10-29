package chattool

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	openaicomputeruse "github.com/coder/coder/v2/coderd/x/chatd/chatopenai/computeruse"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

// SupportedComputerUseProviders returns the providers supported by computer use.
// The returned slice is a fresh copy and safe to mutate.
func SupportedComputerUseProviders() []string {
	providers := make([]string, len(codersdk.AllChatComputerUseProviders))
	for i, p := range codersdk.AllChatComputerUseProviders {
		providers[i] = string(p)
	}
	return providers
}

// DefaultComputerUseProvider returns the effective computer use provider.
func DefaultComputerUseProvider(provider codersdk.ChatComputerUseProvider) codersdk.ChatComputerUseProvider {
	if provider == "" {
		return codersdk.ChatComputerUseProviderAnthropic
	}
	return provider
}

// DefaultComputerUseDesktopGeometry returns provider-specific model-facing
// desktop geometry for computer use.
func DefaultComputerUseDesktopGeometry(provider codersdk.ChatComputerUseProvider) workspacesdk.DesktopGeometry {
	switch DefaultComputerUseProvider(provider) {
	case codersdk.ChatComputerUseProviderOpenAI:
		return workspacesdk.DefaultOpenAIComputerUseDesktopGeometry()
	default:
		return workspacesdk.DefaultDesktopGeometry()
	}
}

// computerUseToolName is the name every computer use provider tool and its
// runner share.
const computerUseToolName = "computer"

// anthropicHaltedActionText answers every Anthropic computer call that follows
// a failed one in the same step. The model batches actions that assume the
// earlier ones succeeded, so running them could act on the wrong screen.
const anthropicHaltedActionText = "Not executed: an earlier computer action in this turn failed."

// anthropicMaxDurationSeconds caps Anthropic wait and hold_key durations, which
// the Anthropic schema expresses in seconds.
const anthropicMaxDurationSeconds = 300

// anthropicMaxKeyRepeat is the largest key repeat count the Anthropic
// computer use toolset accepts.
const anthropicMaxKeyRepeat = 100

// computerUseTool implements fantasy.AgentTool and chatloop.ToolDefiner.
type computerUseTool struct {
	provider codersdk.ChatComputerUseProvider
	// anthropicToolVersion is the Anthropic tool version declared to the
	// model. It decides the shape of Anthropic action results.
	anthropicToolVersion fantasyanthropic.ComputerUseToolVersion
	declaredWidth        int
	declaredHeight       int
	getWorkspaceConn     func(ctx context.Context) (workspacesdk.AgentConn, error)
	storeFile            StoreFileFunc
	providerOptions      fantasy.ProviderOptions
	clock                quartz.Clock
	logger               slog.Logger

	// step tracks the computer calls of the current step so Anthropic
	// batches halt after a failure and end with a screenshot.
	stepMu sync.Mutex
	step   computerUseStep
}

// computerUseStep is the per-step state of Anthropic computer calls.
type computerUseStep struct {
	// remaining counts the step's computer calls that have not run yet.
	remaining int
	// failed is set once a computer call in the step returns an error.
	failed bool
}

// NewComputerUseTool creates a provider-aware computer use AgentTool that
// delegates to the agent's desktop endpoints. modelID selects the Anthropic
// tool version, as in ComputerUseProviderTool. declaredWidth and
// declaredHeight are the model-facing desktop dimensions advertised to
// providers and requested for screenshots.
func NewComputerUseTool(
	provider codersdk.ChatComputerUseProvider,
	modelID string,
	declaredWidth, declaredHeight int,
	getWorkspaceConn func(ctx context.Context) (workspacesdk.AgentConn, error),
	storeFile StoreFileFunc,
	clock quartz.Clock,
	logger slog.Logger,
) fantasy.AgentTool {
	return &computerUseTool{
		provider:             DefaultComputerUseProvider(provider),
		anthropicToolVersion: fantasyanthropic.ComputerUseToolVersionForModel(modelID),
		declaredWidth:        declaredWidth,
		declaredHeight:       declaredHeight,
		getWorkspaceConn:     getWorkspaceConn,
		storeFile:            storeFile,
		clock:                clock,
		logger:               logger,
	}
}

func (*computerUseTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{
		Name: computerUseToolName,
		Description: "Control the desktop: take screenshots, move the mouse, click, type, and scroll. " +
			"Use an explicit screenshot action when you want to share a screenshot with the user; " +
			"those screenshots are also attached to the chat.",
		Parameters: map[string]any{},
		Required:   []string{},
	}
}

// ComputerUseProviderTool creates the provider-defined computer-use tool
// definition using the declared model-facing desktop geometry. For Anthropic,
// modelID selects the tool version the model accepts.
func ComputerUseProviderTool(
	provider codersdk.ChatComputerUseProvider,
	modelID string,
	declaredWidth, declaredHeight int,
) (fantasy.Tool, error) {
	switch DefaultComputerUseProvider(provider) {
	case codersdk.ChatComputerUseProviderAnthropic:
		opts := fantasyanthropic.ComputerUseToolOptions{
			ToolVersion: fantasyanthropic.ComputerUseToolVersionForModel(modelID),
		}
		if opts.ToolVersion == fantasyanthropic.ComputerUseToolset20260801 {
			// The toolset offers zoom unless it is disabled, and the
			// agent desktop has no zoom action. Toolset coordinates are
			// in screenshot pixels, so it takes no display size.
			enableZoom := false
			opts.EnableZoom = &enableZoom
		} else {
			opts.DisplayWidthPx = int64(declaredWidth)
			opts.DisplayHeightPx = int64(declaredHeight)
		}
		// The run callback is nil because execution is handled separately
		// by the AgentTool runner in the chatloop. We extract just the
		// provider-defined tool definition.
		return fantasyanthropic.NewComputerUseTool(opts, nil).Definition(), nil
	case codersdk.ChatComputerUseProviderOpenAI:
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

// SerialToolCalls runs a step's computer calls one at a time in call order,
// because they act on one shared desktop.
func (*computerUseTool) SerialToolCalls() bool { return true }

// ObserveStepToolCalls starts a step: it counts the step's computer calls so
// the runner knows which call is the last one, and clears the failure state of
// the previous step.
func (t *computerUseTool) ObserveStepToolCalls(names []string) {
	calls := 0
	for _, name := range names {
		if name == computerUseToolName {
			calls++
		}
	}
	t.stepMu.Lock()
	defer t.stepMu.Unlock()
	t.step = computerUseStep{remaining: calls}
}

// beginStepCall claims the next computer call of the step. last reports
// whether no other computer call follows it in the step; a call outside an
// observed step counts as the last. halted reports whether an earlier call in
// the step failed.
func (t *computerUseTool) beginStepCall() (last, halted bool) {
	t.stepMu.Lock()
	defer t.stepMu.Unlock()
	if t.step.remaining > 0 {
		t.step.remaining--
	}
	return t.step.remaining == 0, t.step.failed
}

// endStepCall records the outcome of a computer call. The last call of a step
// clears the failure state so it cannot leak into a later step.
func (t *computerUseTool) endStepCall(last, failed bool) {
	t.stepMu.Lock()
	defer t.stepMu.Unlock()
	t.step.failed = !last && (t.step.failed || failed)
}

func (t *computerUseTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	switch DefaultComputerUseProvider(t.provider) {
	case codersdk.ChatComputerUseProviderAnthropic:
		last, halted := t.beginStepCall()
		if halted {
			t.endStepCall(last, true)
			return fantasy.NewTextErrorResponse(anthropicHaltedActionText), nil
		}
		// The toolset answers actions with text and attaches a screenshot
		// only to the last result of a batch. Older versions screenshot
		// after every action.
		result := anthropicResultScreenshot
		if t.anthropicToolVersion == fantasyanthropic.ComputerUseToolset20260801 && !last {
			result = anthropicResultText
		}
		resp, err := t.runAnthropicComputerUse(ctx, call, result)
		t.endStepCall(last, err != nil || resp.IsError)
		return resp, err
	case codersdk.ChatComputerUseProviderOpenAI:
		return t.runOpenAIComputerUse(ctx, call)
	default:
		return fantasy.NewTextErrorResponse(fmt.Sprintf(
			"unsupported computer use provider %q, supported providers: %s",
			t.provider,
			strings.Join(SupportedComputerUseProviders(), ", "),
		)), nil
	}
}

// anthropicActionResult selects what a successful Anthropic action other
// than screenshot returns to the model.
type anthropicActionResult int

const (
	// anthropicResultScreenshot returns a screenshot taken after the action.
	anthropicResultScreenshot anthropicActionResult = iota
	// anthropicResultText returns the action's text output.
	anthropicResultText
)

// runAnthropicComputerUse runs one Anthropic computer action and answers it
// as result selects.
func (t *computerUseTool) runAnthropicComputerUse(
	ctx context.Context,
	call fantasy.ToolCall,
	result anthropicActionResult,
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

	switch {
	case input.Action == fantasyanthropic.ActionScreenshot:
		return t.captureSharedScreenshot(ctx, conn, declaredWidth, declaredHeight)
	case input.Action == fantasyanthropic.ActionWait:
		t.wait(ctx, anthropicDurationMillis(input.Duration))
		if result == anthropicResultText {
			return fantasy.NewTextResponse("wait action performed"), nil
		}
		return t.captureScreenshot(ctx, conn, declaredWidth, declaredHeight)
	case t.anthropicToolVersion == fantasyanthropic.ComputerUseToolset20260801 && input.Action == "cursor_position":
		// The position is the answer, even for the last call of a batch.
		// The agent reports it in declared coordinates, which are the
		// toolset's screenshot coordinates.
		action := t.desktopAction(string(input.Action), declaredWidth, declaredHeight)
		actionResp, errResp, done := t.executeDesktopAction(ctx, conn, action)
		if done {
			return errResp, nil
		}
		return fantasy.NewTextResponse(actionResp.Output), nil
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
		d := int(anthropicDurationMillis(input.Duration))
		action.Duration = &d
	}
	if input.ScrollAmount > 0 {
		s := int(input.ScrollAmount)
		action.ScrollAmount = &s
	}
	if input.ScrollDirection != "" {
		action.ScrollDirection = &input.ScrollDirection
	}

	presses := int64(1)
	if input.Action == fantasyanthropic.ActionKey && input.Repeat != 0 {
		if input.Repeat < 1 || input.Repeat > anthropicMaxKeyRepeat {
			return fantasy.NewTextErrorResponse(fmt.Sprintf(
				"invalid computer use input: repeat must be between 1 and %d, got %d",
				anthropicMaxKeyRepeat, input.Repeat,
			)), nil
		}
		presses = input.Repeat
	}
	var actionResp workspacesdk.DesktopActionResponse
	for range presses {
		var errResp fantasy.ToolResponse
		var done bool
		actionResp, errResp, done = t.executeDesktopAction(ctx, conn, action)
		if done {
			return errResp, nil
		}
	}

	if result == anthropicResultText {
		output := actionResp.Output
		if output == "" {
			output = fmt.Sprintf("%s action performed", input.Action)
		}
		return fantasy.NewTextResponse(output), nil
	}
	return t.captureScreenshot(ctx, conn, declaredWidth, declaredHeight)
}

// anthropicDurationMillis converts an Anthropic duration in seconds to
// milliseconds, capped at anthropicMaxDurationSeconds. Zero and negative
// durations stay zero so callers apply their defaults.
func anthropicDurationMillis(seconds int64) int64 {
	if seconds <= 0 {
		return 0
	}
	return min(seconds, anthropicMaxDurationSeconds) * 1000
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
		if _, resp, done := t.executeDesktopAction(ctx, conn, action.Action); done {
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

// executeDesktopAction runs action on the desktop. On failure it returns the
// error response to send to the model and done set to true.
func (*computerUseTool) executeDesktopAction(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	action workspacesdk.DesktopAction,
) (actionResp workspacesdk.DesktopActionResponse, errResp fantasy.ToolResponse, done bool) {
	actionResp, err := conn.ExecuteDesktopAction(ctx, action)
	if err != nil {
		return workspacesdk.DesktopActionResponse{}, fantasy.NewTextErrorResponse(
			fmt.Sprintf("action %q failed: %v", action.Action, err),
		), true
	}
	return actionResp, fantasy.ToolResponse{}, false
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
