import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import {
	chatModelConfigs,
	chatModels,
	createChat,
	mcpServerConfigs,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import {
	AgentCreateForm,
	type CreateChatOptions,
} from "./components/AgentCreateForm";
import { AgentPageHeader } from "./components/AgentPageHeader";
import { ChimeButton } from "./components/ChimeButton";
import { WebPushButton } from "./components/WebPushButton";
import { getModelOptionsFromConfigs } from "./utils/modelOptions";

const lastModelConfigIDStorageKey = "agents.last-model-config-id";
const nilUUID = "00000000-0000-0000-0000-000000000000";

const AgentCreatePage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();

	const chatModelsQuery = useQuery(chatModels());
	const chatModelConfigsQuery = useQuery(chatModelConfigs());
	const mcpServersQuery = useQuery(mcpServerConfigs());
	const createMutation = useMutation(createChat(queryClient));

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
	}: CreateChatOptions) => {
		const modelConfigID = model || nilUUID;
		const content: TypesGen.ChatInputPart[] = [];
		if (message.trim()) {
			content.push({ type: "text", text: message });
		}
		if (fileIDs) {
			for (const fileID of fileIDs) {
				content.push({ type: "file", file_id: fileID });
			}
		}
		const createdChat = await createMutation.mutateAsync({
			content,
			workspace_id: workspaceId,
			model_config_id: modelConfigID,
			mcp_server_ids:
				mcpServerIds && mcpServerIds.length > 0 ? mcpServerIds : undefined,
		});

		if (modelConfigID !== nilUUID) {
			localStorage.setItem(lastModelConfigIDStorageKey, modelConfigID);
		} else {
			localStorage.removeItem(lastModelConfigIDStorageKey);
		}
		navigate(`/agents/${createdChat.id}`);
	};

	return (
		<>
			<AgentPageHeader>
				<ChimeButton />
				<WebPushButton />
			</AgentPageHeader>
			<AgentCreateForm
				onCreateChat={handleCreateChat}
				isCreating={createMutation.isPending}
				createError={createMutation.error}
				modelCatalog={chatModelsQuery.data}
				modelOptions={catalogModelOptions}
				modelConfigs={chatModelConfigsQuery.data ?? []}
				isModelCatalogLoading={chatModelsQuery.isLoading}
				isModelConfigsLoading={chatModelConfigsQuery.isLoading}
				mcpServers={mcpServersQuery.data ?? []}
				onMCPAuthComplete={() => void mcpServersQuery.refetch()}
			/>
		</>
	);
};

export default AgentCreatePage;
