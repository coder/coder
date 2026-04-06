import { isAxiosError } from "axios";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatModelConfigs,
	chatModels,
	chatProviderConfigs,
	createChatModelConfig,
	deleteChatModelConfig,
	updateChatModelConfig,
} from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AdminBadge } from "./components/AdminBadge";
import { ChatModelAdminPanel } from "./components/ChatModelAdminPanel/ChatModelAdminPanel";

const isEndpointUnavailable = (error: unknown): boolean => {
	return isAxiosError(error) && error.response?.status === 404;
};

const AgentSettingsModelsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const providerConfigsQuery = useQuery(chatProviderConfigs());
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const modelCatalogQuery = useQuery(chatModels());

	const createModelMutation = useMutation(createChatModelConfig(queryClient));
	const updateModelMutation = useMutation(updateChatModelConfig(queryClient));
	const deleteModelMutation = useMutation(deleteChatModelConfig(queryClient));

	const isLoading =
		providerConfigsQuery.isLoading ||
		modelConfigsQuery.isLoading ||
		modelCatalogQuery.isLoading;

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<ChatModelAdminPanel
				section="models"
				sectionLabel="Models"
				sectionDescription="Choose which models from your configured providers are available for users to select. You can set a default and adjust context limits."
				sectionBadge={<AdminBadge />}
				providerConfigsData={providerConfigsQuery.data}
				modelConfigsData={modelConfigsQuery.data}
				catalogData={modelCatalogQuery.data}
				isLoading={isLoading}
				providerConfigsUnavailable={isEndpointUnavailable(
					providerConfigsQuery.error,
				)}
				modelConfigsUnavailable={isEndpointUnavailable(modelConfigsQuery.error)}
				providerConfigsError={providerConfigsQuery.error}
				modelConfigsError={modelConfigsQuery.error}
				catalogError={modelCatalogQuery.error}
				isCreatingModel={createModelMutation.isPending}
				isUpdatingModel={updateModelMutation.isPending}
				isDeletingModel={deleteModelMutation.isPending}
				modelMutationError={
					createModelMutation.error ??
					updateModelMutation.error ??
					deleteModelMutation.error
				}
				onCreateModel={(req) => createModelMutation.mutateAsync(req)}
				onUpdateModel={(modelConfigId, req) =>
					updateModelMutation.mutateAsync({ modelConfigId, req })
				}
				onDeleteModel={(id) => deleteModelMutation.mutateAsync(id)}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsModelsPage;
