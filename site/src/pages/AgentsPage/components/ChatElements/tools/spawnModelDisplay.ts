import type * as TypesGen from "#/api/typesGenerated";
import {
	ChatModelReasoningEffortHigh,
	ChatModelReasoningEffortLow,
	ChatModelReasoningEffortMax,
	ChatModelReasoningEffortMedium,
	ChatModelReasoningEffortMinimal,
	ChatModelReasoningEffortNone,
	ChatModelReasoningEffortXHigh,
} from "#/api/typesGenerated";

// Global reasoning effort scale, ordered low to high. Mirrors
// codersdk.ChatModelReasoningEffortValues().
const effortScale: readonly string[] = [
	ChatModelReasoningEffortNone,
	ChatModelReasoningEffortMinimal,
	ChatModelReasoningEffortLow,
	ChatModelReasoningEffortMedium,
	ChatModelReasoningEffortHigh,
	ChatModelReasoningEffortXHigh,
	ChatModelReasoningEffortMax,
];

const effortRank = (value: string | undefined): number =>
	value === undefined ? -1 : effortScale.indexOf(value.trim().toLowerCase());

export type SpawnModelDisplay = {
	modelLabel?: string;
	effortLabel?: string;
};

/**
 * Resolves the model display name and effective reasoning effort for a
 * spawn tool call that explicitly selected them. Effort resolution
 * mirrors the backend (chatprovider.ResolveReasoningEffort): the
 * requested value wins over the config's default and is clamped to the
 * config's max; models without effort settings run without thinking.
 * When the selected config is unknown (effort-only spawn or a config
 * removed since the spawn), the requested effort is shown unclamped.
 */
export const resolveSpawnModelDisplay = ({
	configs,
	modelConfigId,
	reasoningEffort,
}: {
	configs: readonly TypesGen.ChatModelConfig[] | undefined;
	modelConfigId?: string;
	reasoningEffort?: string;
}): SpawnModelDisplay => {
	const requestedRank = effortRank(reasoningEffort);
	const requested = requestedRank >= 0 ? effortScale[requestedRank] : undefined;
	const config = modelConfigId
		? configs?.find((c) => c.id === modelConfigId)
		: undefined;
	if (!config) {
		return requested && requested !== ChatModelReasoningEffortNone
			? { effortLabel: requested }
			: {};
	}

	const modelLabel = config.display_name.trim() || config.model.trim();
	const effortConfig = config.model_config?.reasoning_effort;
	if (!effortConfig) {
		return { modelLabel };
	}

	let effective = requested;
	if (effective === undefined) {
		const defaultRank = effortRank(effortConfig.default);
		effective = defaultRank >= 0 ? effortScale[defaultRank] : undefined;
	}
	if (effective === undefined) {
		return { modelLabel };
	}
	if (effortConfig.max !== undefined) {
		const maxRank = effortRank(effortConfig.max);
		if (maxRank < 0) {
			return { modelLabel };
		}
		if (effortRank(effective) > maxRank) {
			effective = effortScale[maxRank];
		}
	}
	if (effective === ChatModelReasoningEffortNone) {
		return { modelLabel };
	}
	return { modelLabel, effortLabel: effective };
};
