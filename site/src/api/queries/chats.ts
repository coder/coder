import { API } from "api/api";
import type { Chat, CreateChatRequest } from "api/typesGenerated";
import type { MutationOptions, QueryClient, QueryOptions } from "react-query";

export const chatsKey = ["chats"];

export const chats = () => {
	return {
		queryKey: chatsKey,
		queryFn: () => API.getChats(),
	} satisfies QueryOptions<Chat[]>;
};

export const chatKey = (chatId: string) => ["chat", chatId];

export const chat = (chatId: string) => {
	return {
		queryKey: chatKey(chatId),
		queryFn: () => API.getChat(chatId),
	} satisfies QueryOptions<Chat>;
};

export const chatMessagesKey = (chatId: string) => ["chat", chatId, "messages"];

export const chatMessages = (chatId: string) => {
	return {
		queryKey: chatMessagesKey(chatId),
		queryFn: () => API.getChatMessages(chatId),
	};
};

export const createChat = (queryClient: QueryClient) => {
	return {
		mutationFn: (req: CreateChatRequest) => API.createChat(req),
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: chatsKey });
		},
	} satisfies MutationOptions<Chat, unknown, CreateChatRequest>;
};
