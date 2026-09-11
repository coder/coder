import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatUserCustomPrompt,
	updateUserChatCustomPrompt,
	updateUserChatDebugLogging,
	userChatDebugLogging,
} from "#/api/queries/chats";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { AgentSettingsGeneralPageView } from "./AgentSettingsGeneralPageView";
import { VIM_NAVIGATION_EXPERIMENT } from "./hooks/useVimNavigation";

const AgentSettingsGeneralPage: FC = () => {
	const queryClient = useQueryClient();
	const { experiments } = useDashboard();
	const showVimNavigationSettings = experiments.includes(
		VIM_NAVIGATION_EXPERIMENT,
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
			showVimNavigationSettings={showVimNavigationSettings}
		/>
	);
};

export default AgentSettingsGeneralPage;
