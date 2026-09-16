import type * as TypesGen from "#/api/typesGenerated";
import {
	filterModelsWithEnabledProvider,
	type ProviderInfo,
} from "./utils/modelOptions";

export interface CompactionTrigger {
	readonly thresholdPercent: number;
	readonly contextLimit: number;
}

export interface OrganizationCompactionTrigger {
	readonly model: TypesGen.ChatModel;
	readonly trigger: CompactionTrigger;
	readonly point: number;
}

type CompactionThresholdSource = "user" | "model" | "organization";

export interface ResolvedCompactionThreshold {
	readonly percent: number;
	readonly source: CompactionThresholdSource;
	// Token count that triggers organization compaction. The gauge converts
	// this using its runtime context limit; percent uses the configured limit.
	readonly pointTokens?: number;
}

export const isCompactionTriggerEnabled = (trigger: CompactionTrigger) =>
	trigger.thresholdPercent >= 0 &&
	trigger.thresholdPercent < 100 &&
	trigger.contextLimit > 0;

export const compactionTriggerPoint = (trigger: CompactionTrigger) =>
	(trigger.contextLimit * trigger.thresholdPercent) / 100;

export const bindingCompactionTrigger = (
	chat: CompactionTrigger,
	override: CompactionTrigger,
): "chat" | "organization" => {
	if (!isCompactionTriggerEnabled(override)) {
		return "chat";
	}
	if (!isCompactionTriggerEnabled(chat)) {
		return "organization";
	}
	return compactionTriggerPoint(override) < compactionTriggerPoint(chat)
		? "organization"
		: "chat";
};

// Token count at which compaction fires, from whichever trigger binds, or
// undefined when neither is enabled.
export const bindingCompactionTriggerPoint = (
	chat: CompactionTrigger,
	organizationTrigger: OrganizationCompactionTrigger | undefined,
): number | undefined => {
	if (
		organizationTrigger &&
		bindingCompactionTrigger(chat, organizationTrigger.trigger) ===
			"organization"
	) {
		return organizationTrigger.point;
	}
	return isCompactionTriggerEnabled(chat)
		? compactionTriggerPoint(chat)
		: undefined;
};

export const resolveOrganizationCompactionTrigger = (
	overrides: readonly TypesGen.ChatModelOverrideResponse[] | undefined,
	models: readonly TypesGen.ChatModel[] | null | undefined,
	providerInfoByID: ReadonlyMap<string, ProviderInfo>,
): OrganizationCompactionTrigger | undefined => {
	const override = overrides?.find(
		(candidate) => candidate.context === "compaction",
	);
	const model = filterModelsWithEnabledProvider(
		models ?? [],
		providerInfoByID,
	).find((candidate) => candidate.id === override?.model_config_id);
	// Override models that are disabled or whose provider is disabled fall
	// back to the chat model on the backend.
	if (!model?.enabled) {
		return undefined;
	}

	const trigger = {
		thresholdPercent: model.compression_threshold,
		contextLimit: model.context_limit,
	};
	if (!isCompactionTriggerEnabled(trigger)) {
		return undefined;
	}

	return { model, trigger, point: compactionTriggerPoint(trigger) };
};

export const compactionPointAsPercent = (
	point: number,
	contextLimit: number,
): number | undefined =>
	contextLimit > 0 && Number.isFinite(point)
		? (point / contextLimit) * 100
		: undefined;

export const resolveCompactionThreshold = (
	modelID: string | undefined,
	userThresholds: readonly TypesGen.UserChatCompactionThreshold[] | undefined,
	models: readonly TypesGen.ChatModel[] | null | undefined,
	organizationTrigger: OrganizationCompactionTrigger | undefined,
): ResolvedCompactionThreshold | undefined => {
	if (!modelID || !Array.isArray(models)) {
		return undefined;
	}
	const config = models.find((candidate) => candidate.id === modelID);
	if (!config) {
		return undefined;
	}

	const userOverride = userThresholds?.find(
		(threshold) => threshold.model_config_id === modelID,
	);
	const thresholdPercent =
		userOverride?.threshold_percent ?? config.compression_threshold;
	const source = userOverride ? "user" : "model";
	if (organizationTrigger) {
		const organizationPercent = compactionPointAsPercent(
			organizationTrigger.point,
			config.context_limit,
		);
		// Bind before converting to a chat-window percentage because an
		// organization point may convert to 100% or more.
		if (
			organizationPercent !== undefined &&
			bindingCompactionTrigger(
				{
					thresholdPercent,
					contextLimit: config.context_limit,
				},
				organizationTrigger.trigger,
			) === "organization"
		) {
			return {
				percent: organizationPercent,
				source: "organization",
				pointTokens: organizationTrigger.point,
			};
		}
	}

	return { percent: thresholdPercent, source };
};
