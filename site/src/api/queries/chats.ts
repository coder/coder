import { API } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import type { QueryClient, UseInfiniteQueryOptions } from "react-query";

export const chatsKey = ["chats"] as const;
export const chatKey = (chatId: string) => ["chats", chatId] as const;
export const chatMessagesKey = (chatId: string) =>
	["chats", chatId, "messages"] as const;

/**
 * Updates a single chat inside every page of the infinite chats query
 * cache. Use this instead of setQueryData(chatsKey, ...) which writes
 * to the wrong key (the flat list key, not the infinite query key).
 */
export const updateInfiniteChatsCache = (
	queryClient: QueryClient,
	updater: (chats: TypesGen.Chat[]) => TypesGen.Chat[],
) => {
	// Update ALL infinite chat queries regardless of their filter opts.
	queryClient.setQueriesData<{
		pages: TypesGen.Chat[][];
		pageParams: unknown[];
	}>({ queryKey: chatsKey, predicate: isChatListQuery }, (prev) => {
		if (!prev) return prev;
		if (!prev.pages) return prev;
		const nextPages = prev.pages.map((page) => updater(page));
		// Only return a new reference if something actually changed.
		const changed = nextPages.some((page, i) => page !== prev.pages[i]);
		return changed ? { ...prev, pages: nextPages } : prev;
	});
};

/**
 * Prepends a new chat to the first page of every infinite chats query
 * in the cache, but only if the chat doesn't already exist in any
 * page. This avoids the per-page duplication that would occur if
 * a prepend updater were passed to updateInfiniteChatsCache, which
 * runs independently on each page.
 */
export const prependToInfiniteChatsCache = (
	queryClient: QueryClient,
	chat: TypesGen.Chat,
) => {
	queryClient.setQueriesData<{
		pages: TypesGen.Chat[][];
		pageParams: unknown[];
	}>({ queryKey: chatsKey, predicate: isChatListQuery }, (prev) => {
		if (!prev?.pages) return prev;
		// Check across ALL pages to avoid duplicates.
		const exists = prev.pages.some((page) =>
			page.some((c) => c.id === chat.id),
		);
		if (exists) return prev;
		// Only prepend to the first page.
		const nextPages = prev.pages.map((page, i) =>
			i === 0 ? [chat, ...page] : page,
		);
		return { ...prev, pages: nextPages };
	});
};

/**
 * Reads the flat list of chats from the first matching infinite query
 * in the cache. Returns undefined when no data is cached yet.
 */
export const readInfiniteChatsCache = (
	queryClient: QueryClient,
): TypesGen.Chat[] | undefined => {
	const queries = queryClient.getQueriesData<{
		pages: TypesGen.Chat[][];
		pageParams: unknown[];
	}>({ queryKey: chatsKey, predicate: isChatListQuery });
	for (const [, data] of queries) {
		if (data?.pages) {
			return data.pages.flat();
		}
	}
	return undefined;
};

/**
 * Invalidate only the sidebar chat-list queries (flat + infinite)
/**
 * Predicate that matches only chat-list queries (the sidebar), not
 * per-chat queries (detail, messages, diffs, cost).
 *
 * Sidebar keys look like ["chats"] or ["chats", <object|undefined>].
 * Per-chat keys look like ["chats", <string-id>, ...].
 */
const isChatListQuery = (query: { queryKey: readonly unknown[] }): boolean => {
	const key = query.queryKey;
	// Match: ["chats"] (flat list).
	if (key.length <= 1) return true;
	// Match: ["chats", <object | undefined>] (infinite query
	// with optional filter opts like {archived, q}).
	const segment = key[1];
	return segment === undefined || typeof segment === "object";
};

export const invalidateChatListQueries = (queryClient: QueryClient) => {
	return queryClient.invalidateQueries({
		queryKey: chatsKey,
		predicate: isChatListQuery,
	});
};

const DEFAULT_CHAT_PAGE_LIMIT = 50;

export const infiniteChats = (opts?: { q?: string; archived?: boolean }) => {
	const limit = DEFAULT_CHAT_PAGE_LIMIT;

	// Build the search query string including the archived filter.
	const qParts: string[] = [];
	if (opts?.q) {
		qParts.push(opts.q);
	}
	if (opts?.archived !== undefined) {
		qParts.push(`archived:${opts.archived}`);
	}
	const q = qParts.length > 0 ? qParts.join(" ") : undefined;

	return {
		queryKey: [...chatsKey, opts],
		getNextPageParam: (lastPage: TypesGen.Chat[], pages: TypesGen.Chat[][]) => {
			if (lastPage.length < limit) {
				return undefined;
			}
			return pages.length + 1;
		},
		initialPageParam: 0,
		queryFn: ({ pageParam }: { pageParam: unknown }) => {
			if (typeof pageParam !== "number") {
				throw new Error("pageParam must be a number");
			}
			return API.getChats({
				limit,
				offset: pageParam <= 0 ? 0 : (pageParam - 1) * limit,
				q,
			});
		},
		refetchOnWindowFocus: true as const,
	} satisfies UseInfiniteQueryOptions<TypesGen.Chat[]>;
};

export const chats = () => ({
	queryKey: chatsKey,
	queryFn: () => API.getChats(),
	refetchOnWindowFocus: true as const,
});

export const chat = (chatId: string) => ({
	queryKey: chatKey(chatId),
	queryFn: () => API.getChat(chatId),
});

const MESSAGES_PAGE_SIZE = 50;

export const chatMessagesForInfiniteScroll = (chatId: string) => ({
	queryKey: chatMessagesKey(chatId),
	initialPageParam: undefined as number | undefined,
	queryFn: ({ pageParam }: { pageParam: number | undefined }) =>
		API.getChatMessages(chatId, {
			before_id: pageParam,
			limit: MESSAGES_PAGE_SIZE,
		}),
	getNextPageParam: (lastPage: TypesGen.ChatMessagesResponse) => {
		if (!lastPage.has_more || lastPage.messages.length === 0) {
			return undefined;
		}
		// The API returns messages in DESC order (newest first).
		// The last item in the array is the oldest in this page.
		// Use its ID as the cursor for the next (older) page.
		return lastPage.messages[lastPage.messages.length - 1].id;
	},
});

export const archiveChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) => API.archiveChat(chatId),
	onMutate: async (chatId: string) => {
		await queryClient.cancelQueries({
			queryKey: chatsKey,
			predicate: (query) => {
				const key = query.queryKey;
				if (key.length <= 1) return true;
				const segment = key[1];
				return segment === undefined || typeof segment === "object";
			},
		});
		await queryClient.cancelQueries({
			queryKey: chatKey(chatId),
			exact: true,
		});
		const previousChat = queryClient.getQueryData<TypesGen.Chat>(
			chatKey(chatId),
		);
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? { ...chat, archived: true } : chat,
			),
		);
		if (previousChat) {
			queryClient.setQueryData<TypesGen.Chat>(chatKey(chatId), {
				...previousChat,
				archived: true,
			});
		}
		return { previousChat };
	},
	onError: (
		_error: unknown,
		chatId: string,
		context:
			| {
					previousChat?: TypesGen.Chat;
			  }
			| undefined,
	) => {
		// Rollback: invalidate to re-fetch the correct state.
		void invalidateChatListQueries(queryClient);
		if (context?.previousChat) {
			queryClient.setQueryData<TypesGen.Chat>(
				chatKey(chatId),
				context.previousChat,
			);
		}
	},
	onSettled: async (_data: unknown, _error: unknown, chatId: string) => {
		await invalidateChatListQueries(queryClient);
		await queryClient.invalidateQueries({
			queryKey: chatKey(chatId),
			exact: true,
		});
	},
});

export const unarchiveChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) => API.unarchiveChat(chatId),
	onMutate: async (chatId: string) => {
		await queryClient.cancelQueries({
			queryKey: chatsKey,
			predicate: (query) => {
				const key = query.queryKey;
				if (key.length <= 1) return true;
				const segment = key[1];
				return segment === undefined || typeof segment === "object";
			},
		});
		await queryClient.cancelQueries({
			queryKey: chatKey(chatId),
			exact: true,
		});
		const previousChat = queryClient.getQueryData<TypesGen.Chat>(
			chatKey(chatId),
		);
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? { ...chat, archived: false } : chat,
			),
		);
		if (previousChat) {
			queryClient.setQueryData<TypesGen.Chat>(chatKey(chatId), {
				...previousChat,
				archived: false,
			});
		}
		return { previousChat };
	},
	onError: (
		_error: unknown,
		chatId: string,
		context:
			| {
					previousChat?: TypesGen.Chat;
			  }
			| undefined,
	) => {
		// Rollback: invalidate to re-fetch the correct state.
		void invalidateChatListQueries(queryClient);
		if (context?.previousChat) {
			queryClient.setQueryData<TypesGen.Chat>(
				chatKey(chatId),
				context.previousChat,
			);
		}
	},
	onSettled: async (_data: unknown, _error: unknown, chatId: string) => {
		await invalidateChatListQueries(queryClient);
		await queryClient.invalidateQueries({
			queryKey: chatKey(chatId),
			exact: true,
		});
	},
});

export const createChat = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.CreateChatRequest) => API.createChat(req),
	onSuccess: () => {
		void invalidateChatListQueries(queryClient);
	},
});

export const createChatMessage = (
	_queryClient: QueryClient,
	chatId: string,
) => ({
	mutationFn: (req: TypesGen.CreateChatMessageRequest) =>
		API.createChatMessage(chatId, req),
	// No onSuccess invalidation needed: the per-chat WebSocket delivers
	// the response message via upsertDurableMessage, and the global
	// watchChats() WebSocket updates the sidebar sort order.
});

type EditChatMessageMutationArgs = {
	messageId: number;
	req: TypesGen.EditChatMessageRequest;
};

export const editChatMessage = (queryClient: QueryClient, chatId: string) => ({
	mutationFn: ({ messageId, req }: EditChatMessageMutationArgs) =>
		API.editChatMessage(chatId, messageId, req),
	onSuccess: () => {
		// Editing truncates all messages after the edited one on the
		// server. The WebSocket can insert/update messages but cannot
		// remove stale ones, so a full messages refetch is required.
		// Use exact matching to avoid cascading to unrelated queries
		// (diff-status, diff-contents, cost summaries, etc.).
		void queryClient.invalidateQueries({
			queryKey: chatKey(chatId),
			exact: true,
		});
		void queryClient.invalidateQueries({
			queryKey: chatMessagesKey(chatId),
			exact: true,
		});
	},
});

export const interruptChat = (_queryClient: QueryClient, chatId: string) => ({
	mutationFn: () => API.interruptChat(chatId),
	// No onSuccess invalidation needed: the per-chat WebSocket
	// delivers the status change via setChatStatus, and the global
	// watchChats() WebSocket updates the sidebar.
});

export const deleteChatQueuedMessage = (
	queryClient: QueryClient,
	chatId: string,
) => ({
	mutationFn: (queuedMessageId: number) =>
		API.deleteChatQueuedMessage(chatId, queuedMessageId),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatKey(chatId),
			exact: true,
		});
		await queryClient.invalidateQueries({
			queryKey: chatMessagesKey(chatId),
			exact: true,
		});
	},
});

export const promoteChatQueuedMessage = (
	_queryClient: QueryClient,
	chatId: string,
) => ({
	mutationFn: (queuedMessageId: number) =>
		API.promoteChatQueuedMessage(chatId, queuedMessageId),
	// No onSuccess invalidation needed: the per-chat WebSocket
	// delivers the promoted message, queue update, and status
	// change in real-time.
});

export const chatDiffContentsKey = (chatId: string) =>
	["chats", chatId, "diff-contents"] as const;

export const chatDiffContents = (chatId: string) => ({
	queryKey: chatDiffContentsKey(chatId),
	queryFn: () => API.getChatDiffContents(chatId),
});

const chatSystemPromptKey = ["chat-system-prompt"] as const;

export const chatSystemPrompt = () => ({
	queryKey: chatSystemPromptKey,
	queryFn: () => API.getChatSystemPrompt(),
});

export const updateChatSystemPrompt = (queryClient: QueryClient) => ({
	mutationFn: API.updateChatSystemPrompt,
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatSystemPromptKey,
		});
	},
});

const chatUserCustomPromptKey = ["chat-user-custom-prompt"] as const;

export const chatUserCustomPrompt = () => ({
	queryKey: chatUserCustomPromptKey,
	queryFn: () => API.getUserChatCustomPrompt(),
});

export const updateUserChatCustomPrompt = (queryClient: QueryClient) => ({
	mutationFn: API.updateUserChatCustomPrompt,
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatUserCustomPromptKey,
		});
	},
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

type ChatCostDateParams = {
	start_date?: string;
	end_date?: string;
};

type ChatCostUsersParams = ChatCostDateParams & {
	username?: string;
	limit?: number;
	offset?: number;
};

export const chatCostSummaryKey = (user = "me", params?: ChatCostDateParams) =>
	[...chatsKey, "costSummary", user, params] as const;

export const chatCostSummary = (user = "me", params?: ChatCostDateParams) => ({
	queryKey: chatCostSummaryKey(user, params),
	queryFn: () => API.getChatCostSummary(user, params),
	staleTime: 60_000,
});

export const chatCostUsersKey = (params?: ChatCostUsersParams) =>
	[...chatsKey, "costUsers", params] as const;

export const chatCostUsers = (params?: ChatCostUsersParams) => ({
	queryKey: chatCostUsersKey(params),
	queryFn: () => API.getChatCostUsers(params),
	staleTime: 60_000,
});

export const chatUsageLimitConfigKey = [
	...chatsKey,
	"usageLimitConfig",
] as const;

export const chatUsageLimitConfig = () => ({
	queryKey: chatUsageLimitConfigKey,
	queryFn: () => API.getChatUsageLimitConfig(),
});

export const updateChatUsageLimitConfig = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.ChatUsageLimitConfig) =>
		API.updateChatUsageLimitConfig(req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatUsageLimitConfigKey,
		});
	},
});

type UpsertChatUsageLimitOverrideMutationArgs = {
	userID: string;
	req: TypesGen.UpsertChatUsageLimitOverrideRequest;
};

export const upsertChatUsageLimitOverride = (queryClient: QueryClient) => ({
	mutationFn: ({ userID, req }: UpsertChatUsageLimitOverrideMutationArgs) =>
		API.upsertChatUsageLimitOverride(userID, req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatUsageLimitConfigKey,
		});
	},
});

export const deleteChatUsageLimitOverride = (queryClient: QueryClient) => ({
	mutationFn: (userID: string) => API.deleteChatUsageLimitOverride(userID),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatUsageLimitConfigKey,
		});
	},
});

type UpsertChatUsageLimitGroupOverrideMutationArgs = {
	groupID: string;
	req: TypesGen.UpsertChatUsageLimitGroupOverrideRequest;
};

export const upsertChatUsageLimitGroupOverride = (
	queryClient: QueryClient,
) => ({
	mutationFn: ({
		groupID,
		req,
	}: UpsertChatUsageLimitGroupOverrideMutationArgs) =>
		API.upsertChatUsageLimitGroupOverride(groupID, req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatUsageLimitConfigKey,
		});
	},
});

export const deleteChatUsageLimitGroupOverride = (
	queryClient: QueryClient,
) => ({
	mutationFn: (groupID: string) =>
		API.deleteChatUsageLimitGroupOverride(groupID),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatUsageLimitConfigKey,
		});
	},
});
