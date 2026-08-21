import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import type { ProviderInfo } from "#/pages/AgentsPage/utils/modelOptions";
import { SubagentModelOverrideSettings } from "#/pages/AISettingsPage/CoderAgentsPage/components/SubagentModelOverrideSettings";

export type SaveModelOverride = (
	req: TypesGen.UpdateChatModelOverrideRequest,
	options?: { onSuccess?: () => void; onError?: () => void },
) => void;

interface DefaultsPageViewProps {
	overrides: readonly TypesGen.ChatModelOverrideResponse[] | undefined;
	enabledModels: readonly TypesGen.ChatModel[];
	providerInfoByID: ReadonlyMap<string, ProviderInfo>;
	isLoading: boolean;
	loadError: unknown;
	refetchError: unknown;
	canEdit: boolean;
	showAdvisor: boolean;
	saveByContext: ReadonlyMap<
		TypesGen.ChatModelOverrideContext,
		SaveModelOverride
	>;
	savingContexts: ReadonlySet<TypesGen.ChatModelOverrideContext>;
	errorContexts: ReadonlySet<TypesGen.ChatModelOverrideContext>;
}

const settings: readonly {
	context: TypesGen.ChatModelOverrideContext;
	title: string;
	description: string;
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
		description: "Used to generate chat titles.",
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

const DefaultsPageView: FC<DefaultsPageViewProps> = ({
	overrides,
	enabledModels,
	providerInfoByID,
	isLoading,
	loadError,
	refetchError,
	canEdit,
	showAdvisor,
	saveByContext,
	savingContexts,
	errorContexts,
}) => {
	if (loadError) {
		return <ErrorAlert error={loadError} />;
	}
	const visibleSettings = settings.filter(
		(setting) => setting.context !== "advisor" || showAdvisor,
	);

	return (
		<div className="flex max-w-4xl flex-col gap-8">
			<SettingsHeader>
				<SettingsHeaderTitle>Defaults & overrides</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Choose organization defaults for each Coder Agents context. Leave a
					model unset to use the default model selection.
				</SettingsHeaderDescription>
			</SettingsHeader>
			{refetchError != null && <ErrorAlert error={refetchError} />}
			{enabledModels.length === 0 && !isLoading && (
				<p role="status" className="m-0 text-content-secondary">
					This organization has no enabled chat models.
				</p>
			)}
			<div className="flex flex-col gap-6 rounded-lg border border-solid border-border px-6 py-7">
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
							modelsError={refetchError}
							isLoading={isLoading}
							onSaveModelOverride={onSave}
							isSaving={savingContexts.has(setting.context)}
							isSaveError={errorContexts.has(setting.context)}
							saveErrorMessage={`Failed to save ${setting.title.toLowerCase()} override.`}
							unsetPlaceholder="Use default"
							disabled={!canEdit}
						/>
					);
				})}
			</div>
		</div>
	);
};

export default DefaultsPageView;
