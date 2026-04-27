import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatModelConfigs,
	chatModels,
	chatProviderConfigs,
	createChatModelConfig,
	createChatProviderConfig,
	deleteChatModelConfig,
	deleteChatProviderConfig,
	updateChatModelConfig,
	updateChatProviderConfig,
} from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { ChatModelAdminPanel } from "./components/ChatModelAdminPanel/ChatModelAdminPanel";

const AgentSettingsProvidersPage: FC = () => {
	const { permissions } = useAuthenticated();

	const queryClient = useQueryClient();

	// Queries.
	const providerConfigsQuery = useQuery(chatProviderConfigs());
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const modelCatalogQuery = useQuery(chatModels());

	// Mutations.
	const createProviderMutation = useMutation(
		createChatProviderConfig(queryClient),
	);
	const updateProviderMutation = useMutation(
		updateChatProviderConfig(queryClient),
	);
	const deleteProviderMutation = useMutation(
		deleteChatProviderConfig(queryClient),
	);
	const createModelMutation = useMutation(createChatModelConfig(queryClient));
	const updateModelMutation = useMutation(updateChatModelConfig(queryClient));
	const deleteModelMutation = useMutation(deleteChatModelConfig(queryClient));

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<ChatModelAdminPanel
				section="providers"
				sectionLabel="Providers"
				sectionDescription="Connect third-party LLM services like OpenAI, Anthropic, or Google. Each provider supplies models that users can select for their conversations."
				providerConfigsData={providerConfigsQuery.data}
				modelConfigsData={modelConfigsQuery.data}
				modelCatalogData={modelCatalogQuery.data}
				isLoading={
					providerConfigsQuery.isLoading ||
					modelConfigsQuery.isLoading ||
					modelCatalogQuery.isLoading
				}
				providerConfigsError={
					providerConfigsQuery.isError ? providerConfigsQuery.error : null
				}
				modelConfigsError={
					modelConfigsQuery.isError ? modelConfigsQuery.error : null
				}
				modelCatalogError={
					modelCatalogQuery.isError ? modelCatalogQuery.error : null
				}
				onCreateProvider={(req) => createProviderMutation.mutateAsync(req)}
				onUpdateProvider={(providerConfigId, req) =>
					updateProviderMutation.mutateAsync({ providerConfigId, req })
				}
				onDeleteProvider={(id) => deleteProviderMutation.mutateAsync(id)}
				isProviderMutationPending={
					createProviderMutation.isPending ||
					updateProviderMutation.isPending ||
					deleteProviderMutation.isPending
				}
				providerMutationError={
					createProviderMutation.error ??
					updateProviderMutation.error ??
					deleteProviderMutation.error
				}
				onCreateModel={(req) => createModelMutation.mutateAsync(req)}
				onUpdateModel={(modelConfigId, req) =>
					updateModelMutation.mutateAsync({ modelConfigId, req })
				}
				onDeleteModel={(id) => deleteModelMutation.mutateAsync(id)}
				isCreatingModel={createModelMutation.isPending}
				isUpdatingModel={updateModelMutation.isPending}
				isDeletingModel={deleteModelMutation.isPending}
				modelMutationError={
					createModelMutation.error ??
					updateModelMutation.error ??
					deleteModelMutation.error
				}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsProvidersPage;
