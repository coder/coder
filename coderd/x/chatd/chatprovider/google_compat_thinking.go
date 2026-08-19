package chatprovider

import (
	"strconv"
	"strings"

	"github.com/coder/coder/v2/codersdk"
)

// rewriteGoogleCompatThinkingConfig swaps reasoning_effort for an explicit
// Google thinking_config on requests for thinking-capable Gemini models.
// Google's OpenAI-compatible endpoint rejects requests carrying both fields,
// and it never emits thought text unless include_thoughts is requested
// through thinking_config, so this is the only way to surface Gemini
// reasoning in the chat UI on this path. Models without known thinking
// support (pre-2.5 or unrecognized Gemini variants) keep their previous
// request shape untouched.
func rewriteGoogleCompatThinkingConfig(payload map[string]any) bool {
	modelID, _ := payload["model"].(string)
	normalized := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(modelID)), "models/")
	normalized = strings.TrimPrefix(normalized, "google/")
	if !strings.HasPrefix(normalized, "gemini-") {
		return false
	}

	supported := googleSupportedThinkingLevels(normalized)
	if len(supported) == 0 && !googleSupportsThinkingBudget(normalized) {
		return false
	}

	thinkingConfig := map[string]any{"include_thoughts": true}
	if effort, ok := payload["reasoning_effort"].(string); ok {
		if len(supported) > 0 {
			level := clampGoogleThinkingLevel(googleThinkingLevel(effort), supported)
			thinkingConfig["thinking_level"] = strings.ToLower(level)
		} else if budget, ok := googleCompatThinkingBudget(effort); ok {
			thinkingConfig["thinking_budget"] = budget
		} else {
			// Unknown effort value: keep the request untouched rather than
			// guessing a thinking budget.
			return false
		}
	}

	extraBody, _ := payload["extra_body"].(map[string]any)
	if extraBody == nil {
		extraBody = map[string]any{}
		payload["extra_body"] = extraBody
	}
	google, _ := extraBody["google"].(map[string]any)
	if google == nil {
		google = map[string]any{}
		extraBody["google"] = google
	}
	// An explicitly configured thinking_config wins, but reasoning_effort
	// must go regardless because Google rejects the combination.
	if _, exists := google["thinking_config"]; !exists {
		google["thinking_config"] = thinkingConfig
	}
	delete(payload, "reasoning_effort")
	return true
}

// googleSupportsThinkingBudget reports whether a Gemini model predating
// thinking_level thinks via thinking_budget. Only the Gemini 2.5 Pro, Flash,
// and Flash-Lite chat families qualify; specialized 2.5 variants such as
// image and TTS models reject thinking_config outright ("Thinking is not
// enabled for this model", verified live). Unrecognized name tokens fail
// closed so new specialized variants keep their previous request shape.
func googleSupportsThinkingBudget(normalized string) bool {
	rest, ok := strings.CutPrefix(normalized, "gemini-")
	if !ok {
		return false
	}
	segments := strings.Split(rest, "-")
	major, minor, hasVersion := parseGoogleModelVersion(segments[0])
	if !hasVersion || major != 2 || minor != 5 {
		return false
	}
	family := false
	for _, segment := range segments[1:] {
		switch segment {
		case "pro", "flash":
			family = true
		case "lite", "preview", "exp", "latest":
		default:
			if _, err := strconv.Atoi(segment); err != nil {
				return false
			}
		}
	}
	return family
}

// googleCompatThinkingBudget maps the global reasoning effort scale onto the
// thinking budgets Google's OpenAI-compatible endpoint uses when translating
// reasoning_effort for pre-Gemini-3 models, keeping effort semantics intact
// when reasoning_effort is replaced by an explicit thinking_config.
func googleCompatThinkingBudget(effort string) (int, bool) {
	switch effort {
	case codersdk.ChatModelReasoningEffortNone:
		return 0, true
	case codersdk.ChatModelReasoningEffortMinimal, codersdk.ChatModelReasoningEffortLow:
		return 1024, true
	case codersdk.ChatModelReasoningEffortMedium:
		return 8192, true
	case codersdk.ChatModelReasoningEffortHigh, codersdk.ChatModelReasoningEffortXHigh, codersdk.ChatModelReasoningEffortMax:
		return 24576, true
	default:
		return 0, false
	}
}
