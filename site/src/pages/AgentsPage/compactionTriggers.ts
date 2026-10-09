import type { UseQueryResult } from "react-query";
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
	| {
			readonly percent: number;
			readonly source: "user" | "model";
			// The organization overrides never loaded, so a binding organization
			// override may exist that this threshold does not reflect.
			readonly organizationOverrideNotLoaded: boolean;
	  }
	| {
			readonly source: "organization";
			// Token count that triggers organization compaction.
			readonly pointTokens: number;
	  };

export const compactionDisabledThresholdPercent = 100;

// Mirrors chatd compactionOverrideWindowPercent. The remaining 20% is target
// headroom for the summary prompt and summary, not a guaranteed fit.
export const compactionOverrideWindowPercent = 80;

export const modelCompactionTrigger = (
	model: TypesGen.ChatModel,
): CompactionTrigger => ({
	thresholdPercent: model.compression_threshold,
	contextLimit: model.context_limit,
});

const isCompactionTriggerEnabled = (trigger: CompactionTrigger) =>
	trigger.thresholdPercent >= 0 &&
	trigger.thresholdPercent < compactionDisabledThresholdPercent &&
	trigger.contextLimit > 0;

const compactionTriggerPoint = (trigger: CompactionTrigger) =>
	(trigger.contextLimit * trigger.thresholdPercent) / 100;

// Mirrors chatd (keep in sync with TestBindingCompactionTriggerSource): a
// disabled organization trigger yields chat, a disabled chat trigger yields
// organization, otherwise the lower token point binds with ties to chat.
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

// Undefined when neither trigger is enabled.
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

// "viewer" drops an override whose provider the current user cannot use, as
// chatd does per user; "organization" keeps it.
export const resolveOrganizationCompactionTrigger = (
	modelConfigID: string | undefined,
	models: readonly TypesGen.ChatModel[],
	providerInfoByID: ReadonlyMap<string, ProviderInfo>,
	scope: "viewer" | "organization" = "viewer",
): OrganizationCompactionTrigger | undefined => {
	const model = filterModelsWithEnabledProvider(models, providerInfoByID).find(
		(candidate) => candidate.id === modelConfigID,
	);
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

	const trigger = {
		thresholdPercent: compactionOverrideWindowPercent,
		contextLimit: model.context_limit,
	};
	return { model, trigger, pointTokens: compactionTriggerPoint(trigger) };
};

export type CompactionTriggerLoadError = {
	readonly organizationID: string;
	readonly error: unknown;
};

type OverridesQueryState = Pick<
	UseQueryResult<TypesGen.ChatModelOverridesResponse>,
	"data" | "error"
>;

type OrganizationOverridesState = OverridesQueryState & {
	readonly organizationID: string;
};

const compactionOverrideModelConfigID = (
	data: TypesGen.ChatModelOverridesResponse | undefined,
) =>
	data?.overrides.find((override) => override.context === "compaction")
		?.model_config_id;

/**
 * Reports a failed overrides fetch only when nothing is cached and the
 * organization has an enabled model, since only those have threshold rows.
 */
export const resolveCompactionTriggersByOrganization = (
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
			compactionOverrideModelConfigID(data),
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

// An initial overrides failure keeps the chat threshold but flags it; a failed
// background refetch keeps using the cached overrides.
export const resolveChatCompactionThreshold = (
	modelID: string,
	userThresholds: readonly TypesGen.UserChatCompactionThreshold[] | undefined,
	models: readonly TypesGen.ChatModel[] | undefined,
	providerInfoByID: ReadonlyMap<string, ProviderInfo>,
	overrides: OverridesQueryState,
): ResolvedCompactionThreshold | undefined => {
	if (!models) {
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
	const organizationTrigger = resolveOrganizationCompactionTrigger(
		compactionOverrideModelConfigID(overrides.data),
		models,
		providerInfoByID,
	);
	if (
		organizationTrigger &&
		bindingCompactionTriggerSource(
			{
				thresholdPercent,
				contextLimit: config.context_limit,
			},
			organizationTrigger.trigger,
		) === "organization"
	) {
		return {
			source: "organization",
			pointTokens: organizationTrigger.pointTokens,
		};
	}

	return {
		percent: thresholdPercent,
		source: userOverride ? "user" : "model",
		organizationOverrideNotLoaded:
			overrides.error != null && overrides.data === undefined,
	};
};

export const formatCompactionPercent = (percent: number) =>
	percent.toLocaleString("en-US", { maximumFractionDigits: 1 });

/**
 * Converts an organization point against the displayed context window, which
 * may differ from the configured one.
 */
export const compactionThresholdLabel = (
	compaction: ResolvedCompactionThreshold,
	contextLimit: number,
): string | undefined => {
	if (compaction.source === "organization") {
		const percent = compactionPointAsPercent(
			compaction.pointTokens,
			contextLimit,
		);
		if (
			percent === undefined ||
			isCompactionPointBeyondWindow(compaction.pointTokens, contextLimit)
		) {
			return undefined;
		}
		return `Compacts at ${formatCompactionPercent(percent)}% (organization override)`;
	}
	const notLoadedSuffix = compaction.organizationOverrideNotLoaded
		? " (organization override not loaded)"
		: "";
	if (compaction.percent < compactionDisabledThresholdPercent) {
		return `Compacts at ${formatCompactionPercent(compaction.percent)}%${notLoadedSuffix}`;
	}
	if (compaction.organizationOverrideNotLoaded) {
		return "Compaction off (organization override not loaded)";
	}
	return undefined;
};
