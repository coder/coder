import type { FC } from "react";
import {
	type QueryClient,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { API } from "#/api/api";
import {
	chatModelConfigs,
	chatPersonalModelOverridesAdminSettings,
	updateChatPersonalModelOverridesAdminSettings,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AgentSettingsAgentsPageView } from "./AgentSettingsAgentsPageView";

const generalOverrideContext: TypesGen.ChatModelOverrideContext = "general";
const exploreOverrideContext: TypesGen.ChatModelOverrideContext = "explore";
const titleGenerationOverrideContext: TypesGen.ChatModelOverrideContext =
	"title_generation";

const chatModelOverrideKey = (context: TypesGen.ChatModelOverrideContext) =>
	["chat-model-override", context] as const;

const chatModelOverrideQuery = (
	context: TypesGen.ChatModelOverrideContext,
) => ({
	queryKey: chatModelOverrideKey(context),
	queryFn: () => API.experimental.getChatModelOverride(context),
});

const updateChatModelOverrideMutation = (
	queryClient: QueryClient,
	context: TypesGen.ChatModelOverrideContext,
) => ({
	mutationFn: (req: TypesGen.UpdateChatModelOverrideRequest) =>
		API.experimental.updateChatModelOverride(context, req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatModelOverrideKey(context),
			exact: true,
		});
	},
});

const AgentSettingsAgentsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const canEditDeploymentConfig = permissions.editDeploymentConfig;

	const personalModelOverridesAdminSettingsQuery = useQuery({
		...chatPersonalModelOverridesAdminSettings(),
		enabled: canEditDeploymentConfig,
	});
	const generalModelOverrideQuery = useQuery({
		...chatModelOverrideQuery(generalOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const exploreModelOverrideQuery = useQuery({
		...chatModelOverrideQuery(exploreOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const titleGenerationModelQuery = useQuery({
		...chatModelOverrideQuery(titleGenerationOverrideContext),
		enabled: canEditDeploymentConfig,
	});
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const savePersonalModelOverridesAdminSettingsMutation = useMutation(
		updateChatPersonalModelOverridesAdminSettings(queryClient),
	);
	const saveGeneralModelOverrideMutation = useMutation(
		updateChatModelOverrideMutation(queryClient, generalOverrideContext),
	);
	const saveTitleGenerationModelMutation = useMutation(
		updateChatModelOverrideMutation(
			queryClient,
			titleGenerationOverrideContext,
		),
	);
	const saveExploreModelOverrideMutation = useMutation(
		updateChatModelOverrideMutation(queryClient, exploreOverrideContext),
	);

	return (
		<RequirePermission isFeatureVisible={canEditDeploymentConfig}>
			<AgentSettingsAgentsPageView
				adminOverridesData={personalModelOverridesAdminSettingsQuery.data}
				adminOverridesError={personalModelOverridesAdminSettingsQuery.error}
				onRetryAdminOverrides={() => {
					void personalModelOverridesAdminSettingsQuery.refetch();
				}}
				isRetryingAdminOverrides={
					personalModelOverridesAdminSettingsQuery.isFetching
				}
				onSaveAdminOverrides={
					savePersonalModelOverridesAdminSettingsMutation.mutate
				}
				isSavingAdminOverrides={
					savePersonalModelOverridesAdminSettingsMutation.isPending
				}
				isSaveAdminOverridesError={
					savePersonalModelOverridesAdminSettingsMutation.isError
				}
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
