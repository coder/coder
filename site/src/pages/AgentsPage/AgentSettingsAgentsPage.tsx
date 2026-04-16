import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatExploreModelOverride,
	chatModelConfigs,
	updateChatExploreModelOverride,
} from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AgentSettingsAgentsPageView } from "./AgentSettingsAgentsPageView";

const AgentSettingsAgentsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();

	const exploreModelOverrideQuery = useQuery({
		...chatExploreModelOverride(),
		enabled: permissions.editDeploymentConfig,
	});
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const saveExploreModelOverrideMutation = useMutation(
		updateChatExploreModelOverride(queryClient),
	);

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<AgentSettingsAgentsPageView
				exploreModelOverrideData={exploreModelOverrideQuery.data}
				modelConfigsData={modelConfigsQuery.data}
				modelConfigsError={modelConfigsQuery.error}
				isLoadingModelConfigs={modelConfigsQuery.isLoading}
				onSaveExploreModelOverride={saveExploreModelOverrideMutation.mutate}
				isSavingExploreModelOverride={
					saveExploreModelOverrideMutation.isPending
				}
				isSaveExploreModelOverrideError={
					saveExploreModelOverrideMutation.isError
				}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsAgentsPage;
