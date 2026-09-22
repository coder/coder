import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import type { ProviderInfo } from "#/pages/AgentsPage/utils/modelOptions";
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
	isLoading: boolean;
	isOverridesLoading: boolean;
	loadError: unknown;
	refetchError: unknown;
	modelsError: unknown;
	canEdit: boolean;
	showAdvisor: boolean;
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
];

const OrganizationAgentSettingsView: FC<OrganizationAgentSettingsViewProps> = ({
	defaultModelID,
	onSaveDefaultModel,
	isSavingDefaultModel,
	isSaveDefaultModelError,
	overrides,
	enabledModels,
	providerInfoByID,
	isLoading,
	isOverridesLoading,
	loadError,
	refetchError,
	modelsError,
	canEdit,
	showAdvisor,
	saveByContext,
	savingContexts,
	errorContexts,
}) => {
	const bannerError = loadError ?? refetchError ?? modelsError;
	// The default row only needs the model catalog, so a failed initial
	// overrides load removes just the override rows.
	const visibleSettings =
		loadError == null
			? settings.filter(
					(setting) => setting.context !== "advisor" || showAdvisor,
				)
			: [];

	return (
		<div className="flex flex-col gap-6">
			{bannerError != null && <ErrorAlert error={bannerError} />}
			{enabledModels.length === 0 && !isLoading && modelsError == null && (
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
					isLoading={isLoading}
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
					return (
						<SubagentModelOverrideSettings
							key={setting.context}
							title={setting.title}
							description={setting.description}
							modelOverrideData={overrides === undefined ? undefined : saved}
							enabledModels={enabledModels}
							providerInfoByID={providerInfoByID}
							modelsError={modelsError}
							isLoading={isLoading || isOverridesLoading}
							onSaveModelOverride={onSave}
							isSaving={savingContexts.has(setting.context)}
							isSaveError={errorContexts.has(setting.context)}
							saveErrorMessage={`Failed to save ${setting.title.toLowerCase()} override.`}
							unavailableModelWarning={setting.unavailableModelWarning}
							unsetPlaceholder="Use chat model"
							disabled={!canEdit}
						/>
					);
				})}
			</div>
		</div>
	);
};

export default OrganizationAgentSettingsView;
