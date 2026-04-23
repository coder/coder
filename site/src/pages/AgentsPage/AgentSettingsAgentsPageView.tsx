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
	planSubagentModelOverrideData?: TypesGen.ChatAgentModelOverrideResponse;
	exploreModelOverrideData?: TypesGen.ChatAgentModelOverrideResponse;
	computerUseModelOverrideData?: TypesGen.ChatAgentModelOverrideResponse;
	modelConfigsData: TypesGen.ChatModelConfig[] | undefined;
	modelConfigsError: unknown;
	isLoadingModelConfigs: boolean;
	onSaveGeneralModelOverride?: SaveModelOverride;
	isSavingGeneralModelOverride?: boolean;
	isSaveGeneralModelOverrideError?: boolean;
	onSavePlanSubagentModelOverride?: SaveModelOverride;
	isSavingPlanSubagentModelOverride?: boolean;
	isSavePlanSubagentModelOverrideError?: boolean;
	onSaveExploreModelOverride: SaveModelOverride;
	isSavingExploreModelOverride: boolean;
	isSaveExploreModelOverrideError: boolean;
	onSaveComputerUseModelOverride?: SaveModelOverride;
	isSavingComputerUseModelOverride?: boolean;
	isSaveComputerUseModelOverrideError?: boolean;
}

export const AgentSettingsAgentsPageView: FC<
	AgentSettingsAgentsPageViewProps
> = ({
	generalModelOverrideData,
	planSubagentModelOverrideData,
	exploreModelOverrideData,
	computerUseModelOverrideData,
	modelConfigsData,
	modelConfigsError,
	isLoadingModelConfigs,
	onSaveGeneralModelOverride,
	isSavingGeneralModelOverride = false,
	isSaveGeneralModelOverrideError = false,
	onSavePlanSubagentModelOverride,
	isSavingPlanSubagentModelOverride = false,
	isSavePlanSubagentModelOverrideError = false,
	onSaveExploreModelOverride,
	isSavingExploreModelOverride,
	isSaveExploreModelOverrideError,
	onSaveComputerUseModelOverride,
	isSavingComputerUseModelOverride = false,
	isSaveComputerUseModelOverrideError = false,
}) => {
	const enabledModelConfigs = (modelConfigsData ?? []).filter(
		(modelConfig) => modelConfig.enabled,
	);
	const showGeneralModelSection =
		onSaveGeneralModelOverride !== undefined ||
		generalModelOverrideData !== undefined ||
		isSavingGeneralModelOverride ||
		isSaveGeneralModelOverrideError;
	const showPlanSubagentModelSection =
		onSavePlanSubagentModelOverride !== undefined ||
		planSubagentModelOverrideData !== undefined ||
		isSavingPlanSubagentModelOverride ||
		isSavePlanSubagentModelOverrideError;
	const showComputerUseModelSection =
		onSaveComputerUseModelOverride !== undefined ||
		computerUseModelOverrideData !== undefined ||
		isSavingComputerUseModelOverride ||
		isSaveComputerUseModelOverrideError;

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
			{showPlanSubagentModelSection && onSavePlanSubagentModelOverride && (
				<section
					aria-label="Plan subagent model"
					className="flex flex-col gap-3"
				>
					<SectionHeader
						label="Plan subagent model"
						description="Deployment-wide model override for delegated child chats that continue running in plan mode. Root plan mode chats still keep their own selected model."
						level="section"
					/>
					<SubagentModelOverrideSettings
						title="Plan subagent model"
						description="Deployment-wide model override for delegated child chats that continue running in plan mode. Root plan mode chats still keep their own selected model."
						modelOverrideData={planSubagentModelOverrideData}
						enabledModelConfigs={enabledModelConfigs}
						modelConfigsError={modelConfigsError}
						isLoading={isLoadingModelConfigs}
						onSaveModelOverride={onSavePlanSubagentModelOverride}
						isSaving={isSavingPlanSubagentModelOverride}
						isSaveError={isSavePlanSubagentModelOverrideError}
						saveErrorMessage="Failed to save plan subagent model override."
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
			{showComputerUseModelSection && onSaveComputerUseModelOverride && (
				<section
					aria-label="Computer use subagent model"
					className="flex flex-col gap-3"
				>
					<SectionHeader
						label="Computer use subagent model"
						description="Deployment-wide model override for computer use subagents. Desktop support must stay enabled, and incompatible providers still fall back to the Anthropic computer-use default."
						level="section"
					/>
					<SubagentModelOverrideSettings
						title="Computer use subagent model"
						description="Deployment-wide model override for computer use subagents. Desktop support must stay enabled, and incompatible providers still fall back to the Anthropic computer-use default."
						modelOverrideData={computerUseModelOverrideData}
						enabledModelConfigs={enabledModelConfigs}
						modelConfigsError={modelConfigsError}
						isLoading={isLoadingModelConfigs}
						onSaveModelOverride={onSaveComputerUseModelOverride}
						isSaving={isSavingComputerUseModelOverride}
						isSaveError={isSaveComputerUseModelOverrideError}
						saveErrorMessage="Failed to save computer use model override."
						showHeader={false}
					/>
				</section>
			)}
		</div>
	);
};
