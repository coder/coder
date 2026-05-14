package computeruse

import (
	"slices"
	"strings"
	"unicode"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// ComputerUseTool returns the OpenAI provider-defined computer-use tool.
func Tool() fantasy.Tool {
	return fantasyopenai.NewComputerUseTool(nil).Definition()
}

// IsComputerUseTool reports whether tool is the OpenAI provider-defined
// computer-use tool.
func IsTool(tool fantasy.Tool) bool {
	return fantasyopenai.IsComputerUseTool(tool)
}

// ParseInput parses an OpenAI computer-use tool call input.
func ParseInput(input string) (*fantasyopenai.ComputerUseInput, error) {
	return fantasyopenai.ParseComputerUseInput(input)
}

// ComputerUseResultProviderMetadata returns metadata that should accompany an
// OpenAI computer-use screenshot result.
func ResultProviderMetadata(response fantasy.ToolResponse) fantasy.ProviderMetadata {
	if response.IsError || response.Type != "image" || len(response.Data) == 0 ||
		!strings.HasPrefix(response.MediaType, "image/") {
		return nil
	}

	return fantasy.ProviderMetadata{
		fantasyopenai.Name: &fantasyopenai.ComputerCallOutputOptions{
			Detail: "original",
		},
	}
}

// OpenAI scroll deltas are pixels, but Coder desktop scroll amounts are
// wheel clicks.
const computerUseScrollPixelsPerWheelClick int64 = 100

// ComputerUseDesktopAction is a Coder desktop operation requested by an
// OpenAI computer-use tool call.
type DesktopAction struct {
	Action                workspacesdk.DesktopAction
	WaitDurationMillis    int64
	ReleaseMouseOnFailure bool
	ReleaseKeysOnFailure  []string
}

// ComputerUseDesktopActions converts an OpenAI computer-use tool call into
// Coder desktop actions. A caller should execute the returned actions in order,
// wait for WaitDurationMillis entries, and then return a final screenshot.
func DesktopActions(
	parsed *fantasyopenai.ComputerUseInput,
	declaredWidth, declaredHeight int,
) ([]DesktopAction, error) {
	if parsed == nil {
		return nil, xerrors.New("OpenAI computer use input is nil")
	}
	var err error
	actions := make([]DesktopAction, 0, len(parsed.Actions))
	for _, action := range parsed.Actions {
		switch action.Type {
		case "screenshot":
			// OpenAI returns one screenshot per response; individual screenshot
			// actions in the batch are fulfilled by the batch-final capture.
			continue
		case "move":
			actions = append(actions, DesktopAction{
				Action: desktopActionWithCoordinate(
					"mouse_move",
					declaredWidth,
					declaredHeight,
					action.X,
					action.Y,
				),
			})
		case "click":
			actionSet, err := clickActions(
				action.Button,
				declaredWidth,
				declaredHeight,
				action.X,
				action.Y,
			)
			if err != nil {
				return nil, err
			}
			actions, err = appendWithModifiers(actions, action.Keys, actionSet)
			if err != nil {
				return nil, err
			}
		case "double_click":
			actionName, ok := DoubleClickAction(action.Button)
			if !ok {
				return nil, xerrors.Errorf(
					"unsupported OpenAI double-click button %q",
					action.Button,
				)
			}
			actionSet := []DesktopAction{{
				Action: desktopActionWithCoordinate(
					actionName,
					declaredWidth,
					declaredHeight,
					action.X,
					action.Y,
				),
			}}
			actions, err = appendWithModifiers(actions, action.Keys, actionSet)
			if err != nil {
				return nil, err
			}
		case "drag":
			if len(action.Path) < 2 {
				return nil, xerrors.New("OpenAI drag action requires at least two path points")
			}
			actionSet := []DesktopAction{
				{
					Action: desktopActionWithCoordinate(
						"mouse_move",
						declaredWidth,
						declaredHeight,
						action.Path[0].X,
						action.Path[0].Y,
					),
				},
				{
					Action: desktopAction(
						"left_mouse_down",
						declaredWidth,
						declaredHeight,
					),
					ReleaseMouseOnFailure: true,
				},
			}
			for _, point := range action.Path[1:] {
				actionSet = append(actionSet, DesktopAction{
					Action: desktopActionWithCoordinate(
						"mouse_move",
						declaredWidth,
						declaredHeight,
						point.X,
						point.Y,
					),
					ReleaseMouseOnFailure: true,
				})
			}
			actionSet = append(actionSet, DesktopAction{
				Action: desktopAction(
					"left_mouse_up",
					declaredWidth,
					declaredHeight,
				),
				ReleaseMouseOnFailure: true,
			})
			actions, err = appendWithModifiers(actions, action.Keys, actionSet)
			if err != nil {
				return nil, err
			}
		case "keypress":
			text, err := NormalizeKeys(action.Keys)
			if err != nil {
				return nil, err
			}
			desktopAction := desktopAction("key", declaredWidth, declaredHeight)
			desktopAction.Text = &text
			actions = append(actions, DesktopAction{Action: desktopAction})
		case "type":
			desktopAction := desktopAction("type", declaredWidth, declaredHeight)
			desktopAction.Text = &action.Text
			actions = append(actions, DesktopAction{Action: desktopAction})
		case "scroll":
			actionSet := computerUseScrollActions(
				declaredWidth,
				declaredHeight,
				action.X,
				action.Y,
				action.ScrollX,
				action.ScrollY,
			)
			actions, err = appendWithModifiers(actions, action.Keys, actionSet)
			if err != nil {
				return nil, err
			}
		case "wait":
			actions = append(actions, DesktopAction{WaitDurationMillis: 1000})
		default:
			return nil, xerrors.Errorf(
				"unsupported OpenAI computer action type %q",
				action.Type,
			)
		}
	}
	return actions, nil
}

func appendWithModifiers(
	actions []DesktopAction,
	keys []string,
	actionSet []DesktopAction,
) ([]DesktopAction, error) {
	if len(keys) == 0 {
		return append(actions, actionSet...), nil
	}

	modifiers := make([]string, 0, len(keys))
	for _, key := range keys {
		modifier, err := normalizeComputerUseKey(key)
		if err != nil {
			return nil, err
		}
		modifiers = append(modifiers, modifier)
	}

	heldKeys := make([]string, 0, len(modifiers))
	for _, modifier := range modifiers {
		nextHeldKeys := append(slices.Clone(heldKeys), modifier)
		desktopAction := desktopAction("key_down", 0, 0)
		desktopAction.Text = &modifier
		actions = append(actions, DesktopAction{
			Action:               desktopAction,
			ReleaseKeysOnFailure: nextHeldKeys,
		})
		heldKeys = nextHeldKeys
	}

	for _, action := range actionSet {
		action.ReleaseKeysOnFailure = slices.Clone(heldKeys)
		actions = append(actions, action)
	}

	for i := len(heldKeys) - 1; i >= 0; i-- {
		key := heldKeys[i]
		desktopAction := desktopAction("key_up", 0, 0)
		desktopAction.Text = &key
		actions = append(actions, DesktopAction{
			Action:               desktopAction,
			ReleaseKeysOnFailure: slices.Clone(heldKeys[:i+1]),
		})
	}
	return actions, nil
}

func computerUseScrollActions(
	declaredWidth, declaredHeight int,
	x, y, scrollX, scrollY int64,
) []DesktopAction {
	coord := coordinateFromInt64(x, y)
	moveAction := desktopAction("mouse_move", declaredWidth, declaredHeight)
	moveAction.Coordinate = &coord
	actions := []DesktopAction{{Action: moveAction}}

	if scrollY != 0 {
		direction := "down"
		if scrollY < 0 {
			direction = "up"
		}
		scrollAction := desktopAction("scroll", declaredWidth, declaredHeight)
		scrollAction.Coordinate = &coord
		scrollAction.ScrollDirection = &direction
		amount := scrollPixelsToWheelClicks(scrollY)
		scrollAction.ScrollAmount = &amount
		actions = append(actions, DesktopAction{Action: scrollAction})
	}

	if scrollX != 0 {
		direction := "right"
		if scrollX < 0 {
			direction = "left"
		}
		scrollAction := desktopAction("scroll", declaredWidth, declaredHeight)
		scrollAction.Coordinate = &coord
		scrollAction.ScrollDirection = &direction
		amount := scrollPixelsToWheelClicks(scrollX)
		scrollAction.ScrollAmount = &amount
		actions = append(actions, DesktopAction{Action: scrollAction})
	}
	return actions
}

func desktopActionWithCoordinate(
	action string,
	declaredWidth, declaredHeight int,
	x, y int64,
) workspacesdk.DesktopAction {
	desktopAction := desktopAction(action, declaredWidth, declaredHeight)
	coord := coordinateFromInt64(x, y)
	desktopAction.Coordinate = &coord
	return desktopAction
}

func desktopAction(
	action string,
	declaredWidth, declaredHeight int,
) workspacesdk.DesktopAction {
	return workspacesdk.DesktopAction{
		Action:       action,
		ScaledWidth:  &declaredWidth,
		ScaledHeight: &declaredHeight,
	}
}

func coordinateFromInt64(x, y int64) [2]int {
	return [2]int{int(x), int(y)}
}

func scrollPixelsToWheelClicks(pixels int64) int {
	if pixels < 0 {
		pixels = -pixels
	}
	if pixels == 0 {
		return 0
	}
	return int((pixels + computerUseScrollPixelsPerWheelClick - 1) /
		computerUseScrollPixelsPerWheelClick)
}

func clickActions(
	button string,
	declaredWidth, declaredHeight int,
	x, y int64,
) ([]DesktopAction, error) {
	actionName, ok := ClickAction(button)
	if ok {
		return []DesktopAction{{
			Action: desktopActionWithCoordinate(
				actionName,
				declaredWidth,
				declaredHeight,
				x,
				y,
			),
		}}, nil
	}

	navigationKey := ""
	switch button {
	case "back":
		navigationKey = "alt+Left"
	case "forward":
		navigationKey = "alt+Right"
	default:
		return nil, xerrors.Errorf("unsupported OpenAI click button %q", button)
	}

	keyAction := desktopAction("key", 0, 0)
	keyAction.Text = &navigationKey
	return []DesktopAction{
		{
			Action: desktopActionWithCoordinate(
				"mouse_move",
				declaredWidth,
				declaredHeight,
				x,
				y,
			),
		},
		{Action: keyAction},
	}, nil
}

// DoubleClickAction maps an OpenAI computer-use double-click button to a Coder
// desktop action name. The desktop API currently supports only left-button
// double-clicks.
func DoubleClickAction(button string) (string, bool) {
	switch button {
	case "", "left":
		return "double_click", true
	default:
		return "", false
	}
}

// ComputerUseClickAction maps an OpenAI computer-use click button to a Coder
// desktop action name.
func ClickAction(button string) (string, bool) {
	switch button {
	case "", "left":
		return "left_click", true
	case "right":
		return "right_click", true
	case "middle", "wheel":
		return "middle_click", true
	default:
		return "", false
	}
}

// NormalizeComputerUseKeys maps OpenAI keypress tokens to Coder desktop key
// action tokens.
func NormalizeKeys(keys []string) (string, error) {
	if len(keys) == 0 {
		return "", xerrors.New("OpenAI keypress action requires at least one key")
	}
	normalized := make([]string, 0, len(keys))
	for _, key := range keys {
		normalizedKey, err := normalizeComputerUseKey(key)
		if err != nil {
			return "", err
		}
		normalized = append(normalized, normalizedKey)
	}
	return strings.Join(normalized, "+"), nil
}

func normalizeComputerUseKey(key string) (string, error) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return "", xerrors.New("OpenAI keypress action contains an empty key")
	}

	lower := strings.ToLower(trimmed)
	switch lower {
	case "ctrl", "control":
		return "ctrl", nil
	case "cmd", "command", "meta", "super":
		return "meta", nil
	case "shift":
		return "shift", nil
	case "alt", "option":
		return "alt", nil
	case "enter", "return":
		return "Return", nil
	case "escape", "esc":
		return "Escape", nil
	case "tab":
		return "Tab", nil
	case "space":
		return "space", nil
	case "backspace":
		return "BackSpace", nil
	case "delete", "del":
		return "Delete", nil
	case "arrowup", "up":
		return "Up", nil
	case "arrowdown", "down":
		return "Down", nil
	case "arrowleft", "left":
		return "Left", nil
	case "arrowright", "right":
		return "Right", nil
	}

	if isFunctionKey(lower) {
		return "F" + lower[1:], nil
	}

	runes := []rune(trimmed)
	if len(runes) == 1 {
		r := runes[0]
		if unicode.IsLetter(r) {
			return strings.ToLower(trimmed), nil
		}
		if unicode.IsDigit(r) {
			return trimmed, nil
		}
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return trimmed, nil
		}
		return "", xerrors.Errorf("unsupported OpenAI keypress %q", trimmed)
	}

	return "", xerrors.Errorf("unsupported OpenAI keypress %q", trimmed)
}

func isFunctionKey(key string) bool {
	if len(key) < 2 || key[0] != 'f' {
		return false
	}
	number, ok := strings.CutPrefix(key, "f")
	if !ok || number == "" {
		return false
	}
	for _, r := range number {
		if r < '0' || r > '9' {
			return false
		}
	}
	value := 0
	for _, r := range number {
		value = value*10 + int(r-'0')
	}
	return value >= 1 && value <= 35
}
