import { chatModelConfigs, chatModels, createChat } from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { AgentCreateForm, type CreateChatOptions } from "./AgentCreateForm";
import { AgentPageHeader } from "./AgentPageHeader";
import { ChimeButton } from "./ChimeButton";
import {
	buildModelConfigIDByModelID,
	getModelOptionsFromCatalog,
} from "./modelOptions";
import { WebPushButton } from "./WebPushButton";

const lastModelConfigIDStorageKey = "agents.last-model-config-id";
const nilUUID = "00000000-0000-0000-0000-000000000000";
const EMPTY_MODEL_CONFIGS: TypesGen.ChatModelConfig[] = [];

const AgentCreatePage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();

	const chatModelsQuery = useQuery(chatModels());
	const chatModelConfigsQuery = useQuery(chatModelConfigs());
	const createMutation = useMutation(createChat(queryClient));

	const catalogModelOptions = getModelOptionsFromCatalog(
		chatModelsQuery.data,
		chatModelConfigsQuery.data,
	);
	const modelConfigIDByModelID = buildModelConfigIDByModelID(
		chatModelConfigsQuery.data,
	);

	const handleCreateChat = async (options: CreateChatOptions) => {
		const { message, fileIDs, workspaceId, model } = options;
		const modelConfigID =
			(model && modelConfigIDByModelID.get(model)) || nilUUID;
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
		});

		if (typeof window !== "undefined") {
			if (modelConfigID !== nilUUID) {
				localStorage.setItem(lastModelConfigIDStorageKey, modelConfigID);
			} else {
				localStorage.removeItem(lastModelConfigIDStorageKey);
			}
		}

		navigate(`/agents/${createdChat.id}`);
	};

	const handleOpenAnalytics = () => navigate("/agents/analytics");

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
				modelConfigs={chatModelConfigsQuery.data ?? EMPTY_MODEL_CONFIGS}
				isModelCatalogLoading={chatModelsQuery.isLoading}
				isModelConfigsLoading={chatModelConfigsQuery.isLoading}
				modelCatalogError={chatModelsQuery.error}
				onOpenAnalytics={handleOpenAnalytics}
			/>
		</>
	);
};

export default AgentCreatePage;
