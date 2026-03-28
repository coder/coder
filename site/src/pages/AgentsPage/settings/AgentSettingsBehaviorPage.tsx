import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatDesktopEnabled,
	chatModelConfigs,
	chatSystemPrompt,
	chatUserCustomPrompt,
	chatWorkspaceTTL,
	deleteUserCompactionThreshold,
	updateChatDesktopEnabled,
	updateChatSystemPrompt,
	updateChatWorkspaceTTL,
	updateUserChatCustomPrompt,
	updateUserCompactionThreshold,
	userCompactionThresholds,
} from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { AgentSettingsBehaviorPageView } from "./AgentSettingsBehaviorPageView";

const AgentSettingsBehaviorPage: FC = () => {
	const { permissions } = useAuthenticated();
	const canSetSystemPrompt = permissions.editDeploymentConfig;
	const queryClient = useQueryClient();

	const systemPromptQuery = useQuery({
		...chatSystemPrompt(),
		enabled: canSetSystemPrompt,
	});
	const saveSystemPromptMutation = useMutation(
		updateChatSystemPrompt(queryClient),
	);

	const userPromptQuery = useQuery(chatUserCustomPrompt());
	const saveUserPromptMutation = useMutation(
		updateUserChatCustomPrompt(queryClient),
	);

	const desktopEnabledQuery = useQuery(chatDesktopEnabled());
	const saveDesktopEnabledMutation = useMutation(
		updateChatDesktopEnabled(queryClient),
	);

	const workspaceTTLQuery = useQuery(chatWorkspaceTTL());
	const saveWorkspaceTTLMutation = useMutation(
		updateChatWorkspaceTTL(queryClient),
	);

	const modelConfigsQuery = useQuery(chatModelConfigs());

	const compactionThresholdsQuery = useQuery(userCompactionThresholds());
	const updateCompactionThresholdMutation = useMutation(
		updateUserCompactionThreshold(queryClient),
	);
	const deleteCompactionThresholdMutation = useMutation(
		deleteUserCompactionThreshold(queryClient),
	);

	return (
		<AgentSettingsBehaviorPageView
			canSetSystemPrompt={canSetSystemPrompt}
			systemPromptQuery={systemPromptQuery}
			saveSystemPromptMutation={saveSystemPromptMutation}
			userPromptQuery={userPromptQuery}
			saveUserPromptMutation={saveUserPromptMutation}
			desktopEnabledQuery={desktopEnabledQuery}
			saveDesktopEnabledMutation={saveDesktopEnabledMutation}
			workspaceTTLQuery={workspaceTTLQuery}
			saveWorkspaceTTLMutation={saveWorkspaceTTLMutation}
			modelConfigsQuery={modelConfigsQuery}
			compactionThresholdsQuery={compactionThresholdsQuery}
			updateCompactionThresholdMutation={updateCompactionThresholdMutation}
			deleteCompactionThresholdMutation={deleteCompactionThresholdMutation}
		/>
	);
};

export default AgentSettingsBehaviorPage;
