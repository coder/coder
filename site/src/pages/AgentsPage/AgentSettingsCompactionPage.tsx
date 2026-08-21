import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatModels,
	deleteUserCompactionThreshold,
	updateUserCompactionThreshold,
	userChatProviderConfigs,
	userCompactionThresholds,
} from "#/api/queries/chats";
import { AgentSettingsCompactionPageView } from "./AgentSettingsCompactionPageView";
import { providerTypeByIDFromUserConfigs } from "./utils/modelOptions";

const AgentSettingsCompactionPage: FC = () => {
	const queryClient = useQueryClient();
	const modelsQuery = useQuery(chatModels());
	const providerConfigsQuery = useQuery(userChatProviderConfigs());
	const thresholdsQuery = useQuery(userCompactionThresholds());
	const saveThresholdMutation = useMutation(
		updateUserCompactionThreshold(queryClient),
	);
	const resetThresholdMutation = useMutation(
		deleteUserCompactionThreshold(queryClient),
	);

	const handleSaveThreshold = (modelId: string, thresholdPercent: number) =>
		saveThresholdMutation.mutateAsync({
			modelId,
			req: { threshold_percent: thresholdPercent },
		});

	const handleResetThreshold = (modelId: string) =>
		resetThresholdMutation.mutateAsync(modelId);

	const providerTypeByID = providerTypeByIDFromUserConfigs(
		providerConfigsQuery.data,
	);

	return (
		<AgentSettingsCompactionPageView
			models={modelsQuery.data}
			providerTypeByID={providerTypeByID}
			modelsError={modelsQuery.error}
			isLoadingModels={modelsQuery.isLoading}
			thresholds={thresholdsQuery.data?.thresholds}
			isThresholdsLoading={thresholdsQuery.isLoading}
			thresholdsError={thresholdsQuery.error}
			onSaveThreshold={handleSaveThreshold}
			onResetThreshold={handleResetThreshold}
		/>
	);
};

export default AgentSettingsCompactionPage;
