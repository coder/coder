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
const planSubagentOverrideContext: TypesGen.ChatAgentModelOverrideContext =
	"plan_subagent";
const exploreOverrideContext: TypesGen.ChatAgentModelOverrideContext =
	"explore";
const computerUseOverrideContext: TypesGen.ChatAgentModelOverrideContext =
	"computer_use";

const AgentSettingsAgentsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const canEditDeploymentConfig = permissions.editDeploymentConfig;

	const generalModelOverrideQuery = useQuery({
		...chatAgentModelOverrideQuery(generalOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const planSubagentModelOverrideQuery = useQuery({
		...chatAgentModelOverrideQuery(planSubagentOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const exploreModelOverrideQuery = useQuery({
		...chatAgentModelOverrideQuery(exploreOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const computerUseModelOverrideQuery = useQuery({
		...chatAgentModelOverrideQuery(computerUseOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const saveGeneralModelOverrideMutation = useMutation(
		updateChatAgentModelOverrideMutation(queryClient, generalOverrideContext),
	);
	const savePlanSubagentModelOverrideMutation = useMutation(
		updateChatAgentModelOverrideMutation(
			queryClient,
			planSubagentOverrideContext,
		),
	);
	const saveExploreModelOverrideMutation = useMutation(
		updateChatAgentModelOverrideMutation(queryClient, exploreOverrideContext),
	);
	const saveComputerUseModelOverrideMutation = useMutation(
		updateChatAgentModelOverrideMutation(
			queryClient,
			computerUseOverrideContext,
		),
	);

	return (
		<RequirePermission isFeatureVisible={canEditDeploymentConfig}>
			<AgentSettingsAgentsPageView
				generalModelOverrideData={generalModelOverrideQuery.data}
				planSubagentModelOverrideData={planSubagentModelOverrideQuery.data}
				exploreModelOverrideData={exploreModelOverrideQuery.data}
				computerUseModelOverrideData={computerUseModelOverrideQuery.data}
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
				onSavePlanSubagentModelOverride={
					savePlanSubagentModelOverrideMutation.mutate
				}
				isSavingPlanSubagentModelOverride={
					savePlanSubagentModelOverrideMutation.isPending
				}
				isSavePlanSubagentModelOverrideError={
					savePlanSubagentModelOverrideMutation.isError
				}
				onSaveExploreModelOverride={saveExploreModelOverrideMutation.mutate}
				isSavingExploreModelOverride={
					saveExploreModelOverrideMutation.isPending
				}
				isSaveExploreModelOverrideError={
					saveExploreModelOverrideMutation.isError
				}
				onSaveComputerUseModelOverride={
					saveComputerUseModelOverrideMutation.mutate
				}
				isSavingComputerUseModelOverride={
					saveComputerUseModelOverrideMutation.isPending
				}
				isSaveComputerUseModelOverrideError={
					saveComputerUseModelOverrideMutation.isError
				}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsAgentsPage;
