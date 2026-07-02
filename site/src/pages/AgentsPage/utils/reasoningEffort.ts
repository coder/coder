/**
 * Reasoning effort helpers for the per-turn effort selector.
 *
 * Mirrors the backend's global effort scale and per-provider runtime
 * support in coderd/x/chatd/chatprovider/reasoningeffort.go. The
 * backend clamps whatever the client sends; these helpers exist so
 * the UI only offers values the model can actually use.
 */

/**
 * Global effort ordering used for clamping and slider positions.
 * Each provider supports a contiguous subset.
 */
export const reasoningEffortOrder = [
	"none",
	"minimal",
	"low",
	"medium",
	"high",
	"xhigh",
	"max",
] as const;

// Runtime-supported effort values per provider, in ascending global
// order. Azure shares OpenAI's set and Bedrock shares Anthropic's.
const supportedEffortsByProvider: Record<string, readonly string[]> = {
	openai: ["minimal", "low", "medium", "high", "xhigh"],
	azure: ["minimal", "low", "medium", "high", "xhigh"],
	openaicompat: ["minimal", "low", "medium", "high", "xhigh"],
	anthropic: ["low", "medium", "high", "xhigh", "max"],
	bedrock: ["low", "medium", "high", "xhigh", "max"],
	openrouter: ["low", "medium", "high"],
	vercel: ["none", "minimal", "low", "medium", "high", "xhigh"],
};

/**
 * The effort-relevant slice of a model option: the provider decides
 * which values are supported and the config's max/default bound the
 * selectable range.
 */
interface ReasoningEffortModel {
	readonly provider: string;
	readonly reasoningEffortDefault?: string;
	readonly reasoningEffortMax?: string;
}

/**
 * Position of value on the global effort scale, or -1 when the value
 * is not a known effort.
 */
export const reasoningEffortRank = (value: string): number =>
	reasoningEffortOrder.indexOf(
		value.trim().toLowerCase() as (typeof reasoningEffortOrder)[number],
	);

const normalizeEffort = (value: string | undefined): string | undefined => {
	const normalized = value?.trim().toLowerCase();
	return normalized && reasoningEffortRank(normalized) >= 0
		? normalized
		: undefined;
};

/** Display label for an effort value, e.g. "xhigh" renders as "Xhigh". */
export const formatReasoningEffort = (value: string): string => {
	const normalized = value.trim().toLowerCase();
	return normalized.charAt(0).toUpperCase() + normalized.slice(1);
};

/**
 * Effort values supported by the provider's runtime, in ascending
 * global order. Empty for providers without reasoning effort support.
 */
export const getSupportedReasoningEfforts = (
	provider: string,
): readonly string[] =>
	supportedEffortsByProvider[provider.trim().toLowerCase()] ?? [];

/**
 * Effort values the user may select for a model: the provider's
 * supported values from the provider minimum up to the model's
 * configured max. Empty when the model has no max configured or the
 * provider does not support reasoning effort. A max below the provider
 * minimum leaves only the minimum selectable, mirroring the backend's
 * clamp-up behavior.
 */
export const getSelectableReasoningEfforts = (
	model: ReasoningEffortModel,
): readonly string[] => {
	const supported = getSupportedReasoningEfforts(model.provider);
	const max = normalizeEffort(model.reasoningEffortMax);
	if (supported.length === 0 || !max) {
		return [];
	}
	const maxRank = reasoningEffortRank(max);
	const selectable = supported.filter(
		(effort) => reasoningEffortRank(effort) <= maxRank,
	);
	return selectable.length > 0 ? selectable : supported.slice(0, 1);
};

/**
 * Resolve value to an effort valid for the model. Keeps value when
 * the model supports it under its max, otherwise falls back to the
 * model's default (snapped into the selectable range). Returns
 * undefined when the model has no reasoning effort configured or no
 * usable value remains.
 */
export const clampReasoningEffort = (
	value: string | undefined,
	model: ReasoningEffortModel,
): string | undefined => {
	const selectable = getSelectableReasoningEfforts(model);
	if (selectable.length === 0) {
		return undefined;
	}

	const normalized = normalizeEffort(value);
	if (normalized && selectable.includes(normalized)) {
		return normalized;
	}

	const defaultEffort = normalizeEffort(model.reasoningEffortDefault);
	if (!defaultEffort) {
		return undefined;
	}
	if (selectable.includes(defaultEffort)) {
		return defaultEffort;
	}

	// Default outside the selectable range: snap to the largest
	// selectable value not exceeding it, or the minimum when below.
	const defaultRank = reasoningEffortRank(defaultEffort);
	let snapped = selectable[0];
	for (const candidate of selectable) {
		if (reasoningEffortRank(candidate) > defaultRank) {
			break;
		}
		snapped = candidate;
	}
	return snapped;
};
