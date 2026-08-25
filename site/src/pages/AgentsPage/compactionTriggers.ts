import type * as TypesGen from "#/api/typesGenerated";

interface CompactionTrigger {
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

export const resolveOrganizationCompactionTrigger = (
	overrides: readonly TypesGen.ChatModelOverrideResponse[] | undefined,
	models: readonly TypesGen.ChatModel[] | null | undefined,
): OrganizationCompactionTrigger | undefined => {
	const override = overrides?.find(
		(candidate) => candidate.context === "compaction",
	);
	const model = models?.find(
		(candidate) => candidate.id === override?.model_config_id,
	);
	if (!model) {
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
