import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatDebugLogging,
	chatDesktopEnabled,
	chatModelConfigs,
	chatPlanModeInstructions,
	chatRetentionDays,
	chatSystemPrompt,
	chatUserCustomPrompt,
	chatWorkspaceTTL,
	deleteUserCompactionThreshold,
	updateChatDebugLogging,
	updateChatDesktopEnabled,
	updateChatPlanModeInstructions,
	updateChatRetentionDays,
	updateChatSystemPrompt,
	updateChatWorkspaceTTL,
	updateUserChatCustomPrompt,
	updateUserChatDebugLogging,
	updateUserCompactionThreshold,
	userChatDebugLogging,
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
	const planModeInstructionsQuery = useQuery({
		...chatPlanModeInstructions(),
		enabled: permissions.editDeploymentConfig,
	});
	const savePlanModeInstructionsMutation = useMutation(
		updateChatPlanModeInstructions(queryClient),
	);

	const userPromptQuery = useQuery(chatUserCustomPrompt());
	const saveUserPromptMutation = useMutation(
		updateUserChatCustomPrompt(queryClient),
	);

	const desktopEnabledQuery = useQuery(chatDesktopEnabled());
	const saveDesktopEnabledMutation = useMutation(
		updateChatDesktopEnabled(queryClient),
	);

	const debugLoggingQuery = useQuery({
		...chatDebugLogging(),
		enabled: permissions.editDeploymentConfig,
	});
	const saveDebugLoggingMutation = useMutation(
		updateChatDebugLogging(queryClient),
	);

	const userDebugLoggingQuery = useQuery(userChatDebugLogging());
	const saveUserDebugLoggingMutation = useMutation(
		updateUserChatDebugLogging(queryClient),
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
			planModeInstructionsData={planModeInstructionsQuery.data}
			userPromptData={userPromptQuery.data}
			desktopEnabledData={desktopEnabledQuery.data}
			debugLoggingData={debugLoggingQuery.data}
			userDebugLoggingData={userDebugLoggingQuery.data}
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
			onSavePlanModeInstructions={savePlanModeInstructionsMutation.mutate}
			isSavingPlanModeInstructions={savePlanModeInstructionsMutation.isPending}
			isSavePlanModeInstructionsError={savePlanModeInstructionsMutation.isError}
			onSaveUserPrompt={saveUserPromptMutation.mutate}
			isSavingUserPrompt={saveUserPromptMutation.isPending}
			isSaveUserPromptError={saveUserPromptMutation.isError}
			onSaveDesktopEnabled={saveDesktopEnabledMutation.mutate}
			isSavingDesktopEnabled={saveDesktopEnabledMutation.isPending}
			isSaveDesktopEnabledError={saveDesktopEnabledMutation.isError}
			onSaveDebugLogging={saveDebugLoggingMutation.mutate}
			isSavingDebugLogging={saveDebugLoggingMutation.isPending}
			isSaveDebugLoggingError={saveDebugLoggingMutation.isError}
			onSaveUserDebugLogging={saveUserDebugLoggingMutation.mutate}
			isSavingUserDebugLogging={saveUserDebugLoggingMutation.isPending}
			isSaveUserDebugLoggingError={saveUserDebugLoggingMutation.isError}
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
