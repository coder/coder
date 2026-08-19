package chatprovider

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyazure "charm.land/fantasy/providers/azure"
	fantasybedrock "charm.land/fantasy/providers/bedrock"
	fantasygoogle "charm.land/fantasy/providers/google"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"

	"github.com/coder/coder/v2/codersdk"
)

func reasoningEffortRank(value string) (int, bool) {
	rank := slices.Index(codersdk.ChatModelReasoningEffortValues(), value)
	return rank, rank >= 0
}

func IsValidReasoningEffort(value string) bool {
	_, ok := reasoningEffortRank(value)
	return ok
}

// ReasoningEffortLessOrEqual reports whether a is lower than or equal
// to b on the global effort scale. Unknown values return false.
func ReasoningEffortLessOrEqual(a, b string) bool {
	aRank, aOK := reasoningEffortRank(a)
	bRank, bOK := reasoningEffortRank(b)
	return aOK && bOK && aRank <= bRank
}

// ResolveReasoningEffort computes the effective reasoning effort for a
// generation. The requested per-turn value wins over the config's default,
// and the result is clamped to the config's max on the global scale. Returns
// nil when the model config has no reasoning effort configured, no usable
// value remains, or the max is unknown.
func ResolveReasoningEffort(
	requested *string,
	config *codersdk.ChatModelReasoningEffortConfig,
) *string {
	if config == nil {
		return nil
	}

	effective := requested
	var rank int
	var ok bool
	if effective != nil {
		rank, ok = reasoningEffortRank(*effective)
	}
	if !ok {
		effective = config.Default
		if effective != nil {
			rank, ok = reasoningEffortRank(*effective)
		}
	}
	if !ok {
		return nil
	}
	if config.Max != nil {
		maxRank, ok := reasoningEffortRank(*config.Max)
		if !ok {
			return nil
		}
		if rank > maxRank {
			return config.Max
		}
	}
	return effective
}

func SelectableReasoningEfforts(
	config *codersdk.ChatModelReasoningEffortConfig,
) []string {
	if config == nil || config.Max == nil {
		return nil
	}
	maxRank, ok := reasoningEffortRank(*config.Max)
	if !ok {
		return nil
	}
	values := codersdk.ChatModelReasoningEffortValues()
	return values[:maxRank+1]
}

func applyReasoningEffort(
	model Model,
	options fantasy.ProviderOptions,
	effort *string,
) fantasy.ProviderOptions {
	if effort == nil || !model.Valid() {
		return options
	}
	if options == nil {
		options = fantasy.ProviderOptions{}
	}

	switch NormalizeProvider(model.Provider()) {
	case fantasyopenai.Name, fantasyazure.Name:
		providerEffort := fantasyopenai.ReasoningEffort(*effort)
		switch opts := options[fantasyopenai.Name].(type) {
		case *fantasyopenai.ResponsesProviderOptions:
			opts.ReasoningEffort = &providerEffort
		case *fantasyopenai.ProviderOptions:
			opts.ReasoningEffort = &providerEffort
		default:
			if model.transport.UsesResponses() {
				options[fantasyopenai.Name] = &fantasyopenai.ResponsesProviderOptions{
					ReasoningEffort: &providerEffort,
				}
				return options
			}
			options[fantasyopenai.Name] = &fantasyopenai.ProviderOptions{
				ReasoningEffort: &providerEffort,
			}
		}
	case fantasyanthropic.Name, fantasybedrock.Name:
		providerEffort := fantasyanthropic.Effort(*effort)
		providerOptions := ensureProviderOptions[fantasyanthropic.ProviderOptions](options, fantasyanthropic.Name)
		providerOptions.Effort = &providerEffort
	case fantasygoogle.Name:
		// Only Gemini 3+ accepts thinking_level; older generations reject
		// requests carrying it, so keep dropping the effort for them.
		supported := googleSupportedThinkingLevels(model.ModelID())
		if len(supported) == 0 {
			return options
		}
		providerOptions := ensureProviderOptions[fantasygoogle.ProviderOptions](options, fantasygoogle.Name)
		if providerOptions.ThinkingConfig == nil {
			providerOptions.ThinkingConfig = &fantasygoogle.ThinkingConfig{}
		}
		// A configured thinking budget wins: fantasy rejects requests that
		// set both thinking_budget and thinking_level. The resolved effort
		// overrides a config-pinned thinking_level so the user's effort
		// selection stays meaningful.
		if providerOptions.ThinkingConfig.ThinkingBudget == nil {
			level := clampGoogleThinkingLevel(googleThinkingLevel(*effort), supported)
			providerOptions.ThinkingConfig.ThinkingLevel = &level
		}
		// Google returns thought summaries only when they are requested, so
		// default them on for reasoning-effort generations; an explicitly
		// configured include_thoughts (either value) is preserved.
		if providerOptions.ThinkingConfig.IncludeThoughts == nil {
			includeThoughts := true
			providerOptions.ThinkingConfig.IncludeThoughts = &includeThoughts
		}
	case fantasyopenaicompat.Name:
		providerEffort := fantasyopenai.ReasoningEffort(*effort)
		if compatEffort, ok := googleCompatReasoningEffort(model.ModelID(), *effort); ok {
			providerEffort = fantasyopenai.ReasoningEffort(compatEffort)
		}
		providerOptions := ensureProviderOptions[fantasyopenaicompat.ProviderOptions](options, fantasyopenaicompat.Name)
		providerOptions.ReasoningEffort = &providerEffort
	case fantasyopenrouter.Name:
		providerEffort := fantasyopenrouter.ReasoningEffort(*effort)
		providerOptions := ensureProviderOptions[fantasyopenrouter.ProviderOptions](options, fantasyopenrouter.Name)
		if providerOptions.Reasoning == nil {
			providerOptions.Reasoning = &fantasyopenrouter.ReasoningOptions{}
		}
		providerOptions.Reasoning.Effort = &providerEffort
	case fantasyvercel.Name:
		providerEffort := fantasyvercel.ReasoningEffort(*effort)
		providerOptions := ensureProviderOptions[fantasyvercel.ProviderOptions](options, fantasyvercel.Name)
		if providerOptions.Reasoning == nil {
			providerOptions.Reasoning = &fantasyvercel.ReasoningOptions{}
		}
		providerOptions.Reasoning.Effort = &providerEffort
	}
	return options
}

// googleCompatReasoningEffort maps the global reasoning effort scale onto the
// reasoning_effort values a Gemini model accepts behind an OpenAI-compatible
// endpoint, reporting ok=false for non-Gemini model IDs. Google's compat
// layer translates reasoning_effort into the model's thinking configuration
// but validates instead of clamping, so out-of-range values (including the
// Coder-only xhigh and max) fail the whole request with HTTP 400.
func googleCompatReasoningEffort(modelID, effort string) (string, bool) {
	normalized := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(modelID)), "models/")
	normalized = strings.TrimPrefix(normalized, "google/")
	if !strings.HasPrefix(normalized, "gemini-") {
		return "", false
	}
	if supported := googleSupportedThinkingLevels(normalized); len(supported) > 0 {
		level := clampGoogleThinkingLevel(googleThinkingLevel(effort), supported)
		return strings.ToLower(level), true
	}
	// Pre-Gemini-3 models translate reasoning_effort into a thinking budget
	// and accept only none/low/medium/high. Pro models cannot disable
	// thinking, so "none" is rejected for them and clamps up to low.
	isPro := slices.Contains(strings.Split(strings.TrimPrefix(normalized, "gemini-"), "-"), "pro")
	switch effort {
	case codersdk.ChatModelReasoningEffortNone:
		if isPro {
			return codersdk.ChatModelReasoningEffortLow, true
		}
		return codersdk.ChatModelReasoningEffortNone, true
	case codersdk.ChatModelReasoningEffortMinimal, codersdk.ChatModelReasoningEffortLow:
		return codersdk.ChatModelReasoningEffortLow, true
	case codersdk.ChatModelReasoningEffortMedium:
		return codersdk.ChatModelReasoningEffortMedium, true
	default:
		return codersdk.ChatModelReasoningEffortHigh, true
	}
}

// googleThinkingLevelsAscending orders Google thinking levels from least to
// most thinking, for clamping into a model's supported subset.
var googleThinkingLevelsAscending = []fantasygoogle.ThinkingLevel{
	fantasygoogle.ThinkingLevelMinimal,
	fantasygoogle.ThinkingLevelLow,
	fantasygoogle.ThinkingLevelMedium,
	fantasygoogle.ThinkingLevelHigh,
}

// googleSupportedThinkingLevels returns the thinking_level values the Google
// model accepts in ascending order, or nil when the model does not accept
// thinking_level at all. Gemini introduced thinking_level in version 3, and
// each model supports a different subset: Gemini 3 Pro launched with LOW and
// HIGH, 3.1 Pro added MEDIUM, the Flash family accepts all four, and image
// models accept only HIGH (plus MINIMAL for flash). Versionless "-latest"
// aliases track the newest release of their family, which has been Gemini 3+
// since the aliases were introduced. Non-Gemini and unrecognized model IDs
// return nil so reasoning effort degrades to a no-op instead of a rejected
// request.
func googleSupportedThinkingLevels(modelID string) []fantasygoogle.ThinkingLevel {
	normalized := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(modelID)), "models/")
	rest, ok := strings.CutPrefix(normalized, "gemini-")
	if !ok {
		return nil
	}
	segments := strings.Split(rest, "-")

	major, minor, hasVersion := parseGoogleModelVersion(segments[0])
	isLatestAlias := !hasVersion && segments[len(segments)-1] == "latest"
	if hasVersion && major < 3 {
		return nil
	}
	if !hasVersion && !isLatestAlias {
		return nil
	}

	isPro := slices.Contains(segments, "pro")
	isFlash := slices.Contains(segments, "flash")
	isImage := slices.Contains(segments, "image")

	switch {
	case isImage && isFlash:
		return []fantasygoogle.ThinkingLevel{
			fantasygoogle.ThinkingLevelMinimal,
			fantasygoogle.ThinkingLevelHigh,
		}
	case isImage:
		return []fantasygoogle.ThinkingLevel{fantasygoogle.ThinkingLevelHigh}
	case isFlash:
		return slices.Clone(googleThinkingLevelsAscending)
	case isPro && hasVersion && major == 3 && minor == 0:
		return []fantasygoogle.ThinkingLevel{
			fantasygoogle.ThinkingLevelLow,
			fantasygoogle.ThinkingLevelHigh,
		}
	case isPro:
		return []fantasygoogle.ThinkingLevel{
			fantasygoogle.ThinkingLevelLow,
			fantasygoogle.ThinkingLevelMedium,
			fantasygoogle.ThinkingLevelHigh,
		}
	default:
		// Unknown Gemini 3+ variant: LOW and HIGH are the intersection of
		// every documented non-image model's supported set.
		return []fantasygoogle.ThinkingLevel{
			fantasygoogle.ThinkingLevelLow,
			fantasygoogle.ThinkingLevelHigh,
		}
	}
}

// parseGoogleModelVersion parses a Gemini version segment such as "3" or
// "3.7" into major and minor components.
func parseGoogleModelVersion(segment string) (major, minor int, ok bool) {
	majorText, minorText, hasMinor := strings.Cut(segment, ".")
	major, err := strconv.Atoi(majorText)
	if err != nil {
		return 0, 0, false
	}
	if hasMinor {
		minor, err = strconv.Atoi(minorText)
		if err != nil {
			return 0, 0, false
		}
	}
	return major, minor, true
}

// clampGoogleThinkingLevel snaps the desired level into the model's supported
// subset: the lowest supported level at or above the desired one, so at least
// the requested reasoning depth is preserved, else the highest supported.
func clampGoogleThinkingLevel(
	desired fantasygoogle.ThinkingLevel,
	supported []fantasygoogle.ThinkingLevel,
) fantasygoogle.ThinkingLevel {
	desiredRank := slices.Index(googleThinkingLevelsAscending, desired)
	for _, candidate := range supported {
		if slices.Index(googleThinkingLevelsAscending, candidate) >= desiredRank {
			return candidate
		}
	}
	return supported[len(supported)-1]
}

// googleThinkingLevel maps the global reasoning effort scale to Google
// thinking levels. Google offers no way to disable thinking on Gemini 3+
// models and no levels above HIGH, so the scale clamps at both ends.
func googleThinkingLevel(effort string) fantasygoogle.ThinkingLevel {
	switch effort {
	case codersdk.ChatModelReasoningEffortNone, codersdk.ChatModelReasoningEffortMinimal:
		return fantasygoogle.ThinkingLevelMinimal
	case codersdk.ChatModelReasoningEffortLow:
		return fantasygoogle.ThinkingLevelLow
	case codersdk.ChatModelReasoningEffortMedium:
		return fantasygoogle.ThinkingLevelMedium
	default:
		return fantasygoogle.ThinkingLevelHigh
	}
}

func ensureProviderOptions[T any, PT interface {
	*T
	fantasy.ProviderOptionsData
}](options fantasy.ProviderOptions, name string) PT {
	providerOptions, _ := options[name].(PT)
	if providerOptions == nil {
		providerOptions = PT(new(T))
		options[name] = providerOptions
	}
	return providerOptions
}
