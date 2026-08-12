import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatModelConfigs,
	deleteUserCompactionThreshold,
	updateUserCompactionThreshold,
	userChatProviderConfigs,
	userCompactionThresholds,
} from "#/api/queries/chats";
import { AgentSettingsCompactionPageView } from "./AgentSettingsCompactionPageView";
import { providerTypeByIDFromUserConfigs } from "./utils/modelOptions";

const AgentSettingsCompactionPage: FC = () => {
	const queryClient = useQueryClient();
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const providerConfigsQuery = useQuery(userChatProviderConfigs());
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

	const providerTypeByID = providerTypeByIDFromUserConfigs(
		providerConfigsQuery.data,
	);

	return (
		<AgentSettingsCompactionPageView
			modelConfigsData={modelConfigsQuery.data}
			providerTypeByID={providerTypeByID}
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
