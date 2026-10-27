import type * as TypesGen from "#/api/typesGenerated";
import { ChatComputerUseProviders } from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { normalizeProvider } from "#/modules/aiModels/helpers";
import { isKnownComputerUseModel } from "#/modules/aiModels/knownModels/catalog";
import {
	bindingCompactionTriggerSource,
	compactionDisabledThresholdPercent,
	isCompactionPointBeyondWindow,
	modelCompactionTrigger,
	resolveOrganizationCompactionTrigger,
} from "#/pages/AgentsPage/utils/compactionTriggers";
import {
	getModelLabel,
	type ProviderInfo,
} from "#/pages/AgentsPage/utils/modelOptions";
import { DefaultModelSettings } from "#/pages/AISettingsPage/CoderAgentsPage/components/DefaultModelSettings";
import {
	type MutationCallbacks,
	SubagentModelOverrideSettings,
} from "#/pages/AISettingsPage/CoderAgentsPage/components/SubagentModelOverrideSettings";

export type SaveModelOverride = (
	req: TypesGen.UpdateChatModelOverrideRequest,
	options?: MutationCallbacks,
) => void;

type OrganizationAgentSettingsViewProps = {
	defaultModelID: string | undefined;
	onSaveDefaultModel: (modelID: string, options: MutationCallbacks) => void;
	isSavingDefaultModel: boolean;
	isSaveDefaultModelError: boolean;
	overrides: readonly TypesGen.ChatModelOverrideResponse[] | undefined;
	enabledModels: readonly TypesGen.ChatModel[];
	providerInfoByID: ReadonlyMap<string, ProviderInfo>;
	isModelsLoading: boolean;
	isOverridesLoading: boolean;
	overridesLoadError: unknown;
	overridesRefetchError: unknown;
	modelsError: unknown;
	canEdit: boolean;
	showAdvisor: boolean;
	showComputerUse: boolean;
	saveByContext: ReadonlyMap<
		TypesGen.ChatModelOverrideContext,
		SaveModelOverride
	>;
	savingContexts: ReadonlySet<TypesGen.ChatModelOverrideContext>;
	errorContexts: ReadonlySet<TypesGen.ChatModelOverrideContext>;
};

const settings: readonly {
	context: TypesGen.ChatModelOverrideContext;
	title: string;
	description: string;
	unsetPlaceholder?: string;
	unavailableModelWarning?: string;
}[] = [
	{
		context: "general",
		title: "General subagent",
		description:
			"Used by delegated agents that can edit files or run commands.",
	},
	{
		context: "explore",
		title: "Explore subagent",
		description: "Used for read-only codebase exploration.",
	},
	{
		context: "title_generation",
		title: "Title generation",
		description:
			"Used to generate chat titles, turn status labels, and chat summaries.",
		// These side calls fail hard on a broken override instead of falling
		// back to default model selection, so the generic warning is wrong here.
		unavailableModelWarning:
			"The selected model is currently unavailable. Titles, status labels, and summaries will be skipped until you choose another model or clear this setting.",
	},
	{
		context: "compaction",
		title: "Compaction",
		description: "Used to summarize conversations near the context limit.",
	},
	{
		context: "advisor",
		title: "Advisor",
		description: "Used by the advisor for strategic guidance.",
	},
	{
		context: "computer_use",
		title: "Computer use",
		description:
			"Used by the computer use subagent to operate the virtual desktop. Only Anthropic and OpenAI models are supported.",
		unsetPlaceholder: "Use default model",
		// This row lists only Anthropic and OpenAI models, so a saved model can
		// be unavailable here while it stays enabled.
		unavailableModelWarning:
			"The saved model is no longer available for computer use and will be ignored until you choose a new override.",
	},
];

const getProviderType = (
	model: TypesGen.ChatModel,
	providerInfoByID: ReadonlyMap<string, ProviderInfo>,
) => providerInfoByID.get(model.ai_provider_id)?.provider ?? "";

// The API rejects computer use overrides for other provider types.
const filterComputerUseProviderModels = (
	models: readonly TypesGen.ChatModel[],
	providerInfoByID: ReadonlyMap<string, ProviderInfo>,
) =>
	models.filter((model) => {
		const providerType = normalizeProvider(
			getProviderType(model, providerInfoByID),
		);
		return ChatComputerUseProviders.some(
			(provider) => provider === providerType,
		);
	});

// Computer use falls back to the organization default model when the override
// is unset or missing from computerUseProviderModels, so the warning follows
// that model instead.
const isUnverifiedComputerUseModel = (
	selectedModelID: string,
	defaultModelID: string | undefined,
	computerUseProviderModels: readonly TypesGen.ChatModel[],
	enabledModels: readonly TypesGen.ChatModel[],
	providerInfoByID: ReadonlyMap<string, ProviderInfo>,
) => {
	const effectiveModel =
		computerUseProviderModels.find((model) => model.id === selectedModelID) ??
		enabledModels.find((model) => model.id === defaultModelID);
	if (effectiveModel === undefined) {
		return false;
	}
	const providerType = getProviderType(effectiveModel, providerInfoByID);
	return (
		!isKnownComputerUseModel(providerType, effectiveModel.model) ||
		// The OpenAI computer tool exists only on the Responses API.
		(normalizeProvider(providerType) === "openai" &&
			effectiveModel.model_config?.openai_config?.use_responses_api === false)
	);
};

const formatModelList = (modelNames: readonly string[]) => {
	const visibleNames = modelNames.slice(0, 3);
	const remainingCount = modelNames.length - visibleNames.length;
	return remainingCount > 0
		? `${visibleNames.join(", ")} and ${remainingCount} more`
		: visibleNames.join(", ");
};

type CompactionOverrideAlertProps = {
	selectedModelID: string;
	enabledModels: readonly TypesGen.ChatModel[];
	providerInfoByID: ReadonlyMap<string, ProviderInfo>;
};

const CompactionOverrideAlert: React.FC<CompactionOverrideAlertProps> = ({
	selectedModelID,
	enabledModels,
	providerInfoByID,
}) => {
	const organizationTrigger = resolveOrganizationCompactionTrigger(
		selectedModelID,
		enabledModels,
		providerInfoByID,
		"organization",
	);
	if (!organizationTrigger) {
		return null;
	}

	const compactionModel = organizationTrigger.model;
	const undercutModels = enabledModels.filter(
		(model) =>
			bindingCompactionTriggerSource(
				modelCompactionTrigger(model),
				organizationTrigger.trigger,
			) === "organization" &&
			!isCompactionPointBeyondWindow(
				organizationTrigger.pointTokens,
				model.context_limit,
			),
	);
	if (undercutModels.length === 0) {
		return null;
	}
	const offModelsNote = undercutModels.some(
		(model) =>
			model.compression_threshold >= compactionDisabledThresholdPercent,
	)
		? ", including models with compaction off,"
		: "";

	return (
		<Alert severity="warning">
			<AlertDescription>
				{`Chats using ${formatModelList(undercutModels.map(getModelLabel))} may compact earlier than their models' thresholds${offModelsNote} because the organization override compacts chats at ${organizationTrigger.trigger.thresholdPercent}% of ${getModelLabel(compactionModel)}'s ${compactionModel.context_limit.toLocaleString("en-US")}-token window. Personal thresholds that trigger compaction sooner still apply first.`}
			</AlertDescription>
		</Alert>
	);
};

const OrganizationAgentSettingsView: React.FC<
	OrganizationAgentSettingsViewProps
> = ({
	defaultModelID,
	onSaveDefaultModel,
	isSavingDefaultModel,
	isSaveDefaultModelError,
	overrides,
	enabledModels,
	providerInfoByID,
	isModelsLoading,
	isOverridesLoading,
	overridesLoadError,
	overridesRefetchError,
	modelsError,
	canEdit,
	showAdvisor,
	showComputerUse,
	saveByContext,
	savingContexts,
	errorContexts,
}) => {
	const bannerError =
		overridesLoadError ?? overridesRefetchError ?? modelsError;
	// The default row only needs the model catalog, so a failed initial
	// overrides load removes just the override rows.
	const visibleSettings =
		overridesLoadError == null
			? settings.filter(
					(setting) =>
						(setting.context !== "advisor" || showAdvisor) &&
						(setting.context !== "computer_use" || showComputerUse),
				)
			: [];
	const computerUseProviderModels = filterComputerUseProviderModels(
		enabledModels,
		providerInfoByID,
	);
	const renderComputerUseWarning = (selectedModelID: string) =>
		overrides !== undefined &&
		isUnverifiedComputerUseModel(
			selectedModelID,
			defaultModelID,
			computerUseProviderModels,
			enabledModels,
			providerInfoByID,
		) && (
			<InfoTooltip type="warning" ariaLabel="Computer use model warning">
				Make sure that the selected model supports computer use.
			</InfoTooltip>
		);

	return (
		<div className="flex flex-col gap-6">
			{bannerError != null && <ErrorAlert error={bannerError} />}
			{enabledModels.length === 0 &&
				!isModelsLoading &&
				modelsError == null && (
					<p role="status" className="m-0 text-content-secondary">
						This organization has no enabled chat models.
					</p>
				)}
			<div className="flex flex-col gap-6 rounded-lg border border-solid border-border px-6 py-7">
				<DefaultModelSettings
					defaultModelID={defaultModelID}
					enabledModels={enabledModels}
					providerInfoByID={providerInfoByID}
					modelsError={modelsError}
					isLoading={isModelsLoading}
					onSaveDefaultModel={onSaveDefaultModel}
					isSaving={isSavingDefaultModel}
					isSaveError={isSaveDefaultModelError}
					disabled={!canEdit}
				/>
				{visibleSettings.map((setting) => {
					const saved = overrides?.find(
						(override) => override.context === setting.context,
					) ?? { context: setting.context, model_config_id: "" };
					const onSave = saveByContext.get(setting.context);
					if (!onSave) {
						return null;
					}
					const isComputerUse = setting.context === "computer_use";
					return (
						<SubagentModelOverrideSettings
							key={setting.context}
							title={setting.title}
							description={setting.description}
							modelOverrideData={overrides === undefined ? undefined : saved}
							enabledModels={
								isComputerUse ? computerUseProviderModels : enabledModels
							}
							providerInfoByID={providerInfoByID}
							modelsError={modelsError}
							isLoading={isModelsLoading || isOverridesLoading}
							onSaveModelOverride={onSave}
							isSaving={savingContexts.has(setting.context)}
							isSaveError={errorContexts.has(setting.context)}
							saveErrorMessage={`Failed to save ${setting.title.toLowerCase()} override.`}
							unavailableModelWarning={setting.unavailableModelWarning}
							renderSelectedModelAlert={
								setting.context === "compaction"
									? (selectedModelID) => (
											<CompactionOverrideAlert
												selectedModelID={selectedModelID}
												enabledModels={enabledModels}
												providerInfoByID={providerInfoByID}
											/>
										)
									: undefined
							}
							renderSelectorAdornment={
								isComputerUse ? renderComputerUseWarning : undefined
							}
							unsetPlaceholder={setting.unsetPlaceholder ?? "Use chat model"}
							disabled={!canEdit}
						/>
					);
				})}
			</div>
		</div>
	);
};

export default OrganizationAgentSettingsView;
