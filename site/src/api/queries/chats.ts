import {
	API,
	type ChatDiffStatusResponse,
	type ChatGitChangeResponse,
	type ChatModelConfig,
	type ChatModelsResponse,
	type ChatProviderConfig,
	type CreateChatModelConfigRequest,
	type CreateChatProviderConfigRequest,
	type UpdateChatModelConfigRequest,
	type UpdateChatProviderConfigRequest,
} from "api/api";
import type * as TypesGen from "api/typesGenerated";
import type { QueryClient } from "react-query";

export const chatsKey = ["chats"] as const;
export const chatKey = (chatId: string) => ["chat", chatId] as const;

export const chats = () => ({
	queryKey: chatsKey,
	queryFn: () => API.getChats(),
});

export const chat = (chatId: string) => ({
	queryKey: chatKey(chatId),
	queryFn: () => API.getChat(chatId),
});

export const createChat = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.CreateChatRequest) => API.createChat(req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: chatsKey });
	},
});

export const deleteChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) => API.deleteChat(chatId),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: chatsKey });
	},
});

export const createChatMessage = (
	queryClient: QueryClient,
	chatId: string,
) => ({
	mutationFn: (req: TypesGen.CreateChatMessageRequest) =>
		API.createChatMessage(chatId, req),
	onSuccess: async () => {
		await Promise.all([
			queryClient.invalidateQueries({ queryKey: chatKey(chatId) }),
			queryClient.invalidateQueries({ queryKey: chatsKey }),
		]);
	},
});

export const interruptChat = (queryClient: QueryClient, chatId: string) => ({
	mutationFn: () => API.interruptChat(chatId),
	onSuccess: async () => {
		await Promise.all([
			queryClient.invalidateQueries({ queryKey: chatKey(chatId) }),
			queryClient.invalidateQueries({ queryKey: chatsKey }),
		]);
	},
});

export const deleteChatQueuedMessage = (
	queryClient: QueryClient,
	chatId: string,
) => ({
	mutationFn: (queuedMessageId: number) =>
		API.deleteChatQueuedMessage(chatId, queuedMessageId),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: chatKey(chatId) });
	},
});

export const promoteChatQueuedMessage = (
	queryClient: QueryClient,
	chatId: string,
) => ({
	mutationFn: (queuedMessageId: number) =>
		API.promoteChatQueuedMessage(chatId, queuedMessageId),
	onSuccess: async () => {
		await Promise.all([
			queryClient.invalidateQueries({ queryKey: chatKey(chatId) }),
			queryClient.invalidateQueries({ queryKey: chatsKey }),
		]);
	},
});

export const chatGitChangesKey = (chatId: string) =>
	["chat", chatId, "git-changes"] as const;

export const chatGitChanges = (chatId: string) => ({
	queryKey: chatGitChangesKey(chatId),
	queryFn: (): Promise<ChatGitChangeResponse[]> =>
		API.getChatGitChanges(chatId),
});

export const chatDiffStatusKey = (chatId: string) =>
	["chat", chatId, "diff-status"] as const;

export const chatDiffStatus = (chatId: string) => ({
	queryKey: chatDiffStatusKey(chatId),
	queryFn: (): Promise<ChatDiffStatusResponse> => API.getChatDiffStatus(chatId),
});

export const chatDiffContentsKey = (chatId: string) =>
	["chat", chatId, "diff-contents"] as const;

export const chatDiffContents = (chatId: string) => ({
	queryKey: chatDiffContentsKey(chatId),
	queryFn: () => API.getChatDiffContents(chatId),
});

export const chatModelsKey = ["chat-models"] as const;

export const chatModels = () => ({
	queryKey: chatModelsKey,
	queryFn: (): Promise<ChatModelsResponse | null> => API.getChatModels(),
});

export const chatProviderConfigsKey = ["chat-provider-configs"] as const;

export const chatProviderConfigs = () => ({
	queryKey: chatProviderConfigsKey,
	queryFn: (): Promise<ChatProviderConfig[] | null> =>
		API.getChatProviderConfigs(),
});

export const chatModelConfigsKey = ["chat-model-configs"] as const;

export const chatModelConfigs = () => ({
	queryKey: chatModelConfigsKey,
	queryFn: (): Promise<ChatModelConfig[] | null> => API.getChatModelConfigs(),
});

const invalidateChatConfigurationQueries = async (queryClient: QueryClient) => {
	await Promise.all([
		queryClient.invalidateQueries({ queryKey: chatProviderConfigsKey }),
		queryClient.invalidateQueries({ queryKey: chatModelConfigsKey }),
		queryClient.invalidateQueries({ queryKey: chatModelsKey }),
	]);
};

export const createChatProviderConfig = (queryClient: QueryClient) => ({
	mutationFn: (req: CreateChatProviderConfigRequest) =>
		API.createChatProviderConfig(req),
	onSuccess: async () => {
		await invalidateChatConfigurationQueries(queryClient);
	},
});

export type UpdateChatProviderConfigMutationArgs = {
	providerConfigId: string;
	req: UpdateChatProviderConfigRequest;
};

export const updateChatProviderConfig = (queryClient: QueryClient) => ({
	mutationFn: ({
		providerConfigId,
		req,
	}: UpdateChatProviderConfigMutationArgs) =>
		API.updateChatProviderConfig(providerConfigId, req),
	onSuccess: async () => {
		await invalidateChatConfigurationQueries(queryClient);
	},
});

export const deleteChatProviderConfig = (queryClient: QueryClient) => ({
	mutationFn: (providerConfigId: string) =>
		API.deleteChatProviderConfig(providerConfigId),
	onSuccess: async () => {
		await invalidateChatConfigurationQueries(queryClient);
	},
});

export const createChatModelConfig = (queryClient: QueryClient) => ({
	mutationFn: (req: CreateChatModelConfigRequest) =>
		API.createChatModelConfig(req),
	onSuccess: async () => {
		await invalidateChatConfigurationQueries(queryClient);
	},
});

export type UpdateChatModelConfigMutationArgs = {
	modelConfigId: string;
	req: UpdateChatModelConfigRequest;
};

export const updateChatModelConfig = (queryClient: QueryClient) => ({
	mutationFn: ({ modelConfigId, req }: UpdateChatModelConfigMutationArgs) =>
		API.updateChatModelConfig(modelConfigId, req),
	onSuccess: async () => {
		await invalidateChatConfigurationQueries(queryClient);
	},
});

export const deleteChatModelConfig = (queryClient: QueryClient) => ({
	mutationFn: (modelConfigId: string) =>
		API.deleteChatModelConfig(modelConfigId),
	onSuccess: async () => {
		await invalidateChatConfigurationQueries(queryClient);
	},
});
