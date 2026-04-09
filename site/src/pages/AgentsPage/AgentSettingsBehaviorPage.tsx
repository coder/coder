import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatDesktopEnabled,
	chatModelConfigs,
	chatRetentionDays,
	chatSystemPrompt,
	chatUserCustomPrompt,
	chatWorkspaceTTL,
	deleteUserCompactionThreshold,
	updateChatDesktopEnabled,
	updateChatRetentionDays,
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
	const queryClient = useQueryClient();

	const systemPromptQuery = useQuery({
		...chatSystemPrompt(),
		enabled: permissions.editDeploymentConfig,
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

	const retentionDaysQuery = useQuery(chatRetentionDays());
	const saveRetentionDaysMutation = useMutation(
		updateChatRetentionDays(queryClient),
	);

	const modelConfigsQuery = useQuery(chatModelConfigs());

	const thresholdsQuery = useQuery(userCompactionThresholds());
	const saveThresholdMutation = useMutation(
		updateUserCompactionThreshold(queryClient),
	);
	const resetThresholdMutation = useMutation(
		deleteUserCompactionThreshold(queryClient),
	);

	const handleSaveThreshold = (
		modelConfigId: string,
		thresholdPercent: number,
	) =>
		saveThresholdMutation.mutateAsync({
			modelConfigId,
			req: { threshold_percent: thresholdPercent },
		});

	const handleResetThreshold = (modelConfigId: string) =>
		resetThresholdMutation.mutateAsync(modelConfigId);

	return (
		<AgentSettingsBehaviorPageView
			canSetSystemPrompt={permissions.editDeploymentConfig}
			systemPromptData={systemPromptQuery.data}
			userPromptData={userPromptQuery.data}
			desktopEnabledData={desktopEnabledQuery.data}
			workspaceTTLData={workspaceTTLQuery.data}
			isWorkspaceTTLLoading={workspaceTTLQuery.isLoading}
			isWorkspaceTTLLoadError={workspaceTTLQuery.isError}
			modelConfigsData={modelConfigsQuery.data}
			modelConfigsError={modelConfigsQuery.error}
			isLoadingModelConfigs={modelConfigsQuery.isLoading}
			thresholds={thresholdsQuery.data?.thresholds}
			isThresholdsLoading={thresholdsQuery.isLoading}
			thresholdsError={thresholdsQuery.error}
			onSaveThreshold={handleSaveThreshold}
			onResetThreshold={handleResetThreshold}
			onSaveSystemPrompt={saveSystemPromptMutation.mutate}
			isSavingSystemPrompt={saveSystemPromptMutation.isPending}
			isSaveSystemPromptError={saveSystemPromptMutation.isError}
			onSaveUserPrompt={saveUserPromptMutation.mutate}
			isSavingUserPrompt={saveUserPromptMutation.isPending}
			isSaveUserPromptError={saveUserPromptMutation.isError}
			onSaveDesktopEnabled={saveDesktopEnabledMutation.mutate}
			isSavingDesktopEnabled={saveDesktopEnabledMutation.isPending}
			isSaveDesktopEnabledError={saveDesktopEnabledMutation.isError}
			onSaveWorkspaceTTL={saveWorkspaceTTLMutation.mutate}
			isSavingWorkspaceTTL={saveWorkspaceTTLMutation.isPending}
			isSaveWorkspaceTTLError={saveWorkspaceTTLMutation.isError}
			retentionDaysData={retentionDaysQuery.data}
			isRetentionDaysLoading={retentionDaysQuery.isLoading}
			isRetentionDaysLoadError={retentionDaysQuery.isError}
			onSaveRetentionDays={saveRetentionDaysMutation.mutate}
			isSavingRetentionDays={saveRetentionDaysMutation.isPending}
			isSaveRetentionDaysError={saveRetentionDaysMutation.isError}
		/>
	);
};

export default AgentSettingsBehaviorPage;
