import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatPersonalMemorySettings,
	updateChatPersonalMemorySettings,
} from "#/api/queries/chatMemorySettings";
import {
	chatUserCustomPrompt,
	updateUserChatCustomPrompt,
	updateUserChatDebugLogging,
	userChatDebugLogging,
} from "#/api/queries/chats";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { AgentSettingsGeneralPageView } from "./AgentSettingsGeneralPageView";

const AgentSettingsGeneralPage: FC = () => {
	const queryClient = useQueryClient();
	const { experiments } = useDashboard();
	const personalMemoryQuery = useQuery({
		...chatPersonalMemorySettings(),
		enabled: experiments.includes("chat-projects"),
	});
	const savePersonalMemoryMutation = useMutation(
		updateChatPersonalMemorySettings(queryClient),
	);
	const userPromptQuery = useQuery(chatUserCustomPrompt());
	const userDebugLoggingQuery = useQuery(userChatDebugLogging());
	const saveUserPromptMutation = useMutation(
		updateUserChatCustomPrompt(queryClient),
	);
	const saveUserDebugLoggingMutation = useMutation(
		updateUserChatDebugLogging(queryClient),
	);

	return (
		<AgentSettingsGeneralPageView
			userPromptData={userPromptQuery.data}
			onSaveUserPrompt={saveUserPromptMutation.mutate}
			isSavingUserPrompt={saveUserPromptMutation.isPending}
			isSaveUserPromptError={saveUserPromptMutation.isError}
			userDebugLoggingData={userDebugLoggingQuery.data}
			onSaveUserDebugLogging={saveUserDebugLoggingMutation.mutate}
			isSavingUserDebugLogging={saveUserDebugLoggingMutation.isPending}
			isSaveUserDebugLoggingError={saveUserDebugLoggingMutation.isError}
			personalMemoryData={personalMemoryQuery.data}
			onSavePersonalMemory={savePersonalMemoryMutation.mutate}
			isSavingPersonalMemory={savePersonalMemoryMutation.isPending}
			isSavePersonalMemoryError={savePersonalMemoryMutation.isError}
		/>
	);
};

export default AgentSettingsGeneralPage;
