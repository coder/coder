import type { FC } from "react";
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
import {
	type OrganizationCompactionTrigger,
	resolveOrganizationCompactionTrigger,
} from "./compactionTriggers";
import { useOrganizationChatModels } from "./hooks/useOrganizationChatModels";
import { providerTypeByIDFromUserConfigs } from "./utils/modelOptions";

const AgentSettingsCompactionPage: FC = () => {
	const queryClient = useQueryClient();
	const { organizations } = useDashboard();
	const organizationModels = useOrganizationChatModels(
		organizations.map((organization) => organization.id),
	);
	const compactionOverrideQueries = useQueries({
		queries: organizations.map((organization) =>
			organizationChatModelOverrides(organization.id),
		),
	});
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
	const compactionTriggersByOrganizationID = new Map<
		string,
		OrganizationCompactionTrigger
	>();
	for (const [index, organization] of organizations.entries()) {
		const trigger = resolveOrganizationCompactionTrigger(
			compactionOverrideQueries[index]?.data?.overrides,
			organizationModels.models.filter(
				(model) => model.organization_id === organization.id,
			),
		);
		if (trigger) {
			compactionTriggersByOrganizationID.set(organization.id, trigger);
		}
	}

	return (
		<AgentSettingsCompactionPageView
			models={organizationModels.models}
			providerTypeByID={providerTypeByID}
			organizations={organizations}
			compactionTriggersByOrganizationID={compactionTriggersByOrganizationID}
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
