import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatUserCustomPrompt,
	updateUserChatCustomPrompt,
	updateUserChatDebugLogging,
	userChatDebugLogging,
} from "#/api/queries/chats";
import { AgentSettingsGeneralPageView } from "./AgentSettingsGeneralPageView";

const AgentSettingsGeneralPage: FC = () => {
	const queryClient = useQueryClient();
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
		/>
	);
};

export default AgentSettingsGeneralPage;
