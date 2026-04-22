import type { FC } from "react";
import type { UseMutateFunction } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import { PlanModeInstructionsSettings } from "./components/PlanModeInstructionsSettings";
import { SectionHeader } from "./components/SectionHeader";
import { SystemInstructionsSettings } from "./components/SystemInstructionsSettings";

export interface AgentSettingsInstructionsPageViewProps {
	systemPromptData: TypesGen.ChatSystemPromptResponse | undefined;
	planModeInstructionsData:
		| TypesGen.ChatPlanModeInstructionsResponse
		| undefined;
	onSaveSystemPrompt: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateChatSystemPromptRequest,
		unknown
	>;
	isSavingSystemPrompt: boolean;
	isSaveSystemPromptError: boolean;
	onSavePlanModeInstructions: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateChatPlanModeInstructionsRequest,
		unknown
	>;
	isSavingPlanModeInstructions: boolean;
	isSavePlanModeInstructionsError: boolean;
}

export const AgentSettingsInstructionsPageView: FC<
	AgentSettingsInstructionsPageViewProps
> = ({
	systemPromptData,
	planModeInstructionsData,
	onSaveSystemPrompt,
	isSavingSystemPrompt,
	isSaveSystemPromptError,
	onSavePlanModeInstructions,
	isSavingPlanModeInstructions,
	isSavePlanModeInstructionsError,
}) => {
	const isAnyPromptSaving =
		isSavingSystemPrompt || isSavingPlanModeInstructions;

	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label="Instructions"
				description="Control the system prompts and plan mode instructions used across the deployment."
			/>
			<SystemInstructionsSettings
				systemPromptData={systemPromptData}
				onSaveSystemPrompt={onSaveSystemPrompt}
				isSavingSystemPrompt={isSavingSystemPrompt}
				isSaveSystemPromptError={isSaveSystemPromptError}
				isAnyPromptSaving={isAnyPromptSaving}
			/>
			<PlanModeInstructionsSettings
				planModeInstructionsData={planModeInstructionsData}
				onSavePlanModeInstructions={onSavePlanModeInstructions}
				isSavePlanModeInstructionsError={isSavePlanModeInstructionsError}
				isAnyPromptSaving={isAnyPromptSaving}
			/>
		</div>
	);
};
