import { API, type ChatDiffStatusResponse } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import type { QueryClient } from "react-query";

export const chatsKey = ["chats"] as const;
export const chatKey = (chatId: string) => ["chats", chatId] as const;

export const chats = () => ({
	queryKey: chatsKey,
	queryFn: () => API.getChats(),
	refetchOnWindowFocus: true as const,
});

export const chat = (chatId: string) => ({
	queryKey: chatKey(chatId),
	queryFn: () => API.getChat(chatId),
});

export const archiveChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) => API.archiveChat(chatId),
	onMutate: async (chatId: string) => {
		await queryClient.cancelQueries({ queryKey: chatsKey });
		await queryClient.cancelQueries({ queryKey: chatKey(chatId) });
		const previousChats = queryClient.getQueryData<TypesGen.Chat[]>(chatsKey);
		const previousChat = queryClient.getQueryData<TypesGen.ChatWithMessages>(
			chatKey(chatId),
		);
		queryClient.setQueryData<TypesGen.Chat[]>(chatsKey, (old) =>
			old?.map((chat) =>
				chat.id === chatId ? { ...chat, archived: true } : chat,
			),
		);
		if (previousChat) {
			queryClient.setQueryData<TypesGen.ChatWithMessages>(chatKey(chatId), {
				...previousChat,
				chat: { ...previousChat.chat, archived: true },
			});
		}
		return { previousChats, previousChat };
	},
	onError: (
		_error: unknown,
		chatId: string,
		context:
			| {
					previousChats?: TypesGen.Chat[];
					previousChat?: TypesGen.ChatWithMessages;
			  }
			| undefined,
	) => {
		if (context?.previousChats) {
			queryClient.setQueryData<TypesGen.Chat[]>(
				chatsKey,
				context.previousChats,
			);
		}
		if (context?.previousChat) {
			queryClient.setQueryData<TypesGen.ChatWithMessages>(
				chatKey(chatId),
				context.previousChat,
			);
		}
	},
	onSettled: async (_data: unknown, _error: unknown, chatId: string) => {
		await queryClient.invalidateQueries({ queryKey: chatsKey });
		await queryClient.invalidateQueries({ queryKey: chatKey(chatId) });
	},
});

export const unarchiveChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) => API.unarchiveChat(chatId),
	onMutate: async (chatId: string) => {
		await queryClient.cancelQueries({ queryKey: chatsKey });
		await queryClient.cancelQueries({ queryKey: chatKey(chatId) });
		const previousChats = queryClient.getQueryData<TypesGen.Chat[]>(chatsKey);
		const previousChat = queryClient.getQueryData<TypesGen.ChatWithMessages>(
			chatKey(chatId),
		);
		queryClient.setQueryData<TypesGen.Chat[]>(chatsKey, (old) =>
			old?.map((chat) =>
				chat.id === chatId ? { ...chat, archived: false } : chat,
			),
		);
		if (previousChat) {
			queryClient.setQueryData<TypesGen.ChatWithMessages>(chatKey(chatId), {
				...previousChat,
				chat: { ...previousChat.chat, archived: false },
			});
		}
		return { previousChats, previousChat };
	},
	onError: (
		_error: unknown,
		chatId: string,
		context:
			| {
					previousChats?: TypesGen.Chat[];
					previousChat?: TypesGen.ChatWithMessages;
			  }
			| undefined,
	) => {
		if (context?.previousChats) {
			queryClient.setQueryData<TypesGen.Chat[]>(
				chatsKey,
				context.previousChats,
			);
		}
		if (context?.previousChat) {
			queryClient.setQueryData<TypesGen.ChatWithMessages>(
				chatKey(chatId),
				context.previousChat,
			);
		}
	},
	onSettled: async (_data: unknown, _error: unknown, chatId: string) => {
		await queryClient.invalidateQueries({ queryKey: chatsKey });
		await queryClient.invalidateQueries({ queryKey: chatKey(chatId) });
	},
});

export const createChat = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.CreateChatRequest) => API.createChat(req),
	onSuccess: () => {
		void queryClient.invalidateQueries({ queryKey: chatsKey });
	},
});

export const createChatMessage = (
	queryClient: QueryClient,
	chatId: string,
) => ({
	mutationFn: (req: TypesGen.CreateChatMessageRequest) =>
		API.createChatMessage(chatId, req),
	onSuccess: () => {
		void queryClient.invalidateQueries({ queryKey: chatsKey });
	},
});

type EditChatMessageMutationArgs = {
	messageId: number;
	req: TypesGen.EditChatMessageRequest;
};

export const editChatMessage = (queryClient: QueryClient, chatId: string) => ({
	mutationFn: ({ messageId, req }: EditChatMessageMutationArgs) =>
		API.editChatMessage(chatId, messageId, req),
	onSuccess: () => {
		void queryClient.invalidateQueries({ queryKey: chatsKey });
		void queryClient.invalidateQueries({ queryKey: chatKey(chatId) });
	},
});

export const interruptChat = (queryClient: QueryClient, chatId: string) => ({
	mutationFn: () => API.interruptChat(chatId),
	onSuccess: () => {
		void queryClient.invalidateQueries({ queryKey: chatsKey });
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
	onSuccess: () => {
		void queryClient.invalidateQueries({ queryKey: chatsKey });
		void queryClient.invalidateQueries({ queryKey: chatKey(chatId) });
	},
});

export const chatDiffStatusKey = (chatId: string) =>
	["chats", chatId, "diff-status"] as const;

export const chatDiffStatus = (chatId: string) => ({
	queryKey: chatDiffStatusKey(chatId),
	queryFn: (): Promise<ChatDiffStatusResponse> => API.getChatDiffStatus(chatId),
});

export const chatDiffContentsKey = (chatId: string) =>
	["chats", chatId, "diff-contents"] as const;

export const chatDiffContents = (chatId: string) => ({
	queryKey: chatDiffContentsKey(chatId),
	queryFn: () => API.getChatDiffContents(chatId),
});

export const chatModelsKey = ["chat-models"] as const;

export const chatModels = () => ({
	queryKey: chatModelsKey,
	queryFn: (): Promise<TypesGen.ChatModelsResponse> => API.getChatModels(),
});

export const chatProviderConfigsKey = ["chat-provider-configs"] as const;

export const chatProviderConfigs = () => ({
	queryKey: chatProviderConfigsKey,
	queryFn: (): Promise<TypesGen.ChatProviderConfig[]> =>
		API.getChatProviderConfigs(),
});

export const chatModelConfigsKey = ["chat-model-configs"] as const;

export const chatModelConfigs = () => ({
	queryKey: chatModelConfigsKey,
	queryFn: (): Promise<TypesGen.ChatModelConfig[]> => API.getChatModelConfigs(),
});

const invalidateChatConfigurationQueries = async (queryClient: QueryClient) => {
	await Promise.all([
		queryClient.invalidateQueries({ queryKey: chatProviderConfigsKey }),
		queryClient.invalidateQueries({ queryKey: chatModelConfigsKey }),
		queryClient.invalidateQueries({ queryKey: chatModelsKey }),
	]);
};

export const createChatProviderConfig = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.CreateChatProviderConfigRequest) =>
		API.createChatProviderConfig(req),
	onSuccess: async () => {
		await invalidateChatConfigurationQueries(queryClient);
	},
});

type UpdateChatProviderConfigMutationArgs = {
	providerConfigId: string;
	req: TypesGen.UpdateChatProviderConfigRequest;
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
	mutationFn: (req: TypesGen.CreateChatModelConfigRequest) =>
		API.createChatModelConfig(req),
	onSuccess: async () => {
		await invalidateChatConfigurationQueries(queryClient);
	},
});

type UpdateChatModelConfigMutationArgs = {
	modelConfigId: string;
	req: TypesGen.UpdateChatModelConfigRequest;
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
