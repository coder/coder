package chattool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
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

// computerUseToolName is the name every computer use flavor declares.
const computerUseToolName = "computer"

// openaiCompatComputerUseActions lists the desktop actions the openai-compat
// flavor offers. It is the vocabulary the agent desktop implements and the
// shared action runner decodes, minus zoom, which the desktop has no way to
// perform.
var openaiCompatComputerUseActions = []string{
	"screenshot",
	"left_click",
	"right_click",
	"middle_click",
	"double_click",
	"triple_click",
	"mouse_move",
	"left_click_drag",
	"left_mouse_down",
	"left_mouse_up",
	"type",
	"key",
	"hold_key",
	"scroll",
	"wait",
}

// openaiCompatComputerUseTool declares the computer use tool for models reached
// through the OpenAI chat completions API. Those servers accept a hosted
// computer tool definition and then drop it, so this flavor declares an
// ordinary function tool and returns a screenshot with every action.
// declaredWidth and declaredHeight are the model-facing desktop pixels the
// runner maps coordinates against.
func openaiCompatComputerUseTool(declaredWidth, declaredHeight int) fantasy.Tool {
	return fantasy.FunctionTool{
		Name: computerUseToolName,
		Description: fmt.Sprintf(
			"Control a %d by %d pixel desktop. Every call answers with a screenshot of the current screen, "+
				"so take a screenshot first and act on what it shows. Coordinates are pixels measured from the "+
				"top-left corner of the screenshot you last saw.",
			declaredWidth, declaredHeight,
		),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        openaiCompatComputerUseActions,
					"description": "The desktop action to perform.",
				},
				"coordinate": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "integer"},
					"minItems":    2,
					"maxItems":    2,
					"description": "The [x, y] pixel position for actions that act on a place.",
				},
				"start_coordinate": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "integer"},
					"minItems":    2,
					"maxItems":    2,
					"description": "The [x, y] pixel position a drag starts from.",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "The text to type, the key combination to press, or the key to hold.",
				},
				"scroll_direction": map[string]any{
					"type":        "string",
					"enum":        []string{"up", "down", "left", "right"},
					"description": "The direction to scroll.",
				},
				"scroll_amount": map[string]any{
					"type":        "integer",
					"description": "How many wheel clicks to scroll.",
				},
				"duration": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"maximum":     anthropicMaxDurationSeconds,
					"description": "How many whole seconds to hold a key or wait.",
				},
				"repeat": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     anthropicMaxKeyRepeat,
					"description": "How many times to press a key.",
				},
			},
			"required": []string{"action"},
		},
	}
}

// computerUseHaltedActionText answers the computer calls of a step that
// follow a failed one.
const computerUseHaltedActionText = "Not executed: an earlier computer action in this turn failed."

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
}

// NewComputerUseTool creates a provider-aware computer use AgentTool that
// delegates to the agent's desktop endpoints. anthropicToolVersion is the
// Anthropic tool version declared to the model. declaredWidth and declaredHeight
// are the model-facing desktop dimensions advertised to providers and requested
// for screenshots.
func NewComputerUseTool(
	provider codersdk.ChatComputerUseProvider,
	anthropicToolVersion fantasyanthropic.ComputerUseToolVersion,
	declaredWidth, declaredHeight int,
	getWorkspaceConn func(ctx context.Context) (workspacesdk.AgentConn, error),
	storeFile StoreFileFunc,
	clock quartz.Clock,
	logger slog.Logger,
) fantasy.AgentTool {
	return &computerUseTool{
		provider:             DefaultComputerUseProvider(provider),
		anthropicToolVersion: anthropicToolVersion,
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
// anthropicToolVersion is the tool version declared to the model.
func ComputerUseProviderTool(
	provider codersdk.ChatComputerUseProvider,
	anthropicToolVersion fantasyanthropic.ComputerUseToolVersion,
	declaredWidth, declaredHeight int,
) (fantasy.Tool, error) {
	switch DefaultComputerUseProvider(provider) {
	case codersdk.ChatComputerUseProviderAnthropic:
		opts := fantasyanthropic.ComputerUseToolOptions{ToolVersion: anthropicToolVersion}
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
	case codersdk.ChatComputerUseProviderOpenAICompat:
		return openaiCompatComputerUseTool(declaredWidth, declaredHeight), nil
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

// HaltedToolCallError halts the later computer calls of a step after one
// fails, for the flavors that declare one action per call. The model batches
// actions that assume the earlier ones succeeded, so running them could act on
// the wrong screen. The hosted OpenAI flavor is excluded because it sends an
// array of actions in one call rather than one action per call.
func (t *computerUseTool) HaltedToolCallError() (string, bool) {
	switch DefaultComputerUseProvider(t.provider) {
	case codersdk.ChatComputerUseProviderAnthropic, codersdk.ChatComputerUseProviderOpenAICompat:
		return computerUseHaltedActionText, true
	default:
		return "", false
	}
}

func (t *computerUseTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	switch DefaultComputerUseProvider(t.provider) {
	case codersdk.ChatComputerUseProviderAnthropic, codersdk.ChatComputerUseProviderOpenAICompat:
		return t.runActionPerCallComputerUse(ctx, call)
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

// computerUseAnswersWithText reports whether a performed action answers with the
// desktop output as text instead of a fresh screenshot. Only the newest Anthropic
// toolset does, and the model asks for a screenshot when it needs one. The older
// Anthropic toolsets and the chat completions function tool return no screenshot
// with an action result, and an unverified model cannot be relied on to ask for
// one, so every action answers with a screenshot, as those descriptions state.
func (t *computerUseTool) computerUseAnswersWithText() bool {
	return DefaultComputerUseProvider(t.provider) == codersdk.ChatComputerUseProviderAnthropic &&
		t.anthropicToolVersion == fantasyanthropic.ComputerUseToolset20260801
}

// runActionPerCallComputerUse runs one computer action delivered as a single
// action per call, the shape both the Anthropic computer toolsets and the chat
// completions function tool declare. computerUseAnswersWithText decides whether
// a performed action answers with text or with a fresh screenshot.
func (t *computerUseTool) runActionPerCallComputerUse(
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

	switch input.Action {
	case fantasyanthropic.ActionScreenshot:
		return t.captureSharedScreenshot(ctx, conn, declaredWidth, declaredHeight)
	case fantasyanthropic.ActionWait:
		t.wait(ctx, anthropicDurationMillis(input.Duration))
		if t.computerUseAnswersWithText() {
			return fantasy.NewTextResponse("wait action performed"), nil
		}
		return t.captureScreenshot(ctx, conn, declaredWidth, declaredHeight)
	}

	// Build the action request.
	action := t.desktopAction(string(input.Action), declaredWidth, declaredHeight)
	coordinateSent, startCoordinateSent := computerUseCoordinateSent(call.Input)
	if coordinateSent {
		coord := coordinateFromInt64(input.Coordinate[0], input.Coordinate[1])
		action.Coordinate = &coord
	}
	if startCoordinateSent {
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

	if t.computerUseAnswersWithText() {
		// The agent reports cursor_position in declared coordinates, which
		// are the toolset's screenshot coordinates.
		output := actionResp.Output
		if output == "" {
			output = fmt.Sprintf("%s action performed", input.Action)
		}
		return fantasy.NewTextResponse(output), nil
	}
	return t.captureScreenshot(ctx, conn, declaredWidth, declaredHeight)
}

// computerUseCoordinateSent reports which coordinate keys the model sent.
// ComputerUseInput decodes coordinates into fixed arrays, so a zero value
// cannot tell an omitted coordinate from the top-left pixel [0,0], which is a
// valid target. ParseComputerUseInput already rejected malformed input, so a
// decode failure here only means the keys are absent.
func computerUseCoordinateSent(input string) (coordinate, startCoordinate bool) {
	var sent struct {
		Coordinate      *json.RawMessage `json:"coordinate"`
		StartCoordinate *json.RawMessage `json:"start_coordinate"`
	}
	if err := json.Unmarshal([]byte(input), &sent); err != nil {
		return false, false
	}
	return sent.Coordinate != nil, sent.StartCoordinate != nil
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
