import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation, useNavigate } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	chatModelConfigs,
	chatModels,
	chatProviderConfigs,
	createChat,
	mcpServerConfigs,
	userChatPersonalModelOverrides,
	userChatProviderConfigs,
} from "#/api/queries/chats";
import { preferenceSettings } from "#/api/queries/users";
import { workspaces } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { useWebpushNotifications } from "#/contexts/useWebpushNotifications";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useAIGatewayEnabled } from "#/hooks/useEmbeddedMetadata";
import {
	AgentCreateForm,
	type CreateChatOptions,
} from "./components/AgentCreateForm";
import { AgentPageHeader } from "./components/AgentPageHeader";
import { ChimeButton } from "./components/ChimeButton";
import { WebPushButton } from "./components/WebPushButton";
import { getAgentChatSendShortcut } from "./utils/agentChatSendShortcut";
import { getChimeEnabled, setChimeEnabled } from "./utils/chime";
import {
	countConfiguredProviderConfigs,
	getUnsupportedProviderNames,
	resolveModelSelector,
} from "./utils/modelOptions";
import { buildAgentChatPath } from "./utils/navigation";

const lastModelConfigIDStorageKey = "agents.last-model-config-id";

const AgentCreatePage: FC = () => {
	const queryClient = useQueryClient();
	const location = useLocation();
	const navigate = useNavigate();
	const { permissions } = useAuthenticated();
	const aiGatewayDisabled = !useAIGatewayEnabled();

	const chatModelsQuery = useQuery(chatModels());
	const chatModelConfigsQuery = useQuery(chatModelConfigs());
	const chatProviderConfigsQuery = useQuery({
		...chatProviderConfigs(),
		enabled: permissions.editDeploymentConfig,
	});
	const userProviderConfigsQuery = useQuery(userChatProviderConfigs());
	const personalModelOverridesQuery = useQuery(
		userChatPersonalModelOverrides(),
	);
	const preferencesQuery = useQuery(preferenceSettings());
	const mcpServersQuery = useQuery(mcpServerConfigs());
	const workspacesQuery = useQuery(workspaces({ q: "owner:me", limit: 0 }));
	const createMutation = useMutation(createChat(queryClient));
	const webPush = useWebpushNotifications();
	const [chimeEnabled, setChimeEnabledState] = useState(getChimeEnabled);

	const { options: catalogModelOptions, isModelCatalogLoading } =
		resolveModelSelector(
			chatModelConfigsQuery,
			chatModelsQuery,
			userProviderConfigsQuery,
		);
	const providerCount =
		permissions.editDeploymentConfig &&
		chatProviderConfigsQuery.isSuccess &&
		chatModelsQuery.isSuccess
			? countConfiguredProviderConfigs(
					chatProviderConfigsQuery.data,
					chatModelsQuery.data,
				)
			: undefined;
	const modelCount =
		chatModelConfigsQuery.isSuccess && chatModelsQuery.isSuccess
			? catalogModelOptions.length
			: undefined;
	const unsupportedProviderNames = getUnsupportedProviderNames(
		chatModelsQuery.data,
	);

	const handleCreateChat = async ({
		message,
		fileIDs,
		workspaceId,
		model,
		reasoningEffort,
		mcpServerIds,
		organizationId,
		planMode,
	}: CreateChatOptions) => {
		const content: TypesGen.ChatInputPart[] = [];
		if (message.trim()) {
			content.push({ type: "text", text: message });
		}
		if (fileIDs) {
			for (const fileID of fileIDs) {
				content.push({ type: "file", file_id: fileID });
			}
		}
		const createRequest: TypesGen.CreateChatRequest = {
			organization_id: organizationId,
			content,
			workspace_id: workspaceId,
			mcp_server_ids:
				mcpServerIds && mcpServerIds.length > 0 ? mcpServerIds : undefined,
			plan_mode: planMode === "plan" ? "plan" : undefined,
			client_type: "ui",
			...(model ? { model_config_id: model } : {}),
			...(reasoningEffort ? { reasoning_effort: reasoningEffort } : {}),
		};
		const createdChat = await createMutation.mutateAsync(createRequest);

		if (model) {
			localStorage.setItem(lastModelConfigIDStorageKey, model);
		}
		navigate({
			pathname: buildAgentChatPath({ chatId: createdChat.id }),
			search: location.search,
		});
	};

	const rootPersonalModelOverride = personalModelOverridesQuery.data?.enabled
		? personalModelOverridesQuery.data.root
		: undefined;

	const handleChimeToggle = () => {
		const next = !chimeEnabled;
		setChimeEnabledState(next);
		setChimeEnabled(next);
	};

	const handleNotificationToggle = async () => {
		try {
			if (webPush.subscribed) {
				await webPush.unsubscribe();
			} else {
				await webPush.subscribe();
			}
		} catch (error) {
			const action = webPush.subscribed ? "disable" : "enable";
			toast.error(getErrorMessage(error, `Failed to ${action} notifications.`));
		}
	};

	return (
		<>
			<AgentPageHeader
				chimeEnabled={chimeEnabled}
				onToggleChime={handleChimeToggle}
				webPush={webPush}
				onToggleNotifications={handleNotificationToggle}
			>
				<ChimeButton enabled={chimeEnabled} onToggle={handleChimeToggle} />
				<WebPushButton webPush={webPush} onToggle={handleNotificationToggle} />
			</AgentPageHeader>
			<AgentCreateForm
				onCreateChat={handleCreateChat}
				sendShortcut={getAgentChatSendShortcut(
					preferencesQuery.data?.agent_chat_send_shortcut,
					preferencesQuery.isLoading,
				)}
				isCreating={createMutation.isPending}
				createError={createMutation.error}
				canCreateChat={permissions.createChat}
				modelCatalog={chatModelsQuery.data}
				modelOptions={catalogModelOptions}
				canConfigureAgentSetup={permissions.editDeploymentConfig}
				providerCount={providerCount}
				modelCount={modelCount}
				unsupportedProviderNames={unsupportedProviderNames}
				aiGatewayDisabled={aiGatewayDisabled}
				modelConfigs={chatModelConfigsQuery.data ?? []}
				isModelCatalogLoading={isModelCatalogLoading}
				isModelConfigsLoading={chatModelConfigsQuery.isLoading}
				rootPersonalModelOverride={rootPersonalModelOverride}
				isPersonalModelOverridesLoading={personalModelOverridesQuery.isLoading}
				mcpServers={mcpServersQuery.data ?? []}
				onMCPAuthComplete={() => void mcpServersQuery.refetch()}
				workspaceCount={workspacesQuery.data?.count}
				workspaceOptions={workspacesQuery.data?.workspaces ?? []}
				workspacesError={workspacesQuery.error}
				isWorkspacesLoading={workspacesQuery.isLoading}
			/>{" "}
		</>
	);
};

export default AgentCreatePage;
