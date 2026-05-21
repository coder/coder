import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatModelConfigs,
	deleteUserCompactionThreshold,
	updateUserCompactionThreshold,
	userCompactionThresholds,
} from "#/api/queries/chats";
import { AgentSettingsCompactionPageView } from "./AgentSettingsCompactionPageView";

const AgentSettingsCompactionPage: FC = () => {
	const queryClient = useQueryClient();
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
		<AgentSettingsCompactionPageView
			modelConfigsData={modelConfigsQuery.data}
			modelConfigsError={modelConfigsQuery.error}
			isLoadingModelConfigs={modelConfigsQuery.isLoading}
			thresholds={thresholdsQuery.data?.thresholds}
			isThresholdsLoading={thresholdsQuery.isLoading}
			thresholdsError={thresholdsQuery.error}
			onSaveThreshold={handleSaveThreshold}
			onResetThreshold={handleResetThreshold}
		/>
	);
};

export default AgentSettingsCompactionPage;
