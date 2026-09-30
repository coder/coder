import type * as TypesGen from "#/api/typesGenerated";
import {
	filterModelsWithEnabledProvider,
	type ProviderInfo,
} from "./utils/modelOptions";

export type CompactionTrigger = {
	readonly thresholdPercent: number;
	readonly contextLimit: number;
};

export type OrganizationCompactionTrigger = {
	readonly model: TypesGen.ChatModel;
	readonly trigger: CompactionTrigger;
	readonly pointTokens: number;
};

export type ResolvedCompactionThreshold =
	| { readonly percent: number; readonly source: "user" | "model" }
	| {
			readonly percent: number;
			readonly source: "organization";
			// Token count that triggers organization compaction. The gauge converts
			// this using its runtime context limit; percent uses the configured limit.
			readonly pointTokens: number;
	  };

export const compactionDisabledThresholdPercent = 100;

export const modelCompactionTrigger = (
	model: TypesGen.ChatModel,
): CompactionTrigger => ({
	thresholdPercent: model.compression_threshold,
	contextLimit: model.context_limit,
});

export const isCompactionTriggerEnabled = (trigger: CompactionTrigger) =>
	trigger.thresholdPercent >= 0 &&
	trigger.thresholdPercent < compactionDisabledThresholdPercent &&
	trigger.contextLimit > 0;

export const compactionTriggerPoint = (trigger: CompactionTrigger) =>
	(trigger.contextLimit * trigger.thresholdPercent) / 100;

// A disabled organization trigger yields "chat" and a disabled chat trigger
// yields "organization"; otherwise the lower token point binds, ties to chat.
export const bindingCompactionTriggerSource = (
	chat: CompactionTrigger,
	organization: CompactionTrigger,
): "chat" | "organization" => {
	if (!isCompactionTriggerEnabled(organization)) {
		return "chat";
	}
	if (!isCompactionTriggerEnabled(chat)) {
		return "organization";
	}
	return compactionTriggerPoint(organization) < compactionTriggerPoint(chat)
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
		bindingCompactionTriggerSource(chat, organizationTrigger.trigger) ===
			"organization"
	) {
		return organizationTrigger.pointTokens;
	}
	return isCompactionTriggerEnabled(chat)
		? compactionTriggerPoint(chat)
		: undefined;
};

export const organizationCompactionTrigger = (
	model: TypesGen.ChatModel,
): OrganizationCompactionTrigger => {
	const trigger = modelCompactionTrigger(model);
	return { model, trigger, pointTokens: compactionTriggerPoint(trigger) };
};

// "viewer" also drops an override whose provider the current user cannot use,
// as chatd does per user; admin views of the organization setting pass
// "organization".
export const resolveOrganizationCompactionTrigger = (
	modelConfigID: string | undefined,
	models: readonly TypesGen.ChatModel[] | null | undefined,
	providerInfoByID: ReadonlyMap<string, ProviderInfo>,
	scope: "viewer" | "organization" = "viewer",
): OrganizationCompactionTrigger | undefined => {
	const model = filterModelsWithEnabledProvider(
		models ?? [],
		providerInfoByID,
	).find((candidate) => candidate.id === modelConfigID);
	// Override models that are disabled or whose provider is disabled fall
	// back to the chat model on the backend.
	if (!model?.enabled) {
		return undefined;
	}
	if (
		scope === "viewer" &&
		providerInfoByID.get(model.ai_provider_id)?.available === false
	) {
		return undefined;
	}

	if (!isCompactionTriggerEnabled(modelCompactionTrigger(model))) {
		return undefined;
	}

	return organizationCompactionTrigger(model);
};

export type CompactionTriggerLoadError = {
	readonly organizationID: string;
	readonly error: unknown;
};

type OrganizationOverridesState = {
	readonly organizationID: string;
	readonly data: TypesGen.ChatModelOverridesResponse | undefined;
	readonly error: unknown;
};

export const resolveOrganizationCompactionTriggers = (
	organizationOverrides: readonly OrganizationOverridesState[],
	models: readonly TypesGen.ChatModel[],
	providerInfoByID: ReadonlyMap<string, ProviderInfo>,
) => {
	const triggersByOrganizationID = new Map<
		string,
		OrganizationCompactionTrigger
	>();
	const loadErrors: CompactionTriggerLoadError[] = [];
	for (const { organizationID, data, error } of organizationOverrides) {
		const organizationModels = models.filter(
			(model) => model.organization_id === organizationID,
		);
		if (
			error != null &&
			data === undefined &&
			organizationModels.some((model) => model.enabled)
		) {
			loadErrors.push({ organizationID, error });
		}
		const trigger = resolveOrganizationCompactionTrigger(
			data?.overrides.find((override) => override.context === "compaction")
				?.model_config_id,
			organizationModels,
			providerInfoByID,
		);
		if (trigger) {
			triggersByOrganizationID.set(organizationID, trigger);
		}
	}
	return { triggersByOrganizationID, loadErrors };
};

export const compactionPointAsPercent = (
	point: number,
	contextLimit: number,
): number | undefined =>
	contextLimit > 0 && Number.isFinite(point)
		? (point / contextLimit) * 100
		: undefined;

// A binding point past the window fires no trigger inside it, so surfaces
// show no compaction. A point at the window stays reachable because the
// backend fires at usage >= point; an unknown window is not judged.
export const isCompactionPointBeyondWindow = (
	point: number,
	contextLimit: number,
) => contextLimit > 0 && point > contextLimit;

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
			organizationTrigger.pointTokens,
			config.context_limit,
		);
		// Bind on token points, not organizationPercent: the organization point
		// can convert to 100% or more of this window, which reads as disabled.
		if (
			organizationPercent !== undefined &&
			bindingCompactionTriggerSource(
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
				pointTokens: organizationTrigger.pointTokens,
			};
		}
	}

	return { percent: thresholdPercent, source };
};
