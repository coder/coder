import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatUserCustomPrompt,
	updateUserChatCustomPrompt,
} from "#/api/queries/chats";
import { AgentSettingsGeneralPageView } from "./AgentSettingsGeneralPageView";

const AgentSettingsGeneralPage: FC = () => {
	const queryClient = useQueryClient();
	const userPromptQuery = useQuery(chatUserCustomPrompt());
	const saveUserPromptMutation = useMutation(
		updateUserChatCustomPrompt(queryClient),
	);

	return (
		<AgentSettingsGeneralPageView
			userPromptData={userPromptQuery.data}
			onSaveUserPrompt={saveUserPromptMutation.mutate}
			isSavingUserPrompt={saveUserPromptMutation.isPending}
			isSaveUserPromptError={saveUserPromptMutation.isError}
		/>
	);
};

export default AgentSettingsGeneralPage;
