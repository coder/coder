import type { QueryClient } from "react-query";
import { API } from "#/api/api";

const chatDebugLoggingKey = ["chat-debug-logging"] as const;
const userChatDebugLoggingKey = ["user-chat-debug-logging"] as const;

export const chatDebugLogging = () => ({
	queryKey: chatDebugLoggingKey,
	queryFn: () => API.experimental.getChatDebugLogging(),
});

export const userChatDebugLogging = () => ({
	queryKey: userChatDebugLoggingKey,
	queryFn: () => API.experimental.getUserChatDebugLogging(),
});

export const updateChatDebugLogging = (queryClient: QueryClient) => ({
	mutationFn: API.experimental.updateChatDebugLogging,
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatDebugLoggingKey,
		});
		await queryClient.invalidateQueries({
			queryKey: userChatDebugLoggingKey,
		});
	},
});

export const updateUserChatDebugLogging = (queryClient: QueryClient) => ({
	mutationFn: API.experimental.updateUserChatDebugLogging,
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: userChatDebugLoggingKey,
		});
	},
});
