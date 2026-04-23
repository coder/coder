import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatAgentModelOverrideQuery,
	chatModelConfigs,
	updateChatAgentModelOverrideMutation,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AgentSettingsAgentsPageView } from "./AgentSettingsAgentsPageView";

const generalOverrideContext: TypesGen.ChatAgentModelOverrideContext =
	"general";
const exploreOverrideContext: TypesGen.ChatAgentModelOverrideContext =
	"explore";

const AgentSettingsAgentsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const canEditDeploymentConfig = permissions.editDeploymentConfig;

	const generalModelOverrideQuery = useQuery({
		...chatAgentModelOverrideQuery(generalOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const exploreModelOverrideQuery = useQuery({
		...chatAgentModelOverrideQuery(exploreOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const saveGeneralModelOverrideMutation = useMutation(
		updateChatAgentModelOverrideMutation(queryClient, generalOverrideContext),
	);
	const saveExploreModelOverrideMutation = useMutation(
		updateChatAgentModelOverrideMutation(queryClient, exploreOverrideContext),
	);

	return (
		<RequirePermission isFeatureVisible={canEditDeploymentConfig}>
			<AgentSettingsAgentsPageView
				generalModelOverrideData={generalModelOverrideQuery.data}
				exploreModelOverrideData={exploreModelOverrideQuery.data}
				modelConfigsData={modelConfigsQuery.data}
				modelConfigsError={modelConfigsQuery.error}
				isLoadingModelConfigs={modelConfigsQuery.isLoading}
				onSaveGeneralModelOverride={saveGeneralModelOverrideMutation.mutate}
				isSavingGeneralModelOverride={
					saveGeneralModelOverrideMutation.isPending
				}
				isSaveGeneralModelOverrideError={
					saveGeneralModelOverrideMutation.isError
				}
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
