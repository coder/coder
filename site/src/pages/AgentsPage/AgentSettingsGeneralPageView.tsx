import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ChatFullWidthSettings } from "./components/ChatFullWidthSettings";
import { PersonalInstructionsSettings } from "./components/PersonalInstructionsSettings";
import { SectionHeader } from "./components/SectionHeader";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

export interface AgentSettingsGeneralPageViewProps {
	userPromptData: TypesGen.UserChatCustomPrompt | undefined;
	onSaveUserPrompt: (
		req: TypesGen.UserChatCustomPrompt,
		options?: MutationCallbacks,
	) => void;
	isSavingUserPrompt: boolean;
	isSaveUserPromptError: boolean;
}

export const AgentSettingsGeneralPageView: FC<
	AgentSettingsGeneralPageViewProps
> = ({
	userPromptData,
	onSaveUserPrompt,
	isSavingUserPrompt,
	isSaveUserPromptError,
}) => {
	const isAnyPromptSaving = isSavingUserPrompt;

	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label="General"
				description="Personal preferences for your chat experience."
			/>
			<PersonalInstructionsSettings
				userPromptData={userPromptData}
				onSaveUserPrompt={onSaveUserPrompt}
				isSavingUserPrompt={isSavingUserPrompt}
				isSaveUserPromptError={isSaveUserPromptError}
				isAnyPromptSaving={isAnyPromptSaving}
			/>
			<ChatFullWidthSettings />
		</div>
	);
};
