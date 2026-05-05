import {
	type InfiniteData,
	type QueryClient,
	queryOptions,
	type UseInfiniteQueryOptions,
} from "react-query";
import {
	API,
	type ChatPlanModeOrClear,
	type CreateChatMessageRequestWithClearablePlanMode,
} from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import type { UsePaginatedQueryOptions } from "#/hooks/usePaginatedQuery";
import {
	projectEditedConversationIntoCache,
	reconcileEditedMessageInCache,
} from "./chatMessageEdits";

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

/**
 * Adds a child chat to its parent's `children` array across all
 * infinite chat query caches. If the parent is not in any loaded page,
 * the child is silently dropped (it will appear when the parent loads).
 */
export const addChildToParentInCache = (
	queryClient: QueryClient,
	child: TypesGen.Chat,
	parentId: string,
) => {
	updateInfiniteChatsCache(queryClient, (chats) => {
		let changed = false;
		const next = chats.map((c) => {
			if (c.id !== parentId) return c;
			// Avoid duplicates.
			if (c.children?.some((ch) => ch.id === child.id)) return c;
			changed = true;
			return { ...c, children: [child, ...(c.children ?? [])] };
		});
		return changed ? next : chats;
	});
};

/**
 * Updates a child chat within its parent's `children` array across all
 * infinite chat query caches. Returns true if the child was found and
 * updated, false otherwise.
 */
export const updateChildInParentCache = (
	queryClient: QueryClient,
	updater: (child: TypesGen.Chat) => TypesGen.Chat,
	childId: string,
) => {
	let found = false;
	updateInfiniteChatsCache(queryClient, (chats) => {
		let changed = false;
		const next = chats.map((c) => {
			if (!c.children?.length) return c;
			let childChanged = false;
			const nextChildren = c.children.map((ch) => {
				if (ch.id !== childId) return ch;
				const updated = updater(ch);
				if (updated !== ch) {
					childChanged = true;
					found = true;
				}
				return updated;
			});
			if (!childChanged) return c;
			changed = true;
			return { ...c, children: nextChildren };
		});
		return changed ? next : chats;
	});
	return found;
};

/**
 * Removes a child chat from its parent's `children` array across all
 * infinite chat query caches. Returns true if the child was found and
 * removed, false otherwise. Used when a child is archived individually
 * (the sidebar hides children whose archive state differs from the
 * parent) and when a `deleted` pubsub event arrives for a child chat.
 */
export const removeChildFromParentInCache = (
	queryClient: QueryClient,
	childId: string,
) => {
	let found = false;
	updateInfiniteChatsCache(queryClient, (chats) => {
		let changed = false;
		const next = chats.map((c) => {
			if (!c.children?.length) return c;
			const filtered = c.children.filter((ch) => ch.id !== childId);
			if (filtered.length === c.children.length) return c;
			found = true;
			changed = true;
			return { ...c, children: filtered };
		});
		return changed ? next : chats;
	});
	return found;
};

const parseUpdatedAtInstant = (updatedAt: string) => {
	const match = updatedAt.match(/^(.*?)(?:\.(\d+))?(Z|[+-]\d\d:\d\d)$/);
	if (!match) {
		const epochMs = Date.parse(updatedAt);
		return Number.isNaN(epochMs) ? undefined : { epochMs, fractionalNanos: 0 };
	}

	const [, timestampWithoutFraction, fractionalSeconds = "", timezone] = match;
	const epochMs = Date.parse(`${timestampWithoutFraction}${timezone}`);
	if (Number.isNaN(epochMs)) {
		return undefined;
	}
	return {
		epochMs,
		fractionalNanos: Number(fractionalSeconds.slice(0, 9).padEnd(9, "0")),
	};
};

const compareUpdatedAtInstants = (a: string, b: string): number => {
	const parsedA = parseUpdatedAtInstant(a);
	const parsedB = parseUpdatedAtInstant(b);
	if (!parsedA || !parsedB) {
		return a.localeCompare(b);
	}
	if (parsedA.epochMs !== parsedB.epochMs) {
		return parsedA.epochMs - parsedB.epochMs;
	}
	return parsedA.fractionalNanos - parsedB.fractionalNanos;
};

type MergeWatchedChatOptions = {
	readonly eventKind: TypesGen.ChatWatchEventKind;
	readonly activeChatId?: string;
};

// Shallow-compare two ChatDiffStatus objects by their meaningful
// fields, ignoring refreshed_at/stale_at which change on every poll.
const diffStatusEqual = (
	a: TypesGen.ChatDiffStatus | undefined,
	b: TypesGen.ChatDiffStatus | undefined,
): boolean => {
	if (a === b) {
		return true;
	}
	if (!a || !b) {
		return false;
	}
	return (
		a.url === b.url &&
		a.pull_request_state === b.pull_request_state &&
		a.pull_request_title === b.pull_request_title &&
		a.pull_request_draft === b.pull_request_draft &&
		a.changes_requested === b.changes_requested &&
		a.additions === b.additions &&
		a.deletions === b.deletions &&
		a.changed_files === b.changed_files &&
		a.pr_number === b.pr_number &&
		a.approved === b.approved &&
		a.commits === b.commits
	);
};

/**
 * Merges event-scoped chat fields into a cached summary, using updated_at
 * as a stale guard while still adopting the latest DB-backed model config.
 */
export const mergeWatchedChatSummary = (
	cachedChat: TypesGen.Chat,
	watchedChat: TypesGen.Chat,
	{ eventKind, activeChatId }: MergeWatchedChatOptions,
): TypesGen.Chat => {
	const isTitleEvent = eventKind === "title_change";
	const isStatusEvent = eventKind === "status_change";
	const isDiffStatusEvent = eventKind === "diff_status_change";
	const updatedAtComparison = compareUpdatedAtInstants(
		cachedChat.updated_at,
		watchedChat.updated_at,
	);
	const isFreshEnough = updatedAtComparison <= 0;
	const nextStatus =
		isFreshEnough && isStatusEvent ? watchedChat.status : cachedChat.status;
	// maybeGenerateChatTitle can publish a previously loaded chat snapshot, so
	// apply title_change payloads even when the chat summary timestamp is older.
	const nextTitle = isTitleEvent ? watchedChat.title : cachedChat.title;
	// Diff status freshness is tracked outside chats.updated_at, so apply
	// diff_status_change payloads even when the chat summary timestamp is older.
	const nextDiffStatus = isDiffStatusEvent
		? watchedChat.diff_status
		: cachedChat.diff_status;
	const nextWorkspaceId = isFreshEnough
		? (watchedChat.workspace_id ?? cachedChat.workspace_id)
		: cachedChat.workspace_id;
	const nextBuildId = isFreshEnough
		? (watchedChat.build_id ?? cachedChat.build_id)
		: cachedChat.build_id;
	// All event types carry the current model config from the DB.
	const nextLastModelConfigId = isFreshEnough
		? watchedChat.last_model_config_id
		: cachedChat.last_model_config_id;
	const nextHasUnread =
		isFreshEnough && isStatusEvent && watchedChat.id !== activeChatId
			? true
			: cachedChat.has_unread;
	const nextUpdatedAt =
		updatedAtComparison > 0 ? cachedChat.updated_at : watchedChat.updated_at;

	// Keep updated_at in the no-op guard. This gives up the old streaming
	// rerender shortcut so later stale events cannot pass isFreshEnough
	// against a timestamp that should already have been superseded.
	if (
		nextStatus === cachedChat.status &&
		nextTitle === cachedChat.title &&
		diffStatusEqual(nextDiffStatus, cachedChat.diff_status) &&
		nextWorkspaceId === cachedChat.workspace_id &&
		nextBuildId === cachedChat.build_id &&
		nextLastModelConfigId === cachedChat.last_model_config_id &&
		nextHasUnread === cachedChat.has_unread &&
		nextUpdatedAt === cachedChat.updated_at
	) {
		return cachedChat;
	}

	return {
		...cachedChat,
		status: nextStatus,
		title: nextTitle,
		diff_status: nextDiffStatus,
		workspace_id: nextWorkspaceId,
		build_id: nextBuildId,
		last_model_config_id: nextLastModelConfigId,
		has_unread: nextHasUnread,
		updated_at: nextUpdatedAt,
	};
};

/**
 * Applies the same event-scoped merge and stale guard across the list,
 * parent-child, and per-chat caches, covering all three cache layers.
 */
export const mergeWatchedChatIntoCaches = (
	queryClient: QueryClient,
	watchedChat: TypesGen.Chat,
	options: MergeWatchedChatOptions,
) => {
	const mergeCachedChat = (cachedChat: TypesGen.Chat) =>
		mergeWatchedChatSummary(cachedChat, watchedChat, options);

	updateInfiniteChatsCache(queryClient, (chats) => {
		let didUpdate = false;
		const nextChats = chats.map((chat) => {
			if (chat.id !== watchedChat.id) {
				return chat;
			}
			const mergedChat = mergeCachedChat(chat);
			if (mergedChat !== chat) {
				didUpdate = true;
			}
			return mergedChat;
		});
		return didUpdate ? nextChats : chats;
	});

	updateChildInParentCache(queryClient, mergeCachedChat, watchedChat.id);
	queryClient.setQueryData<TypesGen.Chat | undefined>(
		chatKey(watchedChat.id),
		(cachedChat) => {
			if (!cachedChat) {
				return cachedChat;
			}
			return mergeCachedChat(cachedChat);
		},
	);
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

type UpdateChatWorkspaceVariables = {
	chatId: string;
	workspaceId: string | null;
};

type UpdateChatPlanModeVariables = {
	chatId: string;
	planMode?: TypesGen.ChatPlanMode;
};

const CLEAR_PLAN_MODE_WIRE_VALUE = "" satisfies ChatPlanModeOrClear;

const toChatPlanModePayload = (
	planMode: TypesGen.ChatPlanMode | undefined,
): ChatPlanModeOrClear => {
	// The API expects an empty string on the wire to clear plan mode.
	return planMode ?? CLEAR_PLAN_MODE_WIRE_VALUE;
};

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
		// Flip archived flag in the flat root list; strip the
		// chat from any parent's embedded children (individual
		// child archive).
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? { ...chat, archived: true } : chat,
			),
		);
		removeChildFromParentInCache(queryClient, chatId);
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

export const updateChatPlanMode = (queryClient: QueryClient) => ({
	mutationFn: ({ chatId, planMode }: UpdateChatPlanModeVariables) =>
		API.experimental.updateChat(chatId, {
			plan_mode: toChatPlanModePayload(planMode),
		}),
	onMutate: async ({ chatId, planMode }: UpdateChatPlanModeVariables) => {
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
				chat.id === chatId ? { ...chat, plan_mode: planMode } : chat,
			),
		);
		if (previousChat) {
			queryClient.setQueryData<TypesGen.Chat>(chatKey(chatId), {
				...previousChat,
				plan_mode: planMode,
			});
		}
		return { previousChat };
	},
	onError: (
		_error: unknown,
		{ chatId }: UpdateChatPlanModeVariables,
		context:
			| {
					previousChat?: TypesGen.Chat;
			  }
			| undefined,
	) => {
		void invalidateChatListQueries(queryClient);
		const previousChat = context?.previousChat;
		if (!previousChat) {
			return;
		}
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId
					? {
							...chat,
							plan_mode: previousChat.plan_mode,
						}
					: chat,
			),
		);
		queryClient.setQueryData<TypesGen.Chat>(chatKey(chatId), previousChat);
	},
});

export const updateChatWorkspace = (queryClient: QueryClient) => ({
	mutationFn: ({ chatId, workspaceId }: UpdateChatWorkspaceVariables) =>
		API.experimental.updateChat(chatId, {
			workspace_id:
				workspaceId ??
				// The API uses the nil UUID to clear the workspace association.
				"00000000-0000-0000-0000-000000000000",
		}),
	onMutate: async ({ chatId, workspaceId }: UpdateChatWorkspaceVariables) => {
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
				chat.id === chatId
					? { ...chat, workspace_id: workspaceId ?? undefined }
					: chat,
			),
		);
		if (previousChat) {
			queryClient.setQueryData<TypesGen.Chat>(chatKey(chatId), {
				...previousChat,
				workspace_id: workspaceId ?? undefined,
			});
		}
		return { previousChat };
	},
	onError: (
		_error: unknown,
		{ chatId }: UpdateChatWorkspaceVariables,
		context:
			| {
					previousChat?: TypesGen.Chat;
			  }
			| undefined,
	) => {
		void invalidateChatListQueries(queryClient);
		const previousChat = context?.previousChat;
		if (previousChat) {
			updateInfiniteChatsCache(queryClient, (chats) =>
				chats.map((chat) =>
					chat.id === chatId
						? {
								...chat,
								workspace_id: previousChat.workspace_id,
							}
						: chat,
				),
			);
			queryClient.setQueryData<TypesGen.Chat>(chatKey(chatId), previousChat);
		}
	},
	onSettled: async (
		_data: unknown,
		_error: unknown,
		{ chatId }: UpdateChatWorkspaceVariables,
	) => {
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
		void invalidateChatDebugRuns(queryClient, chatId);
	},
});

export const proposeChatTitle = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) => API.experimental.proposeChatTitle(chatId),

	onSettled: (
		_data: { title: string } | undefined,
		_error: unknown,
		chatId: string,
	) => {
		void invalidateChatDebugRuns(queryClient, chatId);
	},
});

type UpdateChatTitleVariables = {
	chatId: string;
	title: string;
};

export const updateChatTitle = (queryClient: QueryClient) => ({
	mutationFn: ({ chatId, title }: UpdateChatTitleVariables) =>
		API.experimental.updateChat(chatId, { title }),

	onSuccess: (_data: unknown, { chatId, title }: UpdateChatTitleVariables) => {
		queryClient.setQueryData<TypesGen.Chat | undefined>(
			chatKey(chatId),
			(chat) => (chat ? { ...chat, title } : chat),
		);
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) => (chat.id === chatId ? { ...chat, title } : chat)),
		);
	},

	onSettled: async (
		_data: unknown,
		_error: unknown,
		{ chatId }: UpdateChatTitleVariables,
	) => {
		await invalidateChatListQueries(queryClient);
		await queryClient.invalidateQueries({
			queryKey: chatKey(chatId),
			exact: true,
		});
	},
});

export const chatDebugRunsKey = (chatId: string) =>
	[...chatKey(chatId), "debug-runs"] as const;

const chatDebugRunKey = (chatId: string, runId: string) =>
	[...chatDebugRunsKey(chatId), runId] as const;

// Foreground poll cadence when the Debug tab is open. The error cadence
// is slower so a transiently unreachable backend is not hammered, but
// the panel still recovers automatically once the request succeeds.
const DEBUG_RUN_POLL_MS = 5_000;
const DEBUG_RUN_ERROR_POLL_MS = 30_000;

// Terminal debug-run statuses that stop the detail query from polling.
// Kept here (rather than imported from the debug panel page) so the
// api/queries layer has no dependency on the page tree. Must stay in
// sync with the success/error classification in the debug panel's
// status-badge logic: any status that renders a non-active badge
// (green/destructive) must end polling, otherwise a successful run
// with status "ok" or "succeeded" would be polled forever. A test in
// chats.test.ts pins this set to the debug panel's SUCCESS/ERROR
// display sets so drift is caught at CI time.
export const TERMINAL_RUN_STATUSES = new Set([
	// Success-like.
	"completed",
	"success",
	"succeeded",
	"ok",
	// Error-like.
	"failed",
	"error",
	"errored",
	"interrupted",
	"cancelled",
	"canceled",
]);

export const chatDebugRuns = (chatId: string) =>
	queryOptions({
		queryKey: chatDebugRunsKey(chatId),
		queryFn: () => API.experimental.getChatDebugRuns(chatId),
		refetchInterval: ({ state }) => {
			// Keep polling on error with backoff so a transient fetch
			// failure does not freeze the panel until a manual remount.
			if (state.status === "error") {
				return DEBUG_RUN_ERROR_POLL_MS;
			}
			// Consistent foreground cadence while the Debug tab is open.
			// A slower terminal-state interval would delay discovery of
			// newly-started runs until the user switches tabs.
			return DEBUG_RUN_POLL_MS;
		},
		refetchIntervalInBackground: false,
	});

export const chatDebugRun = (chatId: string, runId: string) =>
	queryOptions({
		queryKey: chatDebugRunKey(chatId, runId),
		queryFn: () => API.experimental.getChatDebugRun(chatId, runId),
		refetchInterval: ({ state }) => {
			if (state.status === "error") {
				return DEBUG_RUN_ERROR_POLL_MS;
			}
			const status = state.data?.status;
			if (status && TERMINAL_RUN_STATUSES.has(status.toLowerCase())) {
				return false;
			}
			return DEBUG_RUN_POLL_MS;
		},
		refetchIntervalInBackground: false,
	});

const invalidateChatDebugRuns = (queryClient: QueryClient, chatId: string) => {
	return queryClient.invalidateQueries({
		queryKey: chatDebugRunsKey(chatId),
	});
};

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
	queryClient: QueryClient,
	chatId: string,
) => ({
	mutationFn: (req: CreateChatMessageRequestWithClearablePlanMode) =>
		API.experimental.createChatMessage(chatId, req),
	onSuccess: () => {
		void invalidateChatDebugRuns(queryClient, chatId);
	},
});

type EditChatMessageMutationArgs = {
	messageId: number;
	optimisticMessage?: TypesGen.ChatMessage;
	req: TypesGen.EditChatMessageRequest;
};

type EditChatMessageMutationContext = {
	previousData?: InfiniteData<TypesGen.ChatMessagesResponse> | undefined;
};

export const editChatMessage = (queryClient: QueryClient, chatId: string) => ({
	mutationFn: ({ messageId, req }: EditChatMessageMutationArgs) =>
		API.experimental.editChatMessage(chatId, messageId, req),
	onMutate: async ({
		messageId,
		optimisticMessage,
	}: EditChatMessageMutationArgs): Promise<EditChatMessageMutationContext> => {
		// Cancel in-flight refetches so they don't overwrite the
		// optimistic update before the mutation completes.
		await queryClient.cancelQueries({
			queryKey: chatMessagesKey(chatId),
			exact: true,
		});

		const previousData = queryClient.getQueryData<
			InfiniteData<TypesGen.ChatMessagesResponse>
		>(chatMessagesKey(chatId));

		queryClient.setQueryData<
			InfiniteData<TypesGen.ChatMessagesResponse> | undefined
		>(chatMessagesKey(chatId), (current) =>
			projectEditedConversationIntoCache({
				currentData: current,
				editedMessageId: messageId,
				replacementMessage: optimisticMessage,
				queuedMessages: [],
			}),
		);

		return { previousData };
	},
	onError: (
		_error: unknown,
		_variables: EditChatMessageMutationArgs,
		context: EditChatMessageMutationContext | undefined,
	) => {
		// Restore the cache on failure so the user sees the
		// original messages again.
		if (context?.previousData) {
			queryClient.setQueryData(chatMessagesKey(chatId), context.previousData);
		}
		// Invalidate messages as a safety net: the restored snapshot
		// may be missing WebSocket-delivered messages that arrived
		// during the mutation's flight time.
		void queryClient.invalidateQueries({
			queryKey: chatMessagesKey(chatId),
			exact: true,
		});
	},
	onSuccess: (
		response: TypesGen.EditChatMessageResponse,
		variables: EditChatMessageMutationArgs,
	) => {
		queryClient.setQueryData<
			InfiniteData<TypesGen.ChatMessagesResponse> | undefined
		>(chatMessagesKey(chatId), (current) =>
			reconcileEditedMessageInCache({
				currentData: current,
				optimisticMessageId: variables.messageId,
				responseMessage: response.message,
			}),
		);
	},
	onSettled: () => {
		// Refresh chat metadata (status, title, etc.). The messages
		// query is intentionally NOT invalidated here. The per-chat
		// WebSocket handles post-edit message delivery via
		// FullRefresh, making REST invalidation unnecessary.
		// Invalidating chatMessagesKey would trigger a redundant
		// refetch that causes extra store mutations while the
		// sticky user message is settling after the optimistic
		// truncation.
		void queryClient.invalidateQueries({
			queryKey: chatKey(chatId),
			exact: true,
		});
		void invalidateChatDebugRuns(queryClient, chatId);
	},
});

export const interruptChat = (queryClient: QueryClient, chatId: string) => ({
	mutationFn: () => API.experimental.interruptChat(chatId),
	onSuccess: () => {
		void invalidateChatDebugRuns(queryClient, chatId);
	},
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
	queryClient: QueryClient,
	chatId: string,
) => ({
	mutationFn: (queuedMessageId: number) =>
		API.experimental.promoteChatQueuedMessage(chatId, queuedMessageId),
	onSuccess: () => {
		void invalidateChatDebugRuns(queryClient, chatId);
	},
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

const chatPlanModeInstructionsKey = ["chat-plan-mode-instructions"] as const;

export const chatPlanModeInstructions = () => ({
	queryKey: chatPlanModeInstructionsKey,
	queryFn: () => API.experimental.getChatPlanModeInstructions(),
});

export const updateChatPlanModeInstructions = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.UpdateChatPlanModeInstructionsRequest) =>
		API.experimental.updateChatPlanModeInstructions(req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatPlanModeInstructionsKey,
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

const chatPersonalModelOverridesAdminSettingsKey = [
	...chatsKey,
	"admin-personal-model-overrides",
] as const;

export const chatPersonalModelOverridesAdminSettings = () => ({
	queryKey: chatPersonalModelOverridesAdminSettingsKey,
	queryFn: () => API.experimental.getChatPersonalModelOverridesAdminSettings(),
});

export const updateChatPersonalModelOverridesAdminSettings = (
	queryClient: QueryClient,
) => ({
	mutationFn: (
		req: TypesGen.UpdateChatPersonalModelOverridesAdminSettingsRequest,
	) => API.experimental.updateChatPersonalModelOverridesAdminSettings(req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatPersonalModelOverridesAdminSettingsKey,
		});
		await queryClient.invalidateQueries({
			queryKey: userChatPersonalModelOverridesKey,
		});
	},
});

export * from "./chatDebugLogging";
export const chatAdvisorConfigKey = ["chat-advisor-config"] as const;

export const chatAdvisorConfig = () => ({
	queryKey: chatAdvisorConfigKey,
	queryFn: (): Promise<TypesGen.AdvisorConfig> =>
		API.experimental.getChatAdvisorConfig(),
});

export const updateChatAdvisorConfig = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.UpdateAdvisorConfigRequest) =>
		API.experimental.updateChatAdvisorConfig(req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatAdvisorConfigKey,
		});
	},
});

const chatComputerUseProviderKey = ["chat-computer-use-provider"] as const;

export const chatComputerUseProvider = () => ({
	queryKey: chatComputerUseProviderKey,
	queryFn: () => API.experimental.getChatComputerUseProvider(),
});

export const updateChatComputerUseProvider = (queryClient: QueryClient) => ({
	mutationFn: API.experimental.updateChatComputerUseProvider,
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatComputerUseProviderKey,
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

const chatAutoArchiveDaysKey = ["chat-auto-archive-days"] as const;

export const chatAutoArchiveDays = () => ({
	queryKey: chatAutoArchiveDaysKey,
	queryFn: () => API.experimental.getChatAutoArchiveDays(),
});

export const updateChatAutoArchiveDays = (queryClient: QueryClient) => ({
	mutationFn: API.experimental.updateChatAutoArchiveDays,
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatAutoArchiveDaysKey,
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

const userChatPersonalModelOverridesKey = [
	...chatsKey,
	"user-personal-model-overrides",
] as const;

export const userChatPersonalModelOverrides = () => ({
	queryKey: userChatPersonalModelOverridesKey,
	queryFn: (): Promise<TypesGen.UserChatPersonalModelOverridesResponse> =>
		API.experimental.getUserChatPersonalModelOverrides(),
});

type UpdateUserChatPersonalModelOverrideArgs = {
	context: TypesGen.ChatPersonalModelOverrideContext;
	req: TypesGen.UpdateUserChatPersonalModelOverrideRequest;
};

export const updateUserChatPersonalModelOverride = (
	queryClient: QueryClient,
) => ({
	mutationFn: ({ context, req }: UpdateUserChatPersonalModelOverrideArgs) =>
		API.experimental.updateUserChatPersonalModelOverride(context, req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: userChatPersonalModelOverridesKey,
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

export const chatCostSummaryKey = (user = "me", params?: ChatCostDateParams) =>
	[...chatsKey, "costSummary", user, params] as const;

export const chatCostSummary = (user = "me", params?: ChatCostDateParams) => ({
	queryKey: chatCostSummaryKey(user, params),
	queryFn: () => API.experimental.getChatCostSummary(user, params),
	staleTime: 60_000,
});

interface PaginatedChatCostUsersPayload {
	username: string;
	start_date: string;
	end_date: string;
}

export function paginatedChatCostUsers(
	payload: PaginatedChatCostUsersPayload,
): UsePaginatedQueryOptions<
	TypesGen.ChatCostUsersResponse,
	PaginatedChatCostUsersPayload
> {
	return {
		queryPayload: () => payload,
		queryKey: ({ payload, pageNumber }) =>
			[...chatsKey, "costUsers", payload, pageNumber] as const,
		queryFn: ({ payload, limit, offset }) =>
			API.experimental.getChatCostUsers({
				start_date: payload.start_date,
				end_date: payload.end_date,
				username: payload.username || undefined,
				limit,
				offset,
			}),
		staleTime: 60_000,
	};
}

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
