import type {
	InfiniteData,
	QueryClient,
	UseInfiniteQueryOptions,
} from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

export const chatsKey = ["chats"] as const;
export const chatKey = (chatId: string) => ["chats", chatId] as const;
export const chatMessagesKey = (chatId: string) =>
	["chats", chatId, "messages"] as const;

export const chatsByWorkspaceKeyPrefix = [...chatsKey, "by-workspace"] as const;

export const chatsByWorkspace = (workspaceIds: string[]) => {
	const sorted = workspaceIds.toSorted();
	return {
		queryKey: [...chatsKey, "by-workspace", sorted],
		queryFn: () => API.experimental.getChatsByWorkspace(sorted),
		enabled: workspaceIds.length > 0,
	};
};

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

const getNextOptimisticPinOrder = (queryClient: QueryClient): number => {
	let maxPinOrder = 0;
	const queries = queryClient.getQueriesData<
		TypesGen.Chat[] | { pages: TypesGen.Chat[][]; pageParams: unknown[] }
	>({
		queryKey: chatsKey,
		predicate: isChatListQuery,
	});

	for (const [, data] of queries) {
		if (!data) {
			continue;
		}

		if (Array.isArray(data)) {
			for (const chat of data) {
				maxPinOrder = Math.max(maxPinOrder, chat.pin_order);
			}
			continue;
		}

		for (const page of data.pages) {
			for (const chat of page) {
				maxPinOrder = Math.max(maxPinOrder, chat.pin_order);
			}
		}
	}

	return maxPinOrder + 1;
};

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

/**
 * Predicate that matches chat-list queries performing a regular
 * refetch (window-focus, invalidation, mount) but not a
 * fetchNextPage or fetchPreviousPage. During pagination fetches
 * react-query sets fetchMeta.fetchMore.direction to "forward"
 * or "backward"; regular refetches leave fetchMeta null.
 *
 * Also excludes queries that have never loaded data. Cancelling
 * a first-ever fetch with revert:true leaves the query stuck in
 * { status: 'pending', fetchStatus: 'idle', data: undefined }
 * with no automatic recovery, so the sidebar shows skeletons
 * forever until the user refocuses the window.
 */
const isChatListRefetch = (query: {
	queryKey: readonly unknown[];
	state: { data: unknown; fetchMeta: unknown };
}): boolean => {
	if (!isChatListQuery(query)) return false;
	// Never cancel the initial load. Reverting a first-ever
	// fetch produces a stuck pending/idle state that react-query
	// does not automatically recover from.
	if (query.state.data === undefined) return false;
	const meta = query.state.fetchMeta as {
		fetchMore?: { direction?: string };
	} | null;
	if (meta?.fetchMore?.direction) return false;
	return true;
};

/**
 * Cancel in-flight background refetches for sidebar chat-list
 * queries, but leave fetchNextPage / fetchPreviousPage fetches
 * alone. Call this before writing WebSocket-driven cache
 * updates so a concurrent refetch cannot overwrite the update
 * with stale server data.
 *
 * Pagination fetches are intentionally excluded because
 * cancelling them would prevent the sidebar from loading
 * additional pages when WebSocket events arrive frequently.
 *
 * Mutation onMutate handlers should keep the broad
 * isChatListQuery predicate instead: mutations are infrequent
 * and must cancel pagination fetches to protect optimistic
 * updates from being overwritten by the oldPages snapshot
 * that fetchNextPage captured before the mutation.
 */
export const cancelChatListRefetches = (queryClient: QueryClient) => {
	return queryClient.cancelQueries({
		queryKey: chatsKey,
		predicate: isChatListRefetch,
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
			return API.experimental.getChats({
				limit,
				offset: pageParam <= 0 ? 0 : (pageParam - 1) * limit,
				q,
			});
		},
		refetchOnWindowFocus: true as const,
		retry: 3,
	} satisfies UseInfiniteQueryOptions<TypesGen.Chat[]>;
};

export const chat = (chatId: string) => ({
	queryKey: chatKey(chatId),
	queryFn: () => API.experimental.getChat(chatId),
});

const MESSAGES_PAGE_SIZE = 50;

export const chatMessagesForInfiniteScroll = (chatId: string) => ({
	queryKey: chatMessagesKey(chatId),
	initialPageParam: undefined as number | undefined,
	queryFn: ({ pageParam }: { pageParam: number | undefined }) =>
		API.experimental.getChatMessages(chatId, {
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
	mutationFn: (chatId: string) =>
		API.experimental.updateChat(chatId, { archived: true }),
	onMutate: async (chatId: string) => {
		await queryClient.cancelQueries({
			queryKey: chatsKey,
			predicate: isChatListQuery,
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
		await queryClient.invalidateQueries({
			queryKey: chatsByWorkspaceKeyPrefix,
		});
	},
});

export const unarchiveChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) =>
		API.experimental.updateChat(chatId, { archived: false }),
	onMutate: async (chatId: string) => {
		await queryClient.cancelQueries({
			queryKey: chatsKey,
			predicate: isChatListQuery,
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
		await queryClient.invalidateQueries({
			queryKey: chatsByWorkspaceKeyPrefix,
		});
	},
});

export const pinChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) =>
		API.experimental.updateChat(chatId, { pin_order: 1 }),
	onMutate: async (chatId: string) => {
		await queryClient.cancelQueries({
			queryKey: chatsKey,
			predicate: isChatListQuery,
		});
		await queryClient.cancelQueries({
			queryKey: chatKey(chatId),
			exact: true,
		});
		const previousChat = queryClient.getQueryData<TypesGen.Chat>(
			chatKey(chatId),
		);
		const optimisticPinOrder = getNextOptimisticPinOrder(queryClient);
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? { ...chat, pin_order: optimisticPinOrder } : chat,
			),
		);
		if (previousChat) {
			queryClient.setQueryData<TypesGen.Chat>(chatKey(chatId), {
				...previousChat,
				pin_order: optimisticPinOrder,
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

export const unpinChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) =>
		API.experimental.updateChat(chatId, { pin_order: 0 }),
	onMutate: async (chatId: string) => {
		await queryClient.cancelQueries({
			queryKey: chatsKey,
			predicate: isChatListQuery,
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
				chat.id === chatId ? { ...chat, pin_order: 0 } : chat,
			),
		);
		if (previousChat) {
			queryClient.setQueryData<TypesGen.Chat>(chatKey(chatId), {
				...previousChat,
				pin_order: 0,
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

export const reorderPinnedChat = (queryClient: QueryClient) => ({
	mutationFn: ({ chatId, pinOrder }: { chatId: string; pinOrder: number }) =>
		API.experimental.updateChat(chatId, { pin_order: pinOrder }),
	onMutate: async ({
		chatId,
		pinOrder,
	}: {
		chatId: string;
		pinOrder: number;
	}) => {
		await queryClient.cancelQueries({
			queryKey: chatsKey,
			predicate: isChatListQuery,
		});
		await queryClient.cancelQueries({
			queryKey: chatKey(chatId),
			exact: true,
		});

		// Optimistically reorder pinned chats in the cache so the
		// sidebar reflects the new order immediately without waiting
		// for the server round-trip.
		const allChats = readInfiniteChatsCache(queryClient) ?? [];
		const pinned = allChats
			.filter((c) => c.pin_order > 0)
			.sort((a, b) => a.pin_order - b.pin_order);
		const oldIdx = pinned.findIndex((c) => c.id === chatId);
		if (oldIdx !== -1) {
			const moved = pinned.splice(oldIdx, 1)[0];
			pinned.splice(pinOrder - 1, 0, moved);
			const newOrders = new Map(pinned.map((c, i) => [c.id, i + 1]));
			updateInfiniteChatsCache(queryClient, (chats) =>
				chats.map((c) => {
					const order = newOrders.get(c.id);
					return order !== undefined ? { ...c, pin_order: order } : c;
				}),
			);
		}
	},
	onSettled: async (
		_data: unknown,
		_error: unknown,
		{ chatId }: { chatId: string; pinOrder: number },
	) => {
		await invalidateChatListQueries(queryClient);
		await queryClient.invalidateQueries({
			queryKey: chatKey(chatId),
			exact: true,
		});
	},
});

export const regenerateChatTitle = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) => API.experimental.regenerateChatTitle(chatId),

	onSuccess: (updatedChat: TypesGen.Chat) => {
		queryClient.setQueryData<TypesGen.Chat>(
			chatKey(updatedChat.id),
			(previousChat) =>
				previousChat ? { ...previousChat, ...updatedChat } : updatedChat,
		);
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === updatedChat.id
					? { ...chat, title: updatedChat.title }
					: chat,
			),
		);
	},

	onSettled: async (
		_data: TypesGen.Chat | undefined,
		_error: unknown,
		chatId: string,
	) => {
		await invalidateChatListQueries(queryClient);
		await queryClient.invalidateQueries({
			queryKey: chatKey(chatId),
			exact: true,
		});
	},
});

export const createChat = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.CreateChatRequest) =>
		API.experimental.createChat(req),
	onSuccess: () => {
		void invalidateChatListQueries(queryClient);
		void queryClient.invalidateQueries({
			queryKey: chatsByWorkspaceKeyPrefix,
		});
	},
});

export const createChatMessage = (
	_queryClient: QueryClient,
	chatId: string,
) => ({
	mutationFn: (req: TypesGen.CreateChatMessageRequest) =>
		API.experimental.createChatMessage(chatId, req),
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
		API.experimental.editChatMessage(chatId, messageId, req),
	onMutate: async ({ messageId }: EditChatMessageMutationArgs) => {
		// Cancel in-flight refetches so they don't overwrite the
		// optimistic update before the mutation completes.
		await queryClient.cancelQueries({
			queryKey: chatMessagesKey(chatId),
			exact: true,
		});

		const previousData = queryClient.getQueryData<
			InfiniteData<TypesGen.ChatMessagesResponse>
		>(chatMessagesKey(chatId));

		// Optimistically remove the edited message and everything
		// after it. The server soft-deletes these and inserts a
		// replacement with a new ID. Without this, the WebSocket
		// handler's upsertCacheMessages adds new messages to the
		// React Query cache without removing the soft-deleted ones,
		// causing deleted messages to flash back into view until
		// the full REST refetch resolves.
		queryClient.setQueryData<
			InfiniteData<TypesGen.ChatMessagesResponse> | undefined
		>(chatMessagesKey(chatId), (current) => {
			if (!current?.pages?.length) {
				return current;
			}
			return {
				...current,
				pages: current.pages.map((page) => ({
					...page,
					messages: page.messages.filter((m) => m.id < messageId),
				})),
			};
		});

		return { previousData };
	},
	onError: (
		_error: unknown,
		_variables: EditChatMessageMutationArgs,
		context:
			| {
					previousData?:
						| InfiniteData<TypesGen.ChatMessagesResponse>
						| undefined;
			  }
			| undefined,
	) => {
		// Restore the cache on failure so the user sees the
		// original messages again.
		if (context?.previousData) {
			queryClient.setQueryData(chatMessagesKey(chatId), context.previousData);
		}
	},
	onSettled: () => {
		// Always reconcile with the server regardless of whether
		// the mutation succeeded or failed. On success this picks
		// up the replacement message; on failure it confirms the
		// restore from onError matches the server state. Use exact
		// matching to avoid cascading to unrelated queries
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
	mutationFn: () => API.experimental.interruptChat(chatId),
	// No onSuccess invalidation needed: the per-chat WebSocket
	// delivers the status change via setChatStatus, and the global
	// watchChats() WebSocket updates the sidebar.
});

export const deleteChatQueuedMessage = (
	queryClient: QueryClient,
	chatId: string,
) => ({
	mutationFn: (queuedMessageId: number) =>
		API.experimental.deleteChatQueuedMessage(chatId, queuedMessageId),
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
		API.experimental.promoteChatQueuedMessage(chatId, queuedMessageId),
	// No onSuccess invalidation needed: the caller upserts the
	// promoted message from the response, and the per-chat
	// WebSocket delivers queue and status updates in real-time.
});

export const chatDiffContentsKey = (chatId: string) =>
	["chats", chatId, "diff-contents"] as const;

export const chatDiffContents = (chatId: string) => ({
	queryKey: chatDiffContentsKey(chatId),
	queryFn: () => API.experimental.getChatDiffContents(chatId),
});

const chatSystemPromptKey = ["chat-system-prompt"] as const;

export const chatSystemPrompt = () => ({
	queryKey: chatSystemPromptKey,
	queryFn: () => API.experimental.getChatSystemPrompt(),
});

export const updateChatSystemPrompt = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.UpdateChatSystemPromptRequest) =>
		API.experimental.updateChatSystemPrompt(req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatSystemPromptKey,
		});
	},
});

const chatDesktopEnabledKey = ["chat-desktop-enabled"] as const;

export const chatDesktopEnabled = () => ({
	queryKey: chatDesktopEnabledKey,
	queryFn: () => API.experimental.getChatDesktopEnabled(),
});

export const updateChatDesktopEnabled = (queryClient: QueryClient) => ({
	mutationFn: API.experimental.updateChatDesktopEnabled,
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatDesktopEnabledKey,
		});
	},
});

const chatWorkspaceTTLKey = ["chat-workspace-ttl"] as const;

export const chatWorkspaceTTL = () => ({
	queryKey: chatWorkspaceTTLKey,
	queryFn: () => API.experimental.getChatWorkspaceTTL(),
});

export const updateChatWorkspaceTTL = (queryClient: QueryClient) => ({
	mutationFn: API.experimental.updateChatWorkspaceTTL,
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatWorkspaceTTLKey,
		});
	},
});

const chatRetentionDaysKey = ["chat-retention-days"] as const;

export const chatRetentionDays = () => ({
	queryKey: chatRetentionDaysKey,
	queryFn: () => API.experimental.getChatRetentionDays(),
});

export const updateChatRetentionDays = (queryClient: QueryClient) => ({
	mutationFn: API.experimental.updateChatRetentionDays,
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatRetentionDaysKey,
		});
	},
});

const chatTemplateAllowlistKey = ["chat-template-allowlist"] as const;

export const chatTemplateAllowlist = () => ({
	queryKey: chatTemplateAllowlistKey,
	queryFn: () => API.experimental.getChatTemplateAllowlist(),
});

export const updateChatTemplateAllowlist = (queryClient: QueryClient) => ({
	mutationFn: API.experimental.updateChatTemplateAllowlist,
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatTemplateAllowlistKey,
		});
	},
});

const chatUserCustomPromptKey = ["chat-user-custom-prompt"] as const;

export const chatUserCustomPrompt = () => ({
	queryKey: chatUserCustomPromptKey,
	queryFn: () => API.experimental.getUserChatCustomPrompt(),
});

export const updateUserChatCustomPrompt = (queryClient: QueryClient) => ({
	mutationFn: API.experimental.updateUserChatCustomPrompt,
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatUserCustomPromptKey,
		});
	},
});

const userCompactionThresholdsKey = [
	"chat-user-compaction-thresholds",
] as const;

export const userCompactionThresholds = () => ({
	queryKey: userCompactionThresholdsKey,
	queryFn: () => API.experimental.getUserChatCompactionThresholds(),
});

export const updateUserCompactionThreshold = (queryClient: QueryClient) => ({
	mutationFn: (vars: {
		modelConfigId: string;
		req: TypesGen.UpdateUserChatCompactionThresholdRequest;
	}) =>
		API.experimental.updateUserChatCompactionThreshold(
			vars.modelConfigId,
			vars.req,
		),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: userCompactionThresholdsKey,
		});
	},
});

export const deleteUserCompactionThreshold = (queryClient: QueryClient) => ({
	mutationFn: (modelConfigId: string) =>
		API.experimental.deleteUserChatCompactionThreshold(modelConfigId),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: userCompactionThresholdsKey,
		});
	},
});

export const chatModelsKey = ["chat-models"] as const;

export const chatModels = () => ({
	queryKey: chatModelsKey,
	queryFn: (): Promise<TypesGen.ChatModelsResponse> =>
		API.experimental.getChatModels(),
});

const chatProviderConfigsKey = ["chat-provider-configs"] as const;

export const chatProviderConfigs = () => ({
	queryKey: chatProviderConfigsKey,
	queryFn: (): Promise<TypesGen.ChatProviderConfig[]> =>
		API.experimental.getChatProviderConfigs(),
});

const chatModelConfigsKey = ["chat-model-configs"] as const;

export const chatModelConfigs = () => ({
	queryKey: chatModelConfigsKey,
	queryFn: (): Promise<TypesGen.ChatModelConfig[]> =>
		API.experimental.getChatModelConfigs(),
});

export const userChatProviderConfigsKey = [
	"user-chat-provider-configs",
] as const;

export const userChatProviderConfigs = () => ({
	queryKey: userChatProviderConfigsKey,
	queryFn: (): Promise<TypesGen.UserChatProviderConfig[]> =>
		API.experimental.getUserChatProviderConfigs(),
});

type UpsertUserChatProviderKeyArgs = {
	providerConfigId: string;
	req: TypesGen.CreateUserChatProviderKeyRequest;
};

export const upsertUserChatProviderKey = (queryClient: QueryClient) => ({
	mutationFn: ({ providerConfigId, req }: UpsertUserChatProviderKeyArgs) =>
		API.experimental.upsertUserChatProviderKey(providerConfigId, req),
	onSuccess: async () => {
		await Promise.all([
			queryClient.invalidateQueries({
				queryKey: userChatProviderConfigsKey,
			}),
			queryClient.invalidateQueries({ queryKey: chatModelsKey }),
		]);
	},
});

export const deleteUserChatProviderKey = (queryClient: QueryClient) => ({
	mutationFn: (providerConfigId: string) =>
		API.experimental.deleteUserChatProviderKey(providerConfigId),
	onSuccess: async () => {
		await Promise.all([
			queryClient.invalidateQueries({
				queryKey: userChatProviderConfigsKey,
			}),
			queryClient.invalidateQueries({ queryKey: chatModelsKey }),
		]);
	},
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
		API.experimental.createChatProviderConfig(req),
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
		API.experimental.updateChatProviderConfig(providerConfigId, req),
	onSuccess: async () => {
		await invalidateChatConfigurationQueries(queryClient);
	},
});

export const deleteChatProviderConfig = (queryClient: QueryClient) => ({
	mutationFn: (providerConfigId: string) =>
		API.experimental.deleteChatProviderConfig(providerConfigId),
	onSuccess: async () => {
		await invalidateChatConfigurationQueries(queryClient);
	},
});

export const createChatModelConfig = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.CreateChatModelConfigRequest) =>
		API.experimental.createChatModelConfig(req),
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
		API.experimental.updateChatModelConfig(modelConfigId, req),
	onSuccess: async () => {
		await invalidateChatConfigurationQueries(queryClient);
	},
});

export const deleteChatModelConfig = (queryClient: QueryClient) => ({
	mutationFn: (modelConfigId: string) =>
		API.experimental.deleteChatModelConfig(modelConfigId),
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
	queryFn: () => API.experimental.getChatCostSummary(user, params),
	staleTime: 60_000,
});

export const chatCostUsersKey = (params?: ChatCostUsersParams) =>
	[...chatsKey, "costUsers", params] as const;

export const chatCostUsers = (params?: ChatCostUsersParams) => ({
	queryKey: chatCostUsersKey(params),
	queryFn: () => API.experimental.getChatCostUsers(params),
	staleTime: 60_000,
});

const prInsightsKey = (params?: { start_date?: string; end_date?: string }) =>
	[...chatsKey, "prInsights", params] as const;

export const prInsights = (params?: {
	start_date?: string;
	end_date?: string;
}) => ({
	queryKey: prInsightsKey(params),
	queryFn: () => API.experimental.getPRInsights(params),
	staleTime: 60_000,
});

export const chatUsageLimitStatusKey = [
	...chatsKey,
	"usageLimitStatus",
] as const;

export const chatUsageLimitStatus = () => ({
	queryKey: chatUsageLimitStatusKey,
	queryFn: () => API.experimental.getChatUsageLimitStatus(),
	refetchInterval: 60_000,
});

const chatUsageLimitConfigKey = [...chatsKey, "usageLimitConfig"] as const;

export const chatUsageLimitConfig = () => ({
	queryKey: chatUsageLimitConfigKey,
	queryFn: () => API.experimental.getChatUsageLimitConfig(),
});

export const updateChatUsageLimitConfig = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.ChatUsageLimitConfig) =>
		API.experimental.updateChatUsageLimitConfig(req),
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
		API.experimental.upsertChatUsageLimitOverride(userID, req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatUsageLimitConfigKey,
		});
	},
});

export const deleteChatUsageLimitOverride = (queryClient: QueryClient) => ({
	mutationFn: (userID: string) =>
		API.experimental.deleteChatUsageLimitOverride(userID),
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
		API.experimental.upsertChatUsageLimitGroupOverride(groupID, req),
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
		API.experimental.deleteChatUsageLimitGroupOverride(groupID),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatUsageLimitConfigKey,
		});
	},
});

// ── MCP Server Configs ───────────────────────────────────────

export const mcpServerConfigsKey = ["mcp-server-configs"] as const;

export const mcpServerConfigs = () => ({
	queryKey: mcpServerConfigsKey,
	queryFn: (): Promise<TypesGen.MCPServerConfig[]> =>
		API.experimental.getMCPServerConfigs(),
});

const invalidateMCPServerConfigQueries = async (queryClient: QueryClient) => {
	await queryClient.invalidateQueries({ queryKey: mcpServerConfigsKey });
};

export const createMCPServerConfig = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.CreateMCPServerConfigRequest) =>
		API.experimental.createMCPServerConfig(req),
	onSuccess: async () => {
		await invalidateMCPServerConfigQueries(queryClient);
	},
});

type UpdateMCPServerConfigMutationArgs = {
	id: string;
	req: TypesGen.UpdateMCPServerConfigRequest;
};

export const updateMCPServerConfig = (queryClient: QueryClient) => ({
	mutationFn: ({ id, req }: UpdateMCPServerConfigMutationArgs) =>
		API.experimental.updateMCPServerConfig(id, req),
	onSuccess: async () => {
		await invalidateMCPServerConfigQueries(queryClient);
	},
});

export const deleteMCPServerConfig = (queryClient: QueryClient) => ({
	mutationFn: (id: string) => API.experimental.deleteMCPServerConfig(id),
	onSuccess: async () => {
		await invalidateMCPServerConfigQueries(queryClient);
	},
});
