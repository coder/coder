import type { FC } from "react";
import type { UseMutateFunction } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	filterConfigsWithEnabledProvider,
	type ProviderInfo,
} from "#/pages/AgentsPage/utils/modelOptions";
import {
	type MutationCallbacks,
	SubagentModelOverrideSettings,
} from "#/pages/AISettingsPage/CoderAgentsPage/components/SubagentModelOverrideSettings";

type SaveModelOverride = UseMutateFunction<
	void,
	Error,
	TypesGen.UpdateChatModelOverrideRequest,
	unknown
>;

interface DefaultsPageViewProps {
	generalModelOverrideData?: TypesGen.ChatModelOverrideResponse;
	titleGenerationModelOverrideData?: TypesGen.ChatModelOverrideResponse;
	compactionModelOverrideData?: TypesGen.ChatModelOverrideResponse;
	exploreModelOverrideData?: TypesGen.ChatModelOverrideResponse;
	advisorModelOverrideData?: TypesGen.ChatModelOverrideResponse;
	modelConfigsData: TypesGen.ChatModelConfig[] | undefined;
	providerInfoByID: ReadonlyMap<string, ProviderInfo>;
	modelConfigsError: unknown;
	isLoadingModelConfigs: boolean;
	onSaveGeneralModelOverride: SaveModelOverride;
	isSavingGeneralModelOverride: boolean;
	isSaveGeneralModelOverrideError: boolean;
	onSaveTitleGenerationModel: SaveModelOverride;
	isSavingTitleGenerationModel: boolean;
	isSaveTitleGenerationModelError: boolean;
	onSaveCompactionModel: SaveModelOverride;
	isSavingCompactionModel: boolean;
	isSaveCompactionModelError: boolean;
	onSaveExploreModelOverride: SaveModelOverride;
	isSavingExploreModelOverride: boolean;
	isSaveExploreModelOverrideError: boolean;
	showAdvisorModelOverride: boolean;
	onSaveAdvisorModelOverride?: (
		req: TypesGen.UpdateChatModelOverrideRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingAdvisorModelOverride: boolean;
	isSaveAdvisorModelOverrideError: boolean;
}

const DefaultsPageView: FC<DefaultsPageViewProps> = ({
	generalModelOverrideData,
	titleGenerationModelOverrideData,
	compactionModelOverrideData,
	exploreModelOverrideData,
	advisorModelOverrideData,
	modelConfigsData,
	providerInfoByID,
	modelConfigsError,
	isLoadingModelConfigs,
	onSaveGeneralModelOverride,
	isSavingGeneralModelOverride,
	isSaveGeneralModelOverrideError,
	onSaveTitleGenerationModel,
	isSavingTitleGenerationModel,
	isSaveTitleGenerationModelError,
	onSaveCompactionModel,
	isSavingCompactionModel,
	isSaveCompactionModelError,
	onSaveExploreModelOverride,
	isSavingExploreModelOverride,
	isSaveExploreModelOverrideError,
	showAdvisorModelOverride,
	onSaveAdvisorModelOverride,
	isSavingAdvisorModelOverride,
	isSaveAdvisorModelOverrideError,
}) => {
	const enabledModelConfigs = filterConfigsWithEnabledProvider(
		(modelConfigsData ?? []).filter((modelConfig) => modelConfig.enabled),
		providerInfoByID,
	);

	return (
		<div className="flex flex-col gap-8">
			<SettingsHeader>
				<SettingsHeaderTitle>Defaults &amp; overrides</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Choose organization-scoped model overrides for agent tasks. Users can
					pick their own models unless an administrator disables personal
					overrides.
				</SettingsHeaderDescription>
			</SettingsHeader>
			<div className="flex flex-col gap-6 rounded-lg border border-solid border-border px-6 py-7">
				<SubagentModelOverrideSettings
					title="General model"
					description="Used by delegated agents that can edit files or run commands."
					modelOverrideData={generalModelOverrideData}
					enabledModelConfigs={enabledModelConfigs}
					providerInfoByID={providerInfoByID}
					modelConfigsError={modelConfigsError}
					isLoading={isLoadingModelConfigs}
					onSaveModelOverride={onSaveGeneralModelOverride}
					isSaving={isSavingGeneralModelOverride}
					isSaveError={isSaveGeneralModelOverrideError}
					saveErrorMessage="Failed to save general model override."
				/>
				<SubagentModelOverrideSettings
					title="Title generation model"
					description="Leave unset to use Coder's title default, which prefers fast models from configured providers."
					modelOverrideData={titleGenerationModelOverrideData}
					enabledModelConfigs={enabledModelConfigs}
					providerInfoByID={providerInfoByID}
					modelConfigsError={modelConfigsError}
					isLoading={isLoadingModelConfigs}
					onSaveModelOverride={onSaveTitleGenerationModel}
					isSaving={isSavingTitleGenerationModel}
					isSaveError={isSaveTitleGenerationModelError}
					saveErrorMessage="Failed to save title generation model."
					unsetPlaceholder="Use title default"
					unavailableModelWarning="The selected model is currently unavailable. Title generation will be skipped until you choose another model or clear this setting."
				/>
				<SubagentModelOverrideSettings
					title="Compaction model"
					description="Used to summarize long conversations when they approach the model's context limit. Leave unset to summarize with the chat model."
					modelOverrideData={compactionModelOverrideData}
					enabledModelConfigs={enabledModelConfigs}
					providerInfoByID={providerInfoByID}
					modelConfigsError={modelConfigsError}
					isLoading={isLoadingModelConfigs}
					onSaveModelOverride={onSaveCompactionModel}
					isSaving={isSavingCompactionModel}
					isSaveError={isSaveCompactionModelError}
					saveErrorMessage="Failed to save compaction model."
					unsetPlaceholder="Use chat model"
				/>
				<SubagentModelOverrideSettings
					title="Explore subagent model"
					description="Used for read-only codebase exploration before work returns to the main agent."
					modelOverrideData={exploreModelOverrideData}
					enabledModelConfigs={enabledModelConfigs}
					providerInfoByID={providerInfoByID}
					modelConfigsError={modelConfigsError}
					isLoading={isLoadingModelConfigs}
					onSaveModelOverride={onSaveExploreModelOverride}
					isSaving={isSavingExploreModelOverride}
					isSaveError={isSaveExploreModelOverrideError}
					saveErrorMessage="Failed to save Explore model override."
				/>
				{showAdvisorModelOverride && onSaveAdvisorModelOverride && (
					<SubagentModelOverrideSettings
						title="Advisor model"
						description="Used by the advisor that provides strategic guidance to root agent chats. Leave unset to reuse the chat model."
						modelOverrideData={advisorModelOverrideData}
						enabledModelConfigs={enabledModelConfigs}
						providerInfoByID={providerInfoByID}
						modelConfigsError={modelConfigsError}
						isLoading={isLoadingModelConfigs}
						onSaveModelOverride={onSaveAdvisorModelOverride}
						isSaving={isSavingAdvisorModelOverride}
						isSaveError={isSaveAdvisorModelOverrideError}
						saveErrorMessage="Failed to save advisor model override."
						unsetPlaceholder="Reuse chat model"
					/>
				)}
			</div>
		</div>
	);
};

export default DefaultsPageView;
