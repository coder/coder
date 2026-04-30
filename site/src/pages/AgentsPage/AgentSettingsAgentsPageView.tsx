import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { SectionHeader } from "./components/SectionHeader";
import {
	type MutationCallbacks,
	SubagentModelOverrideSettings,
} from "./components/SubagentModelOverrideSettings";

type SaveModelOverride = (
	req: { readonly model_config_id: string },
	options?: MutationCallbacks,
) => void;

export interface AgentSettingsAgentsPageViewProps {
	generalModelOverrideData?: TypesGen.ChatModelOverrideResponse;
	titleGenerationModelOverrideData?: TypesGen.ChatModelOverrideResponse;
	exploreModelOverrideData?: TypesGen.ChatModelOverrideResponse;
	modelConfigsData: TypesGen.ChatModelConfig[] | undefined;
	modelConfigsError: unknown;
	isLoadingModelConfigs: boolean;
	onSaveGeneralModelOverride?: SaveModelOverride;
	isSavingGeneralModelOverride?: boolean;
	isSaveGeneralModelOverrideError?: boolean;
	onSaveTitleGenerationModel: SaveModelOverride;
	isSavingTitleGenerationModel: boolean;
	isSaveTitleGenerationModelError: boolean;
	onSaveExploreModelOverride: SaveModelOverride;
	isSavingExploreModelOverride: boolean;
	isSaveExploreModelOverrideError: boolean;
}

export const AgentSettingsAgentsPageView: FC<
	AgentSettingsAgentsPageViewProps
> = ({
	generalModelOverrideData,
	titleGenerationModelOverrideData,
	exploreModelOverrideData,
	modelConfigsData,
	modelConfigsError,
	isLoadingModelConfigs,
	onSaveGeneralModelOverride,
	isSavingGeneralModelOverride = false,
	isSaveGeneralModelOverrideError = false,
	onSaveTitleGenerationModel,
	isSavingTitleGenerationModel,
	isSaveTitleGenerationModelError,
	onSaveExploreModelOverride,
	isSavingExploreModelOverride,
	isSaveExploreModelOverrideError,
}) => {
	const enabledModelConfigs = (modelConfigsData ?? []).filter(
		(modelConfig) => modelConfig.enabled,
	);
	const showGeneralModelSection =
		onSaveGeneralModelOverride !== undefined ||
		generalModelOverrideData !== undefined ||
		isSavingGeneralModelOverride ||
		isSaveGeneralModelOverrideError;

	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label="Agents"
				description="Configure defaults for delegated agents and other agent-specific capabilities."
			/>
			{showGeneralModelSection && onSaveGeneralModelOverride && (
				<section aria-label="General model" className="flex flex-col gap-3">
					<SectionHeader
						label="General model"
						description="Deployment-wide model override for delegated subagents with write capabilities, such as editing files or running commands in the workspace."
						level="section"
					/>
					<SubagentModelOverrideSettings
						title="General model"
						description="Deployment-wide model override for delegated subagents with write capabilities, such as editing files or running commands in the workspace."
						modelOverrideData={generalModelOverrideData}
						enabledModelConfigs={enabledModelConfigs}
						modelConfigsError={modelConfigsError}
						isLoading={isLoadingModelConfigs}
						onSaveModelOverride={onSaveGeneralModelOverride}
						isSaving={isSavingGeneralModelOverride}
						isSaveError={isSaveGeneralModelOverrideError}
						saveErrorMessage="Failed to save general model override."
						showHeader={false}
					/>
				</section>
			)}
			<section
				aria-label="Title generation model"
				className="flex flex-col gap-3"
			>
				<SectionHeader
					label="Title generation model"
					description="Choose a model for generated chat titles. Leave unset to use Coder's default title algorithm, which currently tries fast title models for configured providers first, for example Claude Haiku, GPT-4o mini, and Gemini Flash, then falls back to the chat's current model. When a model is selected here, Coder uses only that model for title generation. Recommended title models are fast and low cost."
					level="section"
				/>
				<SubagentModelOverrideSettings
					title="Title generation model"
					description="Choose a model for generated chat titles."
					modelOverrideData={titleGenerationModelOverrideData}
					enabledModelConfigs={enabledModelConfigs}
					modelConfigsError={modelConfigsError}
					isLoading={isLoadingModelConfigs}
					onSaveModelOverride={onSaveTitleGenerationModel}
					isSaving={isSavingTitleGenerationModel}
					isSaveError={isSaveTitleGenerationModelError}
					saveErrorMessage="Failed to save title generation model."
					unsetPlaceholder="Use title default"
					unavailableModelWarning="The selected model is currently unavailable. Title generation will be skipped until you choose another model or clear this setting."
					showHeader={false}
				/>
			</section>
			<section
				aria-label="Explore subagent model"
				className="flex flex-col gap-3"
			>
				<SectionHeader
					label="Explore subagent model"
					description="Deployment-wide model override for read-only Explore subagents."
					level="section"
				/>
				<SubagentModelOverrideSettings
					title="Explore subagent model"
					description={
						<>
							Deployment-wide model override for read-only Explore subagents
							launched through the <code>spawn_agent</code> tool with a
							<code>type=explore</code> argument.
						</>
					}
					modelOverrideData={exploreModelOverrideData}
					enabledModelConfigs={enabledModelConfigs}
					modelConfigsError={modelConfigsError}
					isLoading={isLoadingModelConfigs}
					onSaveModelOverride={onSaveExploreModelOverride}
					isSaving={isSavingExploreModelOverride}
					isSaveError={isSaveExploreModelOverrideError}
					saveErrorMessage="Failed to save Explore model override."
					showHeader={false}
				/>
			</section>
		</div>
	);
};
