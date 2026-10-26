import { useMutation, useQueries, useQuery, useQueryClient } from "react-query";
import {
	deleteUserCompactionThreshold,
	organizationChatModelOverrides,
	updateUserCompactionThreshold,
	userChatProviderConfigs,
	userCompactionThresholds,
} from "#/api/queries/chats";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useOrganizationChatModels } from "../../hooks/useOrganizationChatModels";
import { resolveCompactionTriggersByOrganization } from "../../utils/compactionTriggers";
import {
	providerInfoByIDFromDescriptors,
	providerInfoByIDFromUserConfigs,
} from "../../utils/modelOptions";
import { AgentSettingsCompactionPageView } from "./AgentSettingsCompactionPageView";

const AgentSettingsCompactionPage: React.FC = () => {
	const queryClient = useQueryClient();
	const { organizations } = useDashboard();
	const organizationModels = useOrganizationChatModels(
		organizations.map((organization) => organization.id),
	);
	const providerConfigsQuery = useQuery(userChatProviderConfigs());
	const thresholdsQuery = useQuery(userCompactionThresholds());
	const modelOverrideQueries = useQueries({
		queries: organizations.map((organization) =>
			organizationChatModelOverrides(organization.id),
		),
	});
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

	const providerInfoByID = providerInfoByIDFromUserConfigs(
		providerConfigsQuery.data,
	);
	const compactionTriggers = resolveCompactionTriggersByOrganization(
		organizations.map((organization, index) => ({
			organizationID: organization.id,
			data: modelOverrideQueries[index].data,
			error: modelOverrideQueries[index].error,
		})),
		organizationModels.models,
		providerInfoByIDFromDescriptors(organizationModels.providers),
	);

	return (
		<AgentSettingsCompactionPageView
			models={organizationModels.models}
			providerInfoByID={providerInfoByID}
			organizations={organizations}
			compactionTriggersByOrganizationID={
				compactionTriggers.triggersByOrganizationID
			}
			modelsError={organizationModels.error ?? organizationModels.partialError}
			isLoadingModels={
				organizationModels.isLoading ||
				modelOverrideQueries.some((query) => query.isLoading)
			}
			compactionTriggerLoadErrors={compactionTriggers.loadErrors}
			thresholds={thresholdsQuery.data?.thresholds}
			isThresholdsLoading={thresholdsQuery.isLoading}
			thresholdsError={thresholdsQuery.error}
			onSaveThreshold={handleSaveThreshold}
			onResetThreshold={handleResetThreshold}
		/>
	);
};

export default AgentSettingsCompactionPage;
