import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	chatModelConfigs,
	chatModels,
	createChat,
	mcpServerConfigs,
	userChatPersonalModelOverrides,
} from "#/api/queries/chats";
import { workspaces } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { useWebpushNotifications } from "#/contexts/useWebpushNotifications";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	AgentCreateForm,
	type CreateChatOptions,
} from "./components/AgentCreateForm";
import { AgentPageHeader } from "./components/AgentPageHeader";
import { ChimeButton } from "./components/ChimeButton";
import { WebPushButton } from "./components/WebPushButton";
import { getChimeEnabled, setChimeEnabled } from "./utils/chime";
import { getModelOptionsFromConfigs } from "./utils/modelOptions";
import { buildAgentChatPath } from "./utils/navigation";

const lastModelConfigIDStorageKey = "agents.last-model-config-id";

const AgentCreatePage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const { permissions } = useAuthenticated();

	const chatModelsQuery = useQuery(chatModels());
	const chatModelConfigsQuery = useQuery(chatModelConfigs());
	const personalModelOverridesQuery = useQuery(
		userChatPersonalModelOverrides(),
	);
	const mcpServersQuery = useQuery(mcpServerConfigs());
	const workspacesQuery = useQuery(workspaces({ q: "owner:me", limit: 0 }));
	const createMutation = useMutation(createChat(queryClient));
	const webPush = useWebpushNotifications();
	const [chimeEnabled, setChimeEnabledState] = useState(getChimeEnabled);

	const catalogModelOptions = getModelOptionsFromConfigs(
		chatModelConfigsQuery.data,
		chatModelsQuery.data,
	);

	const handleCreateChat = async ({
		message,
		fileIDs,
		workspaceId,
		model,
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
		};
		const createdChat = await createMutation.mutateAsync(createRequest);

		if (model) {
			localStorage.setItem(lastModelConfigIDStorageKey, model);
		}
		navigate(buildAgentChatPath({ chatId: createdChat.id }));
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
				isCreating={createMutation.isPending}
				createError={createMutation.error}
				canCreateChat={permissions.createChat}
				modelCatalog={chatModelsQuery.data}
				modelOptions={catalogModelOptions}
				modelConfigs={chatModelConfigsQuery.data ?? []}
				isModelCatalogLoading={chatModelsQuery.isLoading}
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
