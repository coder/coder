import { API } from "api/api";
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

export const createChatMessage = (queryClient: QueryClient, chatId: string) => ({
	mutationFn: (req: TypesGen.CreateChatMessageRequest) =>
		API.createChatMessage(chatId, req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: chatKey(chatId) });
	},
});

export const chatGitChangesKey = (chatId: string) =>
	["chat", chatId, "git-changes"] as const;

export const chatGitChanges = (chatId: string) => ({
	queryKey: chatGitChangesKey(chatId),
	queryFn: () => API.getChatGitChanges(chatId),
});
