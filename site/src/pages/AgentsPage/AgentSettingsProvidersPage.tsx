import { isAxiosError } from "axios";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatModelConfigs,
	chatModels,
	chatProviderConfigs,
	createChatProviderConfig,
	deleteChatProviderConfig,
	updateChatProviderConfig,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AdminBadge } from "./components/AdminBadge";
import { ChatModelAdminPanel } from "./components/ChatModelAdminPanel/ChatModelAdminPanel";

const isEndpointUnavailable = (error: unknown): boolean => {
	return isAxiosError(error) && error.response?.status === 404;
};

const AgentSettingsProvidersPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const providerConfigsQuery = useQuery(chatProviderConfigs());
	const modelConfigsQuery = useQuery(chatModelConfigs());
	const modelCatalogQuery = useQuery(chatModels());

	const createProviderMutation = useMutation(
		createChatProviderConfig(queryClient),
	);
	const updateProviderMutation = useMutation(
		updateChatProviderConfig(queryClient),
	);
	const deleteProviderMutation = useMutation(
		deleteChatProviderConfig(queryClient),
	);

	const isLoading =
		providerConfigsQuery.isLoading ||
		modelConfigsQuery.isLoading ||
		modelCatalogQuery.isLoading;

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<ChatModelAdminPanel
				section="providers"
				sectionLabel="Providers"
				sectionDescription="Connect third-party LLM services like OpenAI, Anthropic, or Google. Each provider supplies models that users can select for their conversations."
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
				onCreateProvider={(req): Promise<TypesGen.ChatProviderConfig> =>
					createProviderMutation.mutateAsync(req)
				}
				onUpdateProvider={(providerConfigId, req) =>
					updateProviderMutation.mutateAsync({ providerConfigId, req })
				}
				onDeleteProvider={(id) => deleteProviderMutation.mutateAsync(id)}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsProvidersPage;
