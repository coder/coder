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

export const resolveSpawnModelDisplay = ({
	models,
	modelId,
	reasoningEffort,
}: {
	models: readonly TypesGen.ChatModel[] | undefined;
	modelId?: string;
	reasoningEffort?: string;
}): SpawnModelDisplay => {
	const requestedRank = effortRank(reasoningEffort);
	const requested = requestedRank >= 0 ? effortScale[requestedRank] : undefined;
	const model = modelId
		? models?.find((model) => model.id === modelId)
		: undefined;
	if (!model) {
		return {};
	}

	const modelLabel = model.display_name.trim() || model.model.trim();
	const effortConfig = model.model_config?.reasoning_effort;
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
