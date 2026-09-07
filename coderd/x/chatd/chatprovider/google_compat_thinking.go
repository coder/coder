package chatprovider

import (
	"slices"
	"strconv"
	"strings"

	fantasygoogle "charm.land/fantasy/providers/google"

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
	normalized, supported, capable := googleCompatThinkingSupport(modelID)
	if !capable {
		return false
	}

	thinkingConfig := map[string]any{"include_thoughts": true}
	if effort, ok := payload["reasoning_effort"].(string); ok {
		if len(supported) > 0 {
			level := clampGoogleThinkingLevel(googleThinkingLevel(effort), supported)
			thinkingConfig["thinking_level"] = strings.ToLower(level)
		} else if budget, ok := googleCompatThinkingBudget(normalized, effort); ok {
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
	// Merge with a config-pinned thinking_config using the same precedence
	// as the native Google path: a pinned thinking_budget wins over the
	// per-turn effort, while the effort overrides a pinned thinking_level.
	// reasoning_effort must go in every case because Google rejects it in
	// combination with thinking_config.
	if pinned, ok := google["thinking_config"].(map[string]any); ok {
		if _, hasBudget := pinned["thinking_budget"]; !hasBudget {
			if level, ok := thinkingConfig["thinking_level"]; ok {
				pinned["thinking_level"] = level
			} else if budget, ok := thinkingConfig["thinking_budget"]; ok {
				pinned["thinking_budget"] = budget
			}
		}
	} else {
		google["thinking_config"] = thinkingConfig
	}
	delete(payload, "reasoning_effort")
	return true
}

// googleCompatThinkingSupport reports whether a model ID on the
// OpenAI-compatible path is a thinking-capable Gemini model, returning the
// normalized model ID and its supported thinking levels (empty for the
// budget-based 2.5 families).
func googleCompatThinkingSupport(modelID string) (string, []fantasygoogle.ThinkingLevel, bool) {
	normalized := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(modelID)), "models/")
	normalized = strings.TrimPrefix(normalized, "google/")
	if !strings.HasPrefix(normalized, "gemini-") {
		return "", nil, false
	}
	supported := googleSupportedThinkingLevels(normalized)
	if len(supported) == 0 && !googleSupportsThinkingBudget(normalized) {
		return "", nil, false
	}
	return normalized, supported, true
}

// googleCompatExtraBodyFromThinkingConfig translates a config-pinned Google
// thinking configuration into the extra_body payload for Gemini models routed
// through the OpenAI-compatible client, which ignores the native Google
// provider options. include_thoughts defaults to enabled so pinned
// configurations still surface thinking in the chat UI. Returns nil when the
// model has no thinking support.
func googleCompatExtraBodyFromThinkingConfig(
	modelID string,
	config *codersdk.ChatModelGoogleThinkingConfig,
) map[string]any {
	if config == nil {
		return nil
	}
	_, supported, capable := googleCompatThinkingSupport(modelID)
	if !capable {
		return nil
	}

	includeThoughts := true
	if config.IncludeThoughts != nil {
		includeThoughts = *config.IncludeThoughts
	}
	thinkingConfig := map[string]any{"include_thoughts": includeThoughts}
	if config.ThinkingBudget != nil {
		thinkingConfig["thinking_budget"] = *config.ThinkingBudget
	} else if pinned := GoogleThinkingLevelFromChat(config.ThinkingLevel); pinned != nil && len(supported) > 0 {
		level := clampGoogleThinkingLevel(*pinned, supported)
		thinkingConfig["thinking_level"] = strings.ToLower(level)
	}
	return map[string]any{
		"extra_body": map[string]any{
			"google": map[string]any{"thinking_config": thinkingConfig},
		},
	}
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
// when reasoning_effort is replaced by an explicit thinking_config. Pro
// models cannot disable thinking (budget 0 is rejected: "This model only
// works in thinking mode"), so "none" clamps up to the low budget for them.
func googleCompatThinkingBudget(modelID string, effort string) (int, bool) {
	switch effort {
	case codersdk.ChatModelReasoningEffortNone:
		if slices.Contains(strings.Split(strings.TrimPrefix(modelID, "gemini-"), "-"), "pro") {
			return 1024, true
		}
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
