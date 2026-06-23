import type { FC } from "react";
import type { UseMutateFunction } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import { PlanModeInstructionsSettings } from "./components/PlanModeInstructionsSettings";
import { SystemInstructionsSettings } from "./components/SystemInstructionsSettings";

export interface InstructionsPageViewProps {
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

export const InstructionsPageView: FC<InstructionsPageViewProps> = ({
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
		<div className="flex max-w-4xl flex-col gap-8">
			<div className="flex flex-col gap-2">
				<h1 className="m-0 font-sans text-[32px] font-semibold leading-[40px] text-content-primary">
					Instructions
				</h1>
				<p className="m-0 font-sans text-sm font-normal leading-6 text-content-secondary">
					Control the system prompts and plan mode instructions used across the
					deployment.
				</p>
			</div>
			<div className="flex flex-col gap-8">
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
		</div>
	);
};
