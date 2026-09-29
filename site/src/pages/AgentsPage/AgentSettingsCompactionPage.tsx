import { useMutation, useQueries, useQuery, useQueryClient } from "react-query";
import {
	deleteUserCompactionThreshold,
	organizationChatModelOverrides,
	updateUserCompactionThreshold,
	userChatProviderConfigs,
	userCompactionThresholds,
} from "#/api/queries/chats";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { AgentSettingsCompactionPageView } from "./AgentSettingsCompactionPageView";
import { useOrganizationChatModels } from "./hooks/useOrganizationChatModels";
import { providerTypeByIDFromUserConfigs } from "./utils/modelOptions";

const AgentSettingsCompactionPage: React.FC = () => {
	const queryClient = useQueryClient();
	const { organizations } = useDashboard();
	const organizationModels = useOrganizationChatModels(
		organizations.map((organization) => organization.id),
	);
	const providerConfigsQuery = useQuery(userChatProviderConfigs());
	const thresholdsQuery = useQuery(userCompactionThresholds());
	// Only refines the displayed trigger point; a failed request falls back
	// to the chat model's own window.
	const modelOverrideQueries = useQueries({
		queries: organizations.map((organization) =>
			organizationChatModelOverrides(organization.id),
		),
	});
	const compactionModelIDByOrganization = new Map<string, string>();
	for (const [index, query] of modelOverrideQueries.entries()) {
		const compactionOverride = query.data?.overrides.find(
			(override) => override.context === "compaction",
		);
		if (compactionOverride) {
			compactionModelIDByOrganization.set(
				organizations[index].id,
				compactionOverride.model_config_id,
			);
		}
	}
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
			organizations={organizations}
			compactionModelIDByOrganization={compactionModelIDByOrganization}
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
