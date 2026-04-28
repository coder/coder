import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { SectionHeader } from "./components/SectionHeader";
import {
	type MutationCallbacks,
	SubagentModelOverrideSettings,
} from "./components/SubagentModelOverrideSettings";

type SaveModelOverride = (
	req: TypesGen.UpdateChatAgentModelOverrideRequest,
	options?: MutationCallbacks,
) => void;

export interface AgentSettingsAgentsPageViewProps {
	generalModelOverrideData?: TypesGen.ChatAgentModelOverrideResponse;
	exploreModelOverrideData?: TypesGen.ChatAgentModelOverrideResponse;
	modelConfigsData: TypesGen.ChatModelConfig[] | undefined;
	modelConfigsError: unknown;
	isLoadingModelConfigs: boolean;
	onSaveGeneralModelOverride?: SaveModelOverride;
	isSavingGeneralModelOverride?: boolean;
	isSaveGeneralModelOverrideError?: boolean;
	onSaveExploreModelOverride: SaveModelOverride;
	isSavingExploreModelOverride: boolean;
	isSaveExploreModelOverrideError: boolean;
}

export const AgentSettingsAgentsPageView: FC<
	AgentSettingsAgentsPageViewProps
> = ({
	generalModelOverrideData,
	exploreModelOverrideData,
	modelConfigsData,
	modelConfigsError,
	isLoadingModelConfigs,
	onSaveGeneralModelOverride,
	isSavingGeneralModelOverride = false,
	isSaveGeneralModelOverrideError = false,
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
