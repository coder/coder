import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { AdminBadge } from "./components/AdminBadge";
import { PlanModeInstructionsSettings } from "./components/PlanModeInstructionsSettings";
import { SectionHeader } from "./components/SectionHeader";
import { SystemInstructionsSettings } from "./components/SystemInstructionsSettings";
import type { MutationCallbacks } from "./types";

export interface AgentSettingsSystemInstructionsPageViewProps {
	systemPromptData: TypesGen.ChatSystemPromptResponse | undefined;
	planModeInstructionsData:
		| TypesGen.ChatPlanModeInstructionsResponse
		| undefined;
	onSaveSystemPrompt: (
		req: TypesGen.UpdateChatSystemPromptRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingSystemPrompt: boolean;
	isSaveSystemPromptError: boolean;
	onSavePlanModeInstructions: (
		req: TypesGen.UpdateChatPlanModeInstructionsRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingPlanModeInstructions: boolean;
	isSavePlanModeInstructionsError: boolean;
}

export const AgentSettingsSystemInstructionsPageView: FC<
	AgentSettingsSystemInstructionsPageViewProps
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
				label="System Instructions"
				description="Control the system prompts and plan mode instructions used across the deployment."
				badge={<AdminBadge />}
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
