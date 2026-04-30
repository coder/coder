import type { FC } from "react";
import {
	type QueryClient,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { API } from "#/api/api";
import { chatModelConfigs } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AgentSettingsAgentsPageView } from "./AgentSettingsAgentsPageView";

const generalOverrideContext: TypesGen.ChatAgentModelOverrideContext =
	"general";
const exploreOverrideContext: TypesGen.ChatAgentModelOverrideContext =
	"explore";

const chatTitleGenerationModelOverrideKey = [
	"chat-title-generation-model-override",
] as const;

const chatAgentModelOverrideKey = (
	context: TypesGen.ChatAgentModelOverrideContext,
) => ["chat-agent-model-override", context] as const;

const chatAgentModelOverrideQuery = (
	context: TypesGen.ChatAgentModelOverrideContext,
) => ({
	queryKey: chatAgentModelOverrideKey(context),
	queryFn: () => API.experimental.getChatAgentModelOverride(context),
});

const chatTitleGenerationModelOverrideQuery = () => ({
	queryKey: chatTitleGenerationModelOverrideKey,
	queryFn: () => API.experimental.getChatTitleGenerationModelOverride(),
});

const updateChatAgentModelOverrideMutation = (
	queryClient: QueryClient,
	context: TypesGen.ChatAgentModelOverrideContext,
) => ({
	mutationFn: (req: TypesGen.UpdateChatAgentModelOverrideRequest) =>
		API.experimental.updateChatAgentModelOverride(context, req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatAgentModelOverrideKey(context),
			exact: true,
		});
	},
});

const updateChatTitleGenerationModelOverrideMutation = (
	queryClient: QueryClient,
) => ({
	mutationFn: (req: TypesGen.UpdateChatTitleGenerationModelOverrideRequest) =>
		API.experimental.updateChatTitleGenerationModelOverride(req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatTitleGenerationModelOverrideKey,
			exact: true,
		});
	},
});

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
	const titleGenerationModelQuery = useQuery({
		...chatTitleGenerationModelOverrideQuery(),
		enabled: canEditDeploymentConfig,
	});
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const saveGeneralModelOverrideMutation = useMutation(
		updateChatAgentModelOverrideMutation(queryClient, generalOverrideContext),
	);
	const saveTitleGenerationModelMutation = useMutation(
		updateChatTitleGenerationModelOverrideMutation(queryClient),
	);
	const saveExploreModelOverrideMutation = useMutation(
		updateChatAgentModelOverrideMutation(queryClient, exploreOverrideContext),
	);

	return (
		<RequirePermission isFeatureVisible={canEditDeploymentConfig}>
			<AgentSettingsAgentsPageView
				generalModelOverrideData={generalModelOverrideQuery.data}
				titleGenerationModelOverrideData={titleGenerationModelQuery.data}
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
				onSaveTitleGenerationModel={saveTitleGenerationModelMutation.mutate}
				isSavingTitleGenerationModel={
					saveTitleGenerationModelMutation.isPending
				}
				isSaveTitleGenerationModelError={
					saveTitleGenerationModelMutation.isError
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
