import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	deleteUserCompactionThreshold,
	updateUserCompactionThreshold,
	userChatProviderConfigs,
	userCompactionThresholds,
} from "#/api/queries/chats";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useOrganizationChatModels } from "../../hooks/useOrganizationChatModels";
import { providerTypeByIDFromUserConfigs } from "../../utils/modelOptions";
import { AgentSettingsCompactionPageView } from "./AgentSettingsCompactionPageView";

const AgentSettingsCompactionPage: FC = () => {
	const queryClient = useQueryClient();
	const { organizations } = useDashboard();
	const organizationModels = useOrganizationChatModels(
		organizations.map((organization) => organization.id),
	);
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
			models={organizationModels.models}
			providerTypeByID={providerTypeByID}
			organizationNameByID={
				new Map(
					organizations.map((organization) => [
						organization.id,
						organization.display_name || organization.name,
					]),
				)
			}
			modelsError={organizationModels.error ?? organizationModels.partialError}
			isLoadingModels={organizationModels.isLoading}
			thresholds={thresholdsQuery.data?.thresholds}
			isThresholdsLoading={thresholdsQuery.isLoading}
			thresholdsError={thresholdsQuery.error}
			onSaveThreshold={handleSaveThreshold}
			onResetThreshold={handleResetThreshold}
		/>
	);
};

export default AgentSettingsCompactionPage;
