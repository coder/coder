import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import {
	invalidateChatEntity,
	invalidateChatListQueries,
	patchChatEntity,
	updateInfiniteChatsCache,
} from "#/api/queries/chats";

type UpdateChatLabelsVariables = {
	chatId: string;
	labels: Record<string, string>;
};

// Labels replace the whole map server-side, so callers pass the complete
// desired map. The list cache is patched before the request so board drags
// settle instantly; the settled invalidation corrects any divergence.
export const updateChatLabels = (queryClient: QueryClient) => ({
	mutationFn: ({ chatId, labels }: UpdateChatLabelsVariables) =>
		API.experimental.updateChat(chatId, { labels }),

	onMutate: ({ chatId, labels }: UpdateChatLabelsVariables) => {
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) => (chat.id === chatId ? { ...chat, labels } : chat)),
		);
		patchChatEntity(queryClient, chatId, (chat) =>
			chat ? { ...chat, labels } : chat,
		);
	},

	onSettled: (
		_data: unknown,
		_error: unknown,
		{ chatId }: UpdateChatLabelsVariables,
	) => {
		void invalidateChatListQueries(queryClient);
		void invalidateChatEntity(queryClient, chatId);
	},
});
