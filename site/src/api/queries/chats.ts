import {
	type InfiniteData,
	infiniteQueryOptions,
	type QueryClient,
	queryOptions,
} from "react-query";
import {
	API,
	type ChatPlanModeOrClear,
	type CreateChatMessageRequestWithClearablePlanMode,
} from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import type { UsePaginatedQueryOptions } from "#/hooks/usePaginatedQuery";
import { preserveChatExecutionOverlay } from "./chatExecutionOverlay";
import {
	projectEditedConversationIntoCache,
	reconcileEditedMessageInCache,
} from "./chatMessageEdits";

/** @public */
export type ChatListProjection = TypesGen.Chat;
/** @public */
export type ChatDetailProjection = TypesGen.Chat & {
	readonly action_required?: TypesGen.ChatStreamActionRequired;
};
/** @public */
export type ChatWatchProjection = TypesGen.ChatWatchEvent;
/** @public */
export type ChatExecutionSnapshotEvent = TypesGen.ChatStreamEvent;

/** Applies durable execution events to the canonical exact-detail projection. */
export const applyExecutionSnapshotEvent = (
	chat: ChatDetailProjection,
	event: ChatExecutionSnapshotEvent,
): ChatDetailProjection => {
	switch (event.type) {
		case "action_required":
			if (!event.action_required) {
				return chat;
			}
			return {
				...chat,
				status: "requires_action",
				last_error: undefined,
				action_required: event.action_required,
			};
		case "error":
			if (!event.error) {
				return chat;
			}
			return {
				...chat,
				status: "error",
				last_error: event.error,
				action_required: undefined,
			};
		case "status": {
			const status = event.status?.status;
			if (!status) {
				return chat;
			}
			return {
				...chat,
				status,
				last_error: status === "error" ? chat.last_error : undefined,
				action_required:
					status === "requires_action" ? chat.action_required : undefined,
			};
		}
		default:
			return chat;
	}
};

/** @public */
export type ChatMessagesProjection = Readonly<{
	messages: readonly TypesGen.ChatMessage[];
	queuedMessages: readonly TypesGen.ChatQueuedMessage[];
}>;

const compareChatMessagesChronologically = (
	left: TypesGen.ChatMessage,
	right: TypesGen.ChatMessage,
): number => {
	const createdAtDifference =
		Date.parse(left.created_at) - Date.parse(right.created_at);
	if (Number.isFinite(createdAtDifference) && createdAtDifference !== 0) {
		return createdAtDifference;
	}
	return left.id - right.id;
};

/**
 * Projects the canonical infinite messages cache into the chronological,
 * deduplicated transcript consumed by the UI. Newer pages take precedence
 * when a message temporarily appears in more than one page.
 */
export const selectChatMessagesProjection = (
	data: InfiniteData<TypesGen.ChatMessagesResponse>,
): ChatMessagesProjection => {
	const messagesByID = new Map<number, TypesGen.ChatMessage>();
	for (const page of data.pages) {
		for (const message of page.messages) {
			if (!messagesByID.has(message.id)) {
				messagesByID.set(message.id, message);
			}
		}
	}
	const messages = Array.from(messagesByID.values());
	messages.sort(compareChatMessagesChronologically);
	return {
		messages,
		queuedMessages: data.pages[0]?.queued_messages ?? [],
	};
};

export type ChatListPRStatusFilter = "draft" | "open" | "merged" | "closed";
export type ChatListStatusFilter = "read" | "unread";

/** @public */
export type InfiniteChatsFilters = Readonly<{
	archived?: boolean;
	prStatuses?: readonly ChatListPRStatusFilter[];
	chatStatus?: ChatListStatusFilter;
	sources?: readonly TypesGen.ChatListSource[];
}>;

const CHAT_LIST_SOURCE_ORDER = [
	"created_by_me",
	"shared_with_me",
] as const satisfies readonly TypesGen.ChatListSource[];

/** @public */
export const normalizeInfiniteChatsFilters = (
	filters?: InfiniteChatsFilters,
): InfiniteChatsFilters => ({
	archived: filters?.archived,
	prStatuses: canonicalizeChatListPRStatuses(filters?.prStatuses ?? []),
	chatStatus: filters?.chatStatus,
	sources: CHAT_LIST_SOURCE_ORDER.filter((source) =>
		filters?.sources?.includes(source),
	),
});

export const chatQueryKeys = {
	all: ["chats"] as const,
	lists: () => ["chats", "list"] as const,
	list: (filters?: InfiniteChatsFilters) =>
		["chats", "list", normalizeInfiniteChatsFilters(filters)] as const,
	searches: () => ["chats", "search"] as const,
	search: (q: string) => ["chats", "search", { q }] as const,
	byWorkspace: () => ["chats", "by-workspace"] as const,
	byWorkspaceIDs: (workspaceIds: readonly string[]) =>
		["chats", "by-workspace", workspaceIds.toSorted()] as const,
	details: () => ["chats", "detail"] as const,
	detail: (chatId: string) => ["chats", "detail", chatId] as const,
	messages: (chatId: string) =>
		["chats", "detail", chatId, "messages"] as const,
	prompts: (chatId: string) => ["chats", "detail", chatId, "prompts"] as const,
	acl: (chatId: string) => ["chats", "detail", chatId, "acl"] as const,
	diffContents: (chatId: string) =>
		["chats", "detail", chatId, "diff-contents"] as const,
	debugRuns: (chatId: string) =>
		["chats", "detail", chatId, "debug-runs"] as const,
	debugRun: (chatId: string, runId: string) =>
		["chats", "detail", chatId, "debug-runs", runId] as const,
};

const CHAT_FOREGROUND_POLL_MS = 15_000;
const CHAT_METADATA_STALE_MS = CHAT_FOREGROUND_POLL_MS;
const CHAT_AUXILIARY_STALE_MS = 30_000;
const CHAT_QUERY_GC_MS = 5 * 60_000;
const CHAT_QUERY_RETRY_COUNT = 3;

const isExactChatDetailProjectionKey = (
	queryKey: readonly unknown[],
): boolean =>
	queryKey.length === 3 &&
	queryKey[0] === "chats" &&
	queryKey[1] === "detail" &&
	typeof queryKey[2] === "string";

const refetchActiveChatCollectionProjections = (queryClient: QueryClient) =>
	Promise.all([
		queryClient.refetchQueries(
			{
				queryKey: chatQueryKeys.lists(),
				type: "active",
			},
			{ throwOnError: true },
		),
		queryClient.refetchQueries(
			{
				queryKey: chatQueryKeys.searches(),
				type: "active",
			},
			{ throwOnError: true },
		),
		queryClient.refetchQueries(
			{
				queryKey: chatQueryKeys.byWorkspace(),
				type: "active",
			},
			{ throwOnError: true },
		),
	]);

/** Refetches the active REST metadata projections used as the global-watch baseline. */
export const refetchActiveChatMetadataProjections = async (
	queryClient: QueryClient,
): Promise<void> => {
	await Promise.all([
		refetchActiveChatCollectionProjections(queryClient),
		queryClient.refetchQueries(
			{
				queryKey: chatQueryKeys.details(),
				type: "active",
				predicate: (query) => isExactChatDetailProjectionKey(query.queryKey),
			},
			{ throwOnError: true },
		),
	]);
};

/** Invalidates every chat collection projection. */
export const invalidateChatCollectionQueries = async (
	queryClient: QueryClient,
): Promise<void> => {
	await Promise.all([
		queryClient.invalidateQueries({ queryKey: chatQueryKeys.lists() }),
		queryClient.invalidateQueries({ queryKey: chatQueryKeys.searches() }),
		queryClient.invalidateQueries({ queryKey: chatQueryKeys.byWorkspace() }),
	]);
};

/** Repairs exact metadata and every collection projection after mutation settlement. */
export const repairChatMetadataAfterMutation = async (
	queryClient: QueryClient,
	chatId: string,
	options: { acl?: boolean } = {},
): Promise<void> => {
	await Promise.all([
		queryClient.invalidateQueries({
			queryKey: chatQueryKeys.detail(chatId),
			exact: true,
		}),
		invalidateChatCollectionQueries(queryClient),
		...(options.acl
			? [
					queryClient.invalidateQueries({
						queryKey: chatQueryKeys.acl(chatId),
						exact: true,
					}),
				]
			: []),
	]);
};

/** Refetches a parent detail even when it was not previously loaded. */
export const repairParentChatProjection = async (
	queryClient: QueryClient,
	parentChatId: string,
): Promise<void> => {
	await queryClient.invalidateQueries({
		queryKey: chatQueryKeys.detail(parentChatId),
		exact: true,
	});
	await queryClient.fetchQuery({
		...chat(parentChatId),
		staleTime: 0,
	});
};

/**
 * Closes races with sparse hints received while the REST baseline was loading.
 * Collection membership is rechecked globally, while exact detail metadata is
 * invalidated only for chats observed during the baseline.
 */
export const refetchDirtyChatMetadataProjections = async (
	queryClient: QueryClient,
	chatIDs: ReadonlySet<string>,
): Promise<void> => {
	await Promise.all([
		refetchActiveChatCollectionProjections(queryClient),
		...Array.from(chatIDs, (chatID) =>
			queryClient.invalidateQueries(
				{
					queryKey: chatQueryKeys.detail(chatID),
					exact: true,
				},
				{ throwOnError: true },
			),
		),
	]);
};

/** Repairs every cache projection that can be affected by a malformed stream. */
export const invalidateChatAfterExecutionStreamFailure = async (
	queryClient: QueryClient,
	chatID: string,
): Promise<void> => {
	await Promise.all([
		queryClient.invalidateQueries({
			queryKey: chatQueryKeys.detail(chatID),
		}),
		queryClient.invalidateQueries({
			queryKey: chatQueryKeys.messages(chatID),
			exact: true,
		}),
		queryClient.invalidateQueries({
			queryKey: chatQueryKeys.prompts(chatID),
			exact: true,
		}),
		queryClient.invalidateQueries({ queryKey: chatQueryKeys.lists() }),
		queryClient.invalidateQueries({ queryKey: chatQueryKeys.searches() }),
		queryClient.invalidateQueries({ queryKey: chatQueryKeys.byWorkspace() }),
	]);
};

export const getCachedChat = (queryClient: QueryClient, chatId: string) =>
	queryClient.getQueryData<ChatDetailProjection>(chat(chatId).queryKey);

export const updateCachedChat = (
	queryClient: QueryClient,
	chatId: string,
	updater: (
		chat: ChatDetailProjection | undefined,
	) => ChatDetailProjection | undefined,
) => queryClient.setQueryData(chat(chatId).queryKey, updater);

/** Writes a durable execution snapshot event into the exact-detail cache. */
export const updateCachedChatExecutionSnapshot = (
	queryClient: QueryClient,
	chatId: string,
	event: ChatExecutionSnapshotEvent,
) =>
	updateCachedChat(queryClient, chatId, (current) =>
		current ? applyExecutionSnapshotEvent(current, event) : current,
	);

export const cancelCachedChat = (queryClient: QueryClient, chatId: string) =>
	queryClient.cancelQueries({ queryKey: chat(chatId).queryKey, exact: true });

export const invalidateCachedChat = (
	queryClient: QueryClient,
	chatId: string,
) =>
	queryClient.invalidateQueries({
		queryKey: chat(chatId).queryKey,
		exact: true,
	});

const removeCachedChatQueryFamily = (
	queryClient: QueryClient,
	chatId: string,
): void => {
	queryClient.removeQueries({ queryKey: chatQueryKeys.detail(chatId) });
};

const collectLoadedChatDetails = (
	queryClient: QueryClient,
): readonly ChatDetailProjection[] =>
	queryClient
		.getQueriesData<ChatDetailProjection>({ queryKey: chatQueryKeys.details() })
		.filter(([queryKey]) => isExactChatDetailProjectionKey(queryKey))
		.flatMap(([, data]) => (data ? [data] : []));

const collectEmbeddedChats = (
	chat: TypesGen.Chat,
): readonly TypesGen.Chat[] => [
	chat,
	...(chat.children ?? []).flatMap(collectEmbeddedChats),
];

const collectLoadedCollectionChats = (
	queryClient: QueryClient,
): readonly TypesGen.Chat[] => {
	const listChats = queryClient
		.getQueriesData<InfiniteChatsCacheData>({ queryKey: chatQueryKeys.lists() })
		.flatMap(([, data]) => data?.pages.flat() ?? []);
	const searchChats = queryClient
		.getQueriesData<readonly TypesGen.Chat[]>({
			queryKey: chatQueryKeys.searches(),
		})
		.flatMap(([, data]) => data ?? []);
	return [...listChats, ...searchChats].flatMap(collectEmbeddedChats);
};

/** Builds the explicit previous-status source used by the owner watch coordinator. */
export const readLoadedChatStatusMap = (
	queryClient: QueryClient,
): Map<string, TypesGen.ChatStatus> => {
	const latestChats = new Map<string, TypesGen.Chat>();
	for (const chat of collectLoadedCollectionChats(queryClient)) {
		const previous = latestChats.get(chat.id);
		if (!previous || previous.updated_at.localeCompare(chat.updated_at) <= 0) {
			latestChats.set(chat.id, chat);
		}
	}
	return new Map(
		Array.from(latestChats, ([chatID, chat]) => [chatID, chat.status]),
	);
};

const removeChatIDsFromWorkspaceCaches = (
	queryClient: QueryClient,
	chatIDs: ReadonlySet<string>,
): void => {
	queryClient.setQueriesData<Record<string, string>>(
		{ queryKey: chatQueryKeys.byWorkspace() },
		(previous) => {
			if (!previous) {
				return previous;
			}
			const next = Object.fromEntries(
				Object.entries(previous).filter(([, chatID]) => !chatIDs.has(chatID)),
			);
			return Object.keys(next).length === Object.keys(previous).length
				? previous
				: next;
		},
	);
};

const removeChatIDsFromCollections = (
	queryClient: QueryClient,
	chatIDs: ReadonlySet<string>,
): void => {
	updateChatCollectionCaches(queryClient, (chats) => {
		let changed = false;
		const nextChats: TypesGen.Chat[] = [];
		for (const candidate of chats) {
			if (chatIDs.has(candidate.id)) {
				changed = true;
				continue;
			}
			const children = candidate.children?.filter(
				(child) => !chatIDs.has(child.id),
			);
			if (children && children.length !== candidate.children?.length) {
				changed = true;
				nextChats.push({ ...candidate, children });
			} else {
				nextChats.push(candidate);
			}
		}
		return changed ? nextChats : chats;
	});
	removeChatIDsFromWorkspaceCaches(queryClient, chatIDs);
};

const invalidateChatWorkspaceQueries = (queryClient: QueryClient): void => {
	void queryClient.invalidateQueries({ queryKey: chatQueryKeys.byWorkspace() });
};

/** Removes a deleted chat subtree from every loaded projection and query family. */
export const removeDeletedChatFamily = (
	queryClient: QueryClient,
	deletedChat: TypesGen.Chat,
): void => {
	const loadedChats = [
		...collectLoadedChatDetails(queryClient),
		...collectLoadedCollectionChats(queryClient),
	];
	const chatIDs = new Set<string>([deletedChat.id]);
	if (!deletedChat.parent_chat_id) {
		for (const chat of loadedChats) {
			if (chat.id === deletedChat.id || chat.root_chat_id === deletedChat.id) {
				chatIDs.add(chat.id);
			}
		}
	} else {
		let foundDescendant = true;
		while (foundDescendant) {
			foundDescendant = false;
			for (const chat of loadedChats) {
				if (
					chat.parent_chat_id &&
					chatIDs.has(chat.parent_chat_id) &&
					!chatIDs.has(chat.id)
				) {
					chatIDs.add(chat.id);
					foundDescendant = true;
				}
			}
		}
	}
	for (const chatID of chatIDs) {
		removeCachedChatQueryFamily(queryClient, chatID);
	}
	removeChatIDsFromCollections(queryClient, chatIDs);
	// By-workspace projections contain only workspace-to-chat IDs, so they do
	// not provide enough ancestry to discover uncached descendants locally.
	invalidateChatWorkspaceQueries(queryClient);
};

/** Evicts a root chat family after the viewer loses access. */
export const evictChatFamilyAfterAccessLoss = (
	queryClient: QueryClient,
	chatId: string,
): void => {
	const cachedChat = getCachedChat(queryClient, chatId);
	const rootChatID = cachedChat?.root_chat_id ?? chatId;
	const chatIDs = new Set<string>([rootChatID, chatId]);
	for (const chat of [
		...collectLoadedChatDetails(queryClient),
		...collectLoadedCollectionChats(queryClient),
	]) {
		if (chat.id === rootChatID || chat.root_chat_id === rootChatID) {
			chatIDs.add(chat.id);
		}
	}
	for (const familyChatID of chatIDs) {
		removeCachedChatQueryFamily(queryClient, familyChatID);
	}
	removeChatIDsFromCollections(queryClient, chatIDs);
	// Access loss can hide descendants that are known only by workspace ID.
	invalidateChatWorkspaceQueries(queryClient);
};

const jsonValuesEqual = (left: unknown, right: unknown): boolean => {
	if (left === right) {
		return true;
	}
	try {
		return JSON.stringify(left) === JSON.stringify(right);
	} catch {
		return false;
	}
};

const chatMessagesEqualByValue = (
	left: TypesGen.ChatMessage,
	right: TypesGen.ChatMessage,
): boolean =>
	left.id === right.id &&
	left.chat_id === right.chat_id &&
	left.model_config_id === right.model_config_id &&
	left.created_at === right.created_at &&
	left.role === right.role &&
	jsonValuesEqual(left.content, right.content) &&
	jsonValuesEqual(left.usage, right.usage);

const getCachedChatMessages = (queryClient: QueryClient, chatId: string) =>
	queryClient.getQueryData<InfiniteData<TypesGen.ChatMessagesResponse>>(
		chatMessagesForInfiniteScroll(chatId).queryKey,
	);

const updateCachedChatMessages = (
	queryClient: QueryClient,
	chatId: string,
	updater: (
		data: InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
	) => InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
) =>
	queryClient.setQueryData(
		chatMessagesForInfiniteScroll(chatId).queryKey,
		updater,
	);

const cancelCachedChatMessages = (queryClient: QueryClient, chatId: string) =>
	queryClient.cancelQueries({
		queryKey: chatMessagesForInfiniteScroll(chatId).queryKey,
		exact: true,
	});

const updateFirstChatMessagesPage = (
	currentData: InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
	updater: (
		page: TypesGen.ChatMessagesResponse,
	) => TypesGen.ChatMessagesResponse,
): InfiniteData<TypesGen.ChatMessagesResponse> | undefined => {
	if (!currentData?.pages.length) {
		return currentData;
	}
	const firstPage = currentData.pages[0];
	const updatedFirstPage = updater(firstPage);
	if (updatedFirstPage === firstPage) {
		return currentData;
	}
	return {
		...currentData,
		pages: [updatedFirstPage, ...currentData.pages.slice(1)],
	};
};

const queuedMessagesEqualByValue = (
	left: readonly TypesGen.ChatQueuedMessage[],
	right: readonly TypesGen.ChatQueuedMessage[],
): boolean => jsonValuesEqual(left, right);

const replaceQueuedMessagesInData = (
	currentData: InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
	queuedMessages: readonly TypesGen.ChatQueuedMessage[],
) =>
	updateFirstChatMessagesPage(currentData, (firstPage) =>
		queuedMessagesEqualByValue(firstPage.queued_messages, queuedMessages)
			? firstPage
			: { ...firstPage, queued_messages: queuedMessages },
	);

const removeQueuedMessageFromData = (
	currentData: InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
	queuedMessageID: number,
) =>
	updateFirstChatMessagesPage(currentData, (firstPage) => {
		const queuedMessages = firstPage.queued_messages.filter(
			(message) => message.id !== queuedMessageID,
		);
		return queuedMessages.length === firstPage.queued_messages.length
			? firstPage
			: { ...firstPage, queued_messages: queuedMessages };
	});

const upsertQueuedMessageInData = (
	currentData: InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
	queuedMessage: TypesGen.ChatQueuedMessage,
) =>
	updateFirstChatMessagesPage(currentData, (firstPage) => {
		const index = firstPage.queued_messages.findIndex(
			(message) => message.id === queuedMessage.id,
		);
		if (index === -1) {
			return {
				...firstPage,
				queued_messages: [...firstPage.queued_messages, queuedMessage],
			};
		}
		if (jsonValuesEqual(firstPage.queued_messages[index], queuedMessage)) {
			return firstPage;
		}
		const queuedMessages = [...firstPage.queued_messages];
		queuedMessages[index] = queuedMessage;
		return { ...firstPage, queued_messages: queuedMessages };
	});

/** Replaces the complete queue snapshot projected by the per-chat stream. */
export const replaceCachedChatQueuedMessages = (
	queryClient: QueryClient,
	chatId: string,
	queuedMessages: readonly TypesGen.ChatQueuedMessage[],
): void => {
	if (
		queryClient.getQueryState(chatMessagesForInfiniteScroll(chatId).queryKey)
			?.fetchStatus === "fetching"
	) {
		void cancelCachedChatMessages(queryClient, chatId);
	}
	updateCachedChatMessages(queryClient, chatId, (currentData) =>
		replaceQueuedMessagesInData(currentData, queuedMessages),
	);
};

const upsertChatMessagesInData = (
	currentData: InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
	messages: readonly TypesGen.ChatMessage[],
) =>
	updateFirstChatMessagesPage(currentData, (firstPage) => {
		const existingByID = new Map(
			firstPage.messages.map((message) => [message.id, message]),
		);
		let changed = false;
		for (const message of messages) {
			const existing = existingByID.get(message.id);
			if (!existing || !chatMessagesEqualByValue(existing, message)) {
				existingByID.set(message.id, message);
				changed = true;
			}
		}
		if (!changed) {
			return firstPage;
		}
		const updatedMessages = Array.from(existingByID.values());
		updatedMessages.sort((left, right) => right.id - left.id);
		return { ...firstPage, messages: updatedMessages };
	});

/**
 * Applies committed stream messages to the newest page. An active fetch is
 * canceled first so its older snapshot cannot overwrite this update.
 */
export const upsertCachedChatMessages = (
	queryClient: QueryClient,
	chatId: string,
	messages: readonly TypesGen.ChatMessage[],
): void => {
	if (messages.length === 0) {
		return;
	}
	if (
		queryClient.getQueryState(chatMessagesForInfiniteScroll(chatId).queryKey)
			?.fetchStatus === "fetching"
	) {
		void cancelCachedChatMessages(queryClient, chatId);
	}
	updateCachedChatMessages(queryClient, chatId, (currentData) =>
		upsertChatMessagesInData(currentData, messages),
	);
};

/** Replaces the complete committed history after an authoritative reset. */
export const replaceCachedChatMessages = (
	queryClient: QueryClient,
	chatId: string,
	messages: readonly TypesGen.ChatMessage[],
): void => {
	void cancelCachedChatMessages(queryClient, chatId);
	updateCachedChatMessages(queryClient, chatId, (currentData) => {
		if (!currentData?.pages.length) {
			return currentData;
		}
		const firstPage = currentData.pages[0];
		const updatedMessages = [...messages].sort(
			(left, right) => right.id - left.id,
		);
		return {
			...currentData,
			pages: [{ ...firstPage, messages: updatedMessages, has_more: false }],
			pageParams: [undefined],
		};
	});
};

const invalidateCachedChatMessages = (
	queryClient: QueryClient,
	chatId: string,
) =>
	queryClient.invalidateQueries({
		queryKey: chatMessagesForInfiniteScroll(chatId).queryKey,
		exact: true,
	});

export const invalidateCachedChatPrompts = (
	queryClient: QueryClient,
	chatId: string,
) =>
	queryClient.invalidateQueries({
		queryKey: chatPromptsQuery(chatId).queryKey,
		exact: true,
	});

export const invalidateCachedChatDiff = (
	queryClient: QueryClient,
	chatId: string,
) =>
	queryClient.invalidateQueries({
		queryKey: chatDiffContents(chatId).queryKey,
		exact: true,
	});

export const CHAT_LIST_PR_STATUS_ORDER = [
	"draft",
	"open",
	"merged",
	"closed",
] as const satisfies readonly ChatListPRStatusFilter[];

const chatListPRStatusSet = new Set<ChatListPRStatusFilter>(
	CHAT_LIST_PR_STATUS_ORDER,
);

type InfiniteChatsCacheData = InfiniteData<TypesGen.Chat[]>;

/** Shared ordering keeps URL serialization stable. */
export const canonicalizeChatListPRStatuses = (
	prStatuses: Iterable<unknown>,
): readonly ChatListPRStatusFilter[] => {
	const selected = new Set<ChatListPRStatusFilter>();
	for (const prStatus of prStatuses) {
		if (
			typeof prStatus === "string" &&
			chatListPRStatusSet.has(prStatus as ChatListPRStatusFilter)
		) {
			selected.add(prStatus as ChatListPRStatusFilter);
		}
	}

	return CHAT_LIST_PR_STATUS_ORDER.filter((status) => selected.has(status));
};

export const chatsByWorkspace = (workspaceIds: string[]) => {
	const sorted = workspaceIds.toSorted();
	return queryOptions({
		queryKey: chatQueryKeys.byWorkspaceIDs(sorted),
		queryFn: () => API.experimental.getChatsByWorkspace(sorted),
		enabled: workspaceIds.length > 0,
		staleTime: CHAT_METADATA_STALE_MS,
		gcTime: CHAT_QUERY_GC_MS,
		retry: CHAT_QUERY_RETRY_COUNT,
		refetchInterval: CHAT_FOREGROUND_POLL_MS,
		refetchIntervalInBackground: false,
		refetchOnMount: true,
		refetchOnWindowFocus: true,
		refetchOnReconnect: true,
	});
};

/**
 * Updates a single chat inside every page of the infinite chats query
 * cache. Use this instead of setQueryData(chatQueryKeys.all, ...) which writes
 * to the wrong key (the flat list key, not the infinite query key).
 */
export const updateInfiniteChatsCache = (
	queryClient: QueryClient,
	updater: (chats: TypesGen.Chat[]) => TypesGen.Chat[],
) => {
	// Update ALL infinite chat queries regardless of their filter opts.
	queryClient.setQueriesData<InfiniteChatsCacheData>(
		{ queryKey: chatQueryKeys.lists() },
		(prev) => {
			if (!prev?.pages) return prev;
			const nextPages = prev.pages.map((page) => updater(page));
			// Only return a new reference if something actually changed.
			const changed = nextPages.some((page, i) => page !== prev.pages[i]);
			return changed ? { ...prev, pages: nextPages } : prev;
		},
	);
};

type ChatCollectionUpdater = (chats: TypesGen.Chat[]) => TypesGen.Chat[];

const updateChatSearchCaches = (
	queryClient: QueryClient,
	updater: ChatCollectionUpdater,
): void => {
	queryClient.setQueriesData<readonly TypesGen.Chat[]>(
		{ queryKey: chatQueryKeys.searches() },
		(previous) => {
			if (!previous) {
				return previous;
			}
			return updater([...previous]);
		},
	);
};

/** Updates every loaded chat collection projection. */
const updateChatCollectionCaches = (
	queryClient: QueryClient,
	updater: ChatCollectionUpdater,
): void => {
	updateInfiniteChatsCache(queryClient, updater);
	updateChatSearchCaches(queryClient, updater);
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
	queryClient.setQueriesData<InfiniteChatsCacheData>(
		{ queryKey: chatQueryKeys.lists() },
		(prev) => {
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
		},
	);
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
	updateChatCollectionCaches(queryClient, (chats) => {
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
	updateChatCollectionCaches(queryClient, (chats) => {
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
	updateChatCollectionCaches(queryClient, (chats) => {
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

// The normalized filter object follows the explicit chat-list namespace.
const archivedFilterForChatListKey = (
	queryKey: readonly unknown[],
): boolean | undefined => {
	const listPrefix = chatQueryKeys.lists();
	if (queryKey.length !== listPrefix.length + 1) {
		return undefined;
	}
	const filters = queryKey[listPrefix.length];
	if (!filters || typeof filters !== "object") {
		return undefined;
	}
	const archived = (filters as { archived?: unknown }).archived;
	return typeof archived === "boolean" ? archived : undefined;
};

const isInfiniteChatsCacheData = (
	data: unknown,
): data is InfiniteChatsCacheData => {
	if (!data || typeof data !== "object") {
		return false;
	}
	const maybeData = data as { pages?: unknown; pageParams?: unknown };
	return Array.isArray(maybeData.pages) && Array.isArray(maybeData.pageParams);
};

const patchChatArchiveState = (
	chat: TypesGen.Chat,
	archived: boolean,
): TypesGen.Chat => {
	const pinOrder = archived ? 0 : chat.pin_order;
	if (chat.archived === archived && chat.pin_order === pinOrder) {
		return chat;
	}
	return { ...chat, archived, pin_order: pinOrder };
};

const collectLoadedChatFamilyIDs = (
	queryClient: QueryClient,
	rootChatID: string,
): ReadonlySet<string> => {
	const chatIDs = new Set<string>([rootChatID]);
	for (const loadedChat of [
		...collectLoadedChatDetails(queryClient),
		...collectLoadedCollectionChats(queryClient),
	]) {
		if (
			loadedChat.id === rootChatID ||
			loadedChat.root_chat_id === rootChatID
		) {
			chatIDs.add(loadedChat.id);
		}
	}
	return chatIDs;
};

const patchChatFamilyArchiveState = (
	chat: TypesGen.Chat,
	chatIDs: ReadonlySet<string>,
	archived: boolean,
): TypesGen.Chat => {
	const children = chat.children?.map((child) =>
		patchChatFamilyArchiveState(child, chatIDs, archived),
	);
	const patchedChat = chatIDs.has(chat.id)
		? patchChatArchiveState(chat, archived)
		: chat;
	if (
		children === undefined ||
		children.every((child, index) => child === patchedChat.children?.[index])
	) {
		return patchedChat;
	}
	return { ...patchedChat, children };
};

const repairChatFamilyMetadataAfterMutation = async (
	queryClient: QueryClient,
	rootChatID: string,
): Promise<void> => {
	const chatIDs = collectLoadedChatFamilyIDs(queryClient, rootChatID);
	await Promise.all([
		...Array.from(chatIDs, (chatID) =>
			queryClient.invalidateQueries({
				queryKey: chatQueryKeys.detail(chatID),
				exact: true,
			}),
		),
		invalidateChatCollectionQueries(queryClient),
	]);
};

/**
 * Applies an accepted archive state to loaded sidebar and detail caches.
 * Removes the chat from any filtered list whose archived filter conflicts
 * with the new state, and resets pin_order to 0 when archiving.
 */
export const applyChatArchiveStateToCaches = (
	queryClient: QueryClient,
	chatId: string,
	archived: boolean,
) => {
	const chatIDs = collectLoadedChatFamilyIDs(queryClient, chatId);
	for (const familyChatID of chatIDs) {
		updateCachedChat(queryClient, familyChatID, (chat) =>
			chat ? patchChatArchiveState(chat, archived) : chat,
		);
	}

	updateChatSearchCaches(queryClient, (chats) => {
		if (archived) {
			const nextChats = chats.filter((chat) => !chatIDs.has(chat.id));
			return nextChats.length === chats.length ? chats : nextChats;
		}
		let changed = false;
		const nextChats = chats.map((chat) => {
			const updatedChat = patchChatFamilyArchiveState(chat, chatIDs, false);
			changed ||= updatedChat !== chat;
			return updatedChat;
		});
		return changed ? nextChats : chats;
	});

	if (archived) {
		removeChatIDsFromWorkspaceCaches(queryClient, chatIDs);
	}

	const queries = queryClient.getQueriesData<InfiniteChatsCacheData>({
		queryKey: chatQueryKeys.lists(),
	});

	for (const [queryKey, data] of queries) {
		if (!isInfiniteChatsCacheData(data)) {
			continue;
		}
		const archivedFilter = archivedFilterForChatListKey(queryKey);
		queryClient.setQueryData<InfiniteChatsCacheData>(queryKey, (prev) => {
			if (!isInfiniteChatsCacheData(prev)) {
				return prev;
			}

			let changed = false;
			const pages = prev.pages.map((page) => {
				let pageChanged = false;
				const nextPage: TypesGen.Chat[] = [];
				for (const chat of page) {
					if (!chatIDs.has(chat.id)) {
						nextPage.push(chat);
						continue;
					}

					if (archivedFilter !== undefined && archivedFilter !== archived) {
						pageChanged = true;
						continue;
					}

					const updatedChat = patchChatFamilyArchiveState(
						chat,
						chatIDs,
						archived,
					);
					if (updatedChat !== chat) {
						pageChanged = true;
					}
					nextPage.push(updatedChat);
				}
				if (pageChanged) {
					changed = true;
					return nextPage;
				}
				return page;
			});

			return changed ? { ...prev, pages } : prev;
		});
	}
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
	{ eventKind }: MergeWatchedChatOptions,
): TypesGen.Chat => {
	const isTitleEvent = eventKind === "title_change";
	const isStatusEvent =
		eventKind === "status_change" || eventKind === "action_required";
	const isSummaryEvent = eventKind === "summary_change";
	const isDiffStatusEvent = eventKind === "diff_status_change";
	const isContextDirtyEvent = eventKind === "context_dirty";
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
	// Context drift is tracked outside chats.updated_at (it is driven by
	// agent context pushes), so apply context_dirty payloads regardless of
	// the summary timestamp. Merge rather than replace so the pinned
	// resources a single-chat GET populated are preserved while the dirty
	// flags update; the open chat refetches the full detail.
	const nextContext =
		isContextDirtyEvent && watchedChat.context
			? { ...cachedChat.context, ...watchedChat.context }
			: cachedChat.context;
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
	const nextLastTurnSummary =
		isFreshEnough || isSummaryEvent
			? watchedChat.last_turn_summary
			: cachedChat.last_turn_summary;
	const nextHasUnread = cachedChat.has_unread;
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
		nextLastTurnSummary === cachedChat.last_turn_summary &&
		nextHasUnread === cachedChat.has_unread &&
		nextUpdatedAt === cachedChat.updated_at &&
		nextContext === cachedChat.context
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
		last_turn_summary: nextLastTurnSummary,
		has_unread: nextHasUnread,
		updated_at: nextUpdatedAt,
		context: nextContext,
	};
};

/** Applies event-scoped projection hints to list and embedded-child caches. */
export const mergeWatchedChatIntoCaches = (
	queryClient: QueryClient,
	watchedChat: TypesGen.Chat,
	options: MergeWatchedChatOptions,
) => {
	const mergeCachedChat = (cachedChat: TypesGen.Chat) =>
		mergeWatchedChatSummary(cachedChat, watchedChat, options);

	updateChatCollectionCaches(queryClient, (chats) => {
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
};

/**
 * Applies only metadata-bearing global hints to exact detail. Execution status
 * remains owned by the active per-chat snapshot stream after bootstrap.
 */
export const mergeWatchedChatMetadataIntoDetail = (
	queryClient: QueryClient,
	watchedChat: TypesGen.Chat,
	options: MergeWatchedChatOptions,
) => {
	if (
		options.eventKind === "status_change" ||
		options.eventKind === "action_required"
	) {
		return;
	}
	updateCachedChat(queryClient, watchedChat.id, (cachedChat) => {
		if (!cachedChat) {
			return cachedChat;
		}
		const merged = mergeWatchedChatSummary(cachedChat, watchedChat, options);
		return merged.status === cachedChat.status
			? merged
			: { ...merged, status: cachedChat.status };
	});
};

export const invalidateChatListQueries = (queryClient: QueryClient) => {
	return queryClient.invalidateQueries({
		queryKey: chatQueryKeys.lists(),
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
	if (
		query.queryKey[0] !== chatQueryKeys.lists()[0] ||
		query.queryKey[1] !== chatQueryKeys.lists()[1]
	) {
		return false;
	}
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
		queryKey: chatQueryKeys.lists(),
		predicate: isChatListRefetch,
	});
};

const DEFAULT_CHAT_PAGE_LIMIT = 50;
export const CHAT_SEARCH_LIMIT = 50;

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

const getInfiniteChatsQueryString = (
	filters: InfiniteChatsFilters | undefined,
): string | undefined => {
	const qParts: string[] = [];
	if (filters?.archived !== undefined) {
		qParts.push(`archived:${filters.archived}`);
	}
	if (filters?.prStatuses?.length) {
		qParts.push(`pr_status:${filters.prStatuses.join(",")}`);
	}
	if (filters?.chatStatus) {
		qParts.push(`has_unread:${filters.chatStatus === "unread"}`);
	}
	if (filters?.sources?.length) {
		qParts.push(`source:${filters.sources.join(",")}`);
	}
	return qParts.length > 0 ? qParts.join(" ") : undefined;
};

export const infiniteChats = (filters?: InfiniteChatsFilters) => {
	const normalizedFilters = normalizeInfiniteChatsFilters(filters);
	const limit = DEFAULT_CHAT_PAGE_LIMIT;
	const q = getInfiniteChatsQueryString(normalizedFilters);

	return infiniteQueryOptions({
		queryKey: chatQueryKeys.list(normalizedFilters),
		getNextPageParam: (lastPage: TypesGen.Chat[]) => {
			if (lastPage.length < limit) {
				return undefined;
			}
			return lastPage.at(-1)?.id;
		},
		initialPageParam: undefined as string | undefined,
		queryFn: ({ pageParam }) =>
			API.experimental.getChats({
				after_id: pageParam,
				limit,
				q,
			}),
		staleTime: CHAT_METADATA_STALE_MS,
		gcTime: CHAT_QUERY_GC_MS,
		retry: CHAT_QUERY_RETRY_COUNT,
		refetchInterval: CHAT_FOREGROUND_POLL_MS,
		refetchIntervalInBackground: false,
		refetchOnMount: true,
		refetchOnWindowFocus: true,
		refetchOnReconnect: true,
	});
};

export const chatSearch = (q: string) =>
	queryOptions({
		queryKey: chatQueryKeys.search(q),
		queryFn: () =>
			API.experimental.getChats({
				limit: CHAT_SEARCH_LIMIT,
				q,
			}),
		staleTime: CHAT_METADATA_STALE_MS,
		gcTime: CHAT_QUERY_GC_MS,
		retry: CHAT_QUERY_RETRY_COUNT,
		refetchInterval: CHAT_FOREGROUND_POLL_MS,
		refetchIntervalInBackground: false,
		refetchOnMount: true,
		refetchOnWindowFocus: true,
		refetchOnReconnect: true,
	});

export const chat = (chatId: string) =>
	queryOptions({
		queryKey: chatQueryKeys.detail(chatId),
		queryFn: async ({ client }) => {
			const response: ChatDetailProjection =
				await API.experimental.getChat(chatId);
			return preserveChatExecutionOverlay(client, response);
		},
		staleTime: CHAT_METADATA_STALE_MS,
		gcTime: CHAT_QUERY_GC_MS,
		retry: CHAT_QUERY_RETRY_COUNT,
		refetchInterval: CHAT_FOREGROUND_POLL_MS,
		refetchIntervalInBackground: false,
		refetchOnMount: true,
		refetchOnWindowFocus: true,
		refetchOnReconnect: true,
	});

export const chatACL = (chatId: string) =>
	queryOptions({
		queryKey: chatQueryKeys.acl(chatId),
		queryFn: () => API.experimental.getChatACL(chatId),
		staleTime: 0,
		gcTime: CHAT_QUERY_GC_MS,
		retry: false,
		refetchOnMount: true,
		refetchOnWindowFocus: true,
		refetchOnReconnect: true,
	});

const MESSAGES_PAGE_SIZE = 50;

export const chatMessagesForInfiniteScroll = (chatId: string) =>
	infiniteQueryOptions({
		queryKey: chatQueryKeys.messages(chatId),
		initialPageParam: undefined as number | undefined,
		select: selectChatMessagesProjection,
		queryFn: ({ pageParam }) =>
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
		staleTime: Number.POSITIVE_INFINITY,
		gcTime: CHAT_QUERY_GC_MS,
		retry: CHAT_QUERY_RETRY_COUNT,
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
	});

// Cap requested prompts to keep the response small; well under the server-side maximum.
const PROMPT_HISTORY_LIMIT = 500;

const PROMPTS_STALE_MS = 30_000;

export const chatPromptsQuery = (chatId: string) =>
	queryOptions({
		queryKey: chatQueryKeys.prompts(chatId),
		queryFn: () =>
			API.experimental.getChatPrompts(chatId, { limit: PROMPT_HISTORY_LIMIT }),
		staleTime: PROMPTS_STALE_MS,
		gcTime: CHAT_QUERY_GC_MS,
		retry: CHAT_QUERY_RETRY_COUNT,
		refetchOnMount: true,
		refetchOnWindowFocus: true,
		refetchOnReconnect: true,
		enabled: chatId !== "",
	});

export const archiveChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) =>
		API.experimental.updateChat(chatId, { archived: true }),
	onMutate: async (chatId: string) => {
		await queryClient.cancelQueries({
			queryKey: chatQueryKeys.lists(),
		});
		await cancelCachedChat(queryClient, chatId);
		const previousChat = getCachedChat(queryClient, chatId);
		// Flip the archived flag in loaded lists and remove stale embedded
		// copies before REST repairs collection membership. Reuse
		// patchChatArchiveState so the optimistic snapshot matches the
		// confirmed onSuccess state,
		// including the pin_order reset for an archived chat.
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? patchChatArchiveState(chat, true) : chat,
			),
		);
		removeChildFromParentInCache(queryClient, chatId);
		if (previousChat) {
			updateCachedChat(queryClient, chatId, () =>
				patchChatArchiveState(previousChat, true),
			);
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
			const previousChat = context.previousChat;
			updateCachedChat(queryClient, chatId, (current) =>
				current
					? {
							...current,
							archived: previousChat.archived,
							pin_order: previousChat.pin_order,
						}
					: current,
			);
		}
	},
	onSuccess: (_data: unknown, chatId: string) => {
		applyChatArchiveStateToCaches(queryClient, chatId, true);
	},
	onSettled: (_data: unknown, _error: unknown, chatId: string) => {
		void repairChatFamilyMetadataAfterMutation(queryClient, chatId);
	},
});

export const unarchiveChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) =>
		API.experimental.updateChat(chatId, { archived: false }),
	onMutate: async (chatId: string) => {
		await queryClient.cancelQueries({
			queryKey: chatQueryKeys.lists(),
		});
		await cancelCachedChat(queryClient, chatId);
		const previousChat = getCachedChat(queryClient, chatId);
		// Reuse patchChatArchiveState so the optimistic snapshot
		// matches the confirmed onSuccess state.
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? patchChatArchiveState(chat, false) : chat,
			),
		);
		if (previousChat) {
			updateCachedChat(queryClient, chatId, () =>
				patchChatArchiveState(previousChat, false),
			);
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
			const previousChat = context.previousChat;
			updateCachedChat(queryClient, chatId, (current) =>
				current
					? {
							...current,
							archived: previousChat.archived,
							pin_order: previousChat.pin_order,
						}
					: current,
			);
		}
	},
	onSuccess: (_data: unknown, chatId: string) => {
		applyChatArchiveStateToCaches(queryClient, chatId, false);
	},
	onSettled: (_data: unknown, _error: unknown, chatId: string) => {
		void repairChatFamilyMetadataAfterMutation(queryClient, chatId);
	},
});

export const updateChatPlanMode = (queryClient: QueryClient) => ({
	mutationFn: ({ chatId, planMode }: UpdateChatPlanModeVariables) =>
		API.experimental.updateChat(chatId, {
			plan_mode: toChatPlanModePayload(planMode),
		}),
	onMutate: async ({ chatId, planMode }: UpdateChatPlanModeVariables) => {
		await queryClient.cancelQueries({
			queryKey: chatQueryKeys.lists(),
		});
		await cancelCachedChat(queryClient, chatId);
		const previousChat = getCachedChat(queryClient, chatId);
		updateChatCollectionCaches(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? { ...chat, plan_mode: planMode } : chat,
			),
		);
		if (previousChat) {
			updateCachedChat(queryClient, chatId, () => ({
				...previousChat,
				plan_mode: planMode,
			}));
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
		updateChatCollectionCaches(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId
					? {
							...chat,
							plan_mode: previousChat.plan_mode,
						}
					: chat,
			),
		);
		updateCachedChat(queryClient, chatId, (current) =>
			current ? { ...current, plan_mode: previousChat.plan_mode } : current,
		);
	},
	onSettled: (
		_data: unknown,
		_error: unknown,
		{ chatId }: UpdateChatPlanModeVariables,
	) => repairChatMetadataAfterMutation(queryClient, chatId),
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
			queryKey: chatQueryKeys.lists(),
		});
		await cancelCachedChat(queryClient, chatId);
		const previousChat = getCachedChat(queryClient, chatId);
		updateChatCollectionCaches(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId
					? { ...chat, workspace_id: workspaceId ?? undefined }
					: chat,
			),
		);
		if (previousChat) {
			updateCachedChat(queryClient, chatId, () => ({
				...previousChat,
				workspace_id: workspaceId ?? undefined,
			}));
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
			updateChatCollectionCaches(queryClient, (chats) =>
				chats.map((chat) =>
					chat.id === chatId
						? {
								...chat,
								workspace_id: previousChat.workspace_id,
							}
						: chat,
				),
			);
			updateCachedChat(queryClient, chatId, (current) =>
				current
					? { ...current, workspace_id: previousChat.workspace_id }
					: current,
			);
		}
	},
	onSettled: (
		_data: unknown,
		_error: unknown,
		{ chatId }: UpdateChatWorkspaceVariables,
	) => repairChatMetadataAfterMutation(queryClient, chatId),
});

export const pinChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) =>
		API.experimental.updateChat(chatId, { pin_order: 1 }),
	onMutate: async (chatId: string) => {
		await queryClient.cancelQueries({
			queryKey: chatQueryKeys.lists(),
		});
		await cancelCachedChat(queryClient, chatId);
		const previousChat = getCachedChat(queryClient, chatId);
		const optimisticPinOrder = 1;
		updateChatCollectionCaches(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? { ...chat, pin_order: optimisticPinOrder } : chat,
			),
		);
		if (previousChat) {
			updateCachedChat(queryClient, chatId, () => ({
				...previousChat,
				pin_order: optimisticPinOrder,
			}));
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
		if (context?.previousChat) {
			const previousChat = context.previousChat;
			updateChatCollectionCaches(queryClient, (chats) =>
				chats.map((chat) =>
					chat.id === chatId
						? { ...chat, pin_order: previousChat.pin_order }
						: chat,
				),
			);
			updateCachedChat(queryClient, chatId, (current) =>
				current ? { ...current, pin_order: previousChat.pin_order } : current,
			);
		}
	},
	onSettled: (_data: unknown, _error: unknown, chatId: string) =>
		repairChatMetadataAfterMutation(queryClient, chatId),
});

export const unpinChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) =>
		API.experimental.updateChat(chatId, { pin_order: 0 }),
	onMutate: async (chatId: string) => {
		await queryClient.cancelQueries({
			queryKey: chatQueryKeys.lists(),
		});
		await cancelCachedChat(queryClient, chatId);
		const previousChat = getCachedChat(queryClient, chatId);
		updateChatCollectionCaches(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? { ...chat, pin_order: 0 } : chat,
			),
		);
		if (previousChat) {
			updateCachedChat(queryClient, chatId, () => ({
				...previousChat,
				pin_order: 0,
			}));
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
		if (context?.previousChat) {
			const previousChat = context.previousChat;
			updateChatCollectionCaches(queryClient, (chats) =>
				chats.map((chat) =>
					chat.id === chatId
						? { ...chat, pin_order: previousChat.pin_order }
						: chat,
				),
			);
			updateCachedChat(queryClient, chatId, (current) =>
				current ? { ...current, pin_order: previousChat.pin_order } : current,
			);
		}
	},
	onSettled: (_data: unknown, _error: unknown, chatId: string) =>
		repairChatMetadataAfterMutation(queryClient, chatId),
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
			queryKey: chatQueryKeys.lists(),
		});
		await cancelCachedChat(queryClient, chatId);

		const previousChat = getCachedChat(queryClient, chatId);
		updateChatCollectionCaches(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? { ...chat, pin_order: pinOrder } : chat,
			),
		);
		if (previousChat) {
			updateCachedChat(queryClient, chatId, (chat) =>
				chat ? { ...chat, pin_order: pinOrder } : chat,
			);
		}
		return { previousChat };
	},
	onError: (
		_error: unknown,
		{ chatId }: { chatId: string; pinOrder: number },
		context: { previousChat?: TypesGen.Chat } | undefined,
	) => {
		if (!context?.previousChat) {
			return;
		}
		const previousPinOrder = context.previousChat.pin_order;
		updateChatCollectionCaches(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? { ...chat, pin_order: previousPinOrder } : chat,
			),
		);
		updateCachedChat(queryClient, chatId, (chat) =>
			chat ? { ...chat, pin_order: previousPinOrder } : chat,
		);
	},
	onSettled: (
		_data: unknown,
		_error: unknown,
		{ chatId }: { chatId: string; pinOrder: number },
	) => repairChatMetadataAfterMutation(queryClient, chatId),
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
		updateCachedChat(queryClient, chatId, (chat) =>
			chat ? { ...chat, title } : chat,
		);
		updateChatCollectionCaches(queryClient, (chats) =>
			chats.map((chat) => (chat.id === chatId ? { ...chat, title } : chat)),
		);
	},

	onSettled: (
		_data: unknown,
		_error: unknown,
		{ chatId }: UpdateChatTitleVariables,
	) => {
		void repairChatMetadataAfterMutation(queryClient, chatId);
	},
});

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
		queryKey: chatQueryKeys.debugRuns(chatId),
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
		staleTime: 0,
		gcTime: CHAT_QUERY_GC_MS,
		retry: false,
		refetchOnMount: true,
		refetchOnWindowFocus: true,
		refetchOnReconnect: true,
	});

export const chatDebugRun = (chatId: string, runId: string) =>
	queryOptions({
		queryKey: chatQueryKeys.debugRun(chatId, runId),
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
		staleTime: 0,
		gcTime: CHAT_QUERY_GC_MS,
		retry: false,
		refetchOnMount: true,
		refetchOnWindowFocus: true,
		refetchOnReconnect: true,
	});

const invalidateChatDebugRuns = (queryClient: QueryClient, chatId: string) => {
	return queryClient.invalidateQueries({
		queryKey: chatQueryKeys.debugRuns(chatId),
	});
};

export const createChat = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.CreateChatRequest) =>
		API.experimental.createChat(req),
	onSettled: (createdChat: TypesGen.Chat | undefined) =>
		createdChat
			? repairChatMetadataAfterMutation(queryClient, createdChat.id)
			: invalidateChatCollectionQueries(queryClient),
});

const chatStateMutationKey = (chatId: string) =>
	["chat-state-mutation", chatId] as const;

type ChatMessagesMutationContext = {
	previousData?: InfiniteData<TypesGen.ChatMessagesResponse>;
	ownedData?: InfiniteData<TypesGen.ChatMessagesResponse>;
};

const beginChatMessagesMutation = async (
	queryClient: QueryClient,
	chatId: string,
	updater?: (
		data: InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
	) => InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
): Promise<ChatMessagesMutationContext> => {
	await cancelCachedChatMessages(queryClient, chatId);
	const previousData = getCachedChatMessages(queryClient, chatId);
	if (updater) {
		updateCachedChatMessages(queryClient, chatId, updater);
	}
	return {
		previousData,
		ownedData: getCachedChatMessages(queryClient, chatId),
	};
};

const updateOwnedChatMessagesMutation = (
	queryClient: QueryClient,
	chatId: string,
	context: ChatMessagesMutationContext | undefined,
	updater: (
		data: InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
	) => InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
): boolean => {
	if (
		!context ||
		getCachedChatMessages(queryClient, chatId) !== context.ownedData
	) {
		return false;
	}
	updateCachedChatMessages(queryClient, chatId, updater);
	context.ownedData = getCachedChatMessages(queryClient, chatId);
	return true;
};

const rollbackOwnedChatMessagesMutation = (
	queryClient: QueryClient,
	chatId: string,
	context: ChatMessagesMutationContext | undefined,
): void => {
	if (
		!context?.previousData ||
		getCachedChatMessages(queryClient, chatId) !== context.ownedData
	) {
		return;
	}
	updateCachedChatMessages(queryClient, chatId, () => context.previousData);
};

type ChatExecutionMutationContext = {
	previousChat?: ChatDetailProjection;
	ownedChat?: ChatDetailProjection;
};

const beginChatExecutionMutation = async (
	queryClient: QueryClient,
	chatId: string,
): Promise<ChatExecutionMutationContext> => {
	await cancelCachedChat(queryClient, chatId);
	const previousChat = getCachedChat(queryClient, chatId);
	updateCachedChat(queryClient, chatId, (current) =>
		current
			? {
					...current,
					status: "running",
					last_error: undefined,
					action_required: undefined,
				}
			: current,
	);
	return {
		previousChat,
		ownedChat: getCachedChat(queryClient, chatId),
	};
};

const updateOwnedChatExecutionMutation = (
	queryClient: QueryClient,
	chatId: string,
	context: ChatExecutionMutationContext | undefined,
	updater: (chat: ChatDetailProjection) => ChatDetailProjection,
): boolean => {
	const current = getCachedChat(queryClient, chatId);
	if (!context || !current || current !== context.ownedChat) {
		return false;
	}
	updateCachedChat(queryClient, chatId, (chat) =>
		chat ? updater(chat) : chat,
	);
	context.ownedChat = getCachedChat(queryClient, chatId);
	return true;
};

const rollbackOwnedChatExecutionMutation = (
	queryClient: QueryClient,
	chatId: string,
	context: ChatExecutionMutationContext | undefined,
): void => {
	if (
		!context?.previousChat ||
		getCachedChat(queryClient, chatId) !== context.ownedChat
	) {
		return;
	}
	updateCachedChat(queryClient, chatId, () => context.previousChat);
};

const reconcileChatStateMutation = async (
	queryClient: QueryClient,
	chatId: string,
): Promise<void> => {
	await Promise.all([
		invalidateCachedChat(queryClient, chatId),
		invalidateCachedChatMessages(queryClient, chatId),
	]);
};

export const createChatMessage = (
	queryClient: QueryClient,
	chatId: string,
) => ({
	mutationKey: chatStateMutationKey(chatId),
	mutationFn: (req: CreateChatMessageRequestWithClearablePlanMode) =>
		API.experimental.createChatMessage(chatId, req),
	onMutate: async (): Promise<
		ChatMessagesMutationContext & ChatExecutionMutationContext
	> => {
		const messagesContext = await beginChatMessagesMutation(
			queryClient,
			chatId,
		);
		await cancelCachedChat(queryClient, chatId);
		return {
			...messagesContext,
			ownedChat: getCachedChat(queryClient, chatId),
		};
	},
	onError: (
		_error: unknown,
		_variables: CreateChatMessageRequestWithClearablePlanMode,
		context:
			| (ChatMessagesMutationContext & ChatExecutionMutationContext)
			| undefined,
	) => {
		rollbackOwnedChatMessagesMutation(queryClient, chatId, context);
	},
	onSuccess: (
		response?: TypesGen.CreateChatMessageResponse,
		_variables?: CreateChatMessageRequestWithClearablePlanMode,
		context?: ChatMessagesMutationContext & ChatExecutionMutationContext,
	) => {
		if (!response) {
			return;
		}
		if (!response.queued) {
			updateOwnedChatExecutionMutation(
				queryClient,
				chatId,
				context,
				(current) => ({
					...current,
					status: "running",
					last_error: undefined,
					action_required: undefined,
				}),
			);
		}
		updateOwnedChatMessagesMutation(
			queryClient,
			chatId,
			context,
			(currentData) => {
				let updatedData = currentData;
				if (response.message) {
					updatedData = upsertChatMessagesInData(updatedData, [
						response.message,
					]);
				}
				if (response.queued_message) {
					updatedData = upsertQueuedMessageInData(
						updatedData,
						response.queued_message,
					);
				}
				return updatedData;
			},
		);
	},
	onSettled: async (
		_data: unknown,
		_error: unknown,
		variables: CreateChatMessageRequestWithClearablePlanMode,
	) => {
		await Promise.all([
			reconcileChatStateMutation(queryClient, chatId),
			invalidateChatDebugRuns(queryClient, chatId),
			invalidateCachedChatPrompts(queryClient, chatId),
			...(variables.plan_mode !== undefined
				? [repairChatMetadataAfterMutation(queryClient, chatId)]
				: []),
		]);
	},
});

type EditChatMessageMutationArgs = {
	messageId: number;
	optimisticMessage?: TypesGen.ChatMessage;
	req: TypesGen.EditChatMessageRequest;
};

export const editChatMessage = (queryClient: QueryClient, chatId: string) => ({
	mutationKey: chatStateMutationKey(chatId),
	mutationFn: ({ messageId, req }: EditChatMessageMutationArgs) =>
		API.experimental.editChatMessage(chatId, messageId, req),
	onMutate: async ({
		messageId,
		optimisticMessage,
	}: EditChatMessageMutationArgs): Promise<
		ChatMessagesMutationContext & ChatExecutionMutationContext
	> => {
		const [messagesContext, executionContext] = await Promise.all([
			beginChatMessagesMutation(queryClient, chatId, (current) =>
				projectEditedConversationIntoCache({
					currentData: current,
					editedMessageId: messageId,
					replacementMessage: optimisticMessage,
					queuedMessages: [],
				}),
			),
			beginChatExecutionMutation(queryClient, chatId),
		]);
		return { ...messagesContext, ...executionContext };
	},
	onError: (
		_error: unknown,
		_variables: EditChatMessageMutationArgs,
		context:
			| (ChatMessagesMutationContext & ChatExecutionMutationContext)
			| undefined,
	) => {
		rollbackOwnedChatMessagesMutation(queryClient, chatId, context);
		rollbackOwnedChatExecutionMutation(queryClient, chatId, context);
	},
	onSuccess: (
		response: TypesGen.EditChatMessageResponse,
		variables: EditChatMessageMutationArgs,
	) => {
		updateCachedChatMessages(queryClient, chatId, (current) =>
			reconcileEditedMessageInCache({
				currentData: current,
				optimisticMessageId: variables.messageId,
				responseMessage: response.message,
			}),
		);
	},
	onSettled: () => {
		void Promise.all([
			reconcileChatStateMutation(queryClient, chatId),
			invalidateCachedChatPrompts(queryClient, chatId),
			invalidateChatDebugRuns(queryClient, chatId),
		]);
	},
});

export const interruptChat = (queryClient: QueryClient, chatId: string) => ({
	mutationKey: chatStateMutationKey(chatId),
	mutationFn: () => API.experimental.interruptChat(chatId),
	onMutate: async (): Promise<
		ChatMessagesMutationContext & ChatExecutionMutationContext
	> => {
		await cancelCachedChat(queryClient, chatId);
		const messagesContext = await beginChatMessagesMutation(
			queryClient,
			chatId,
		);
		return {
			...messagesContext,
			ownedChat: getCachedChat(queryClient, chatId),
		};
	},
	onError: (
		_error: unknown,
		_variables: undefined,
		context:
			| (ChatMessagesMutationContext & ChatExecutionMutationContext)
			| undefined,
	) => {
		rollbackOwnedChatMessagesMutation(queryClient, chatId, context);
	},
	onSuccess: (
		updatedChat?: TypesGen.Chat,
		_variables?: undefined,
		context?: ChatMessagesMutationContext & ChatExecutionMutationContext,
	) => {
		if (!updatedChat) {
			return;
		}
		if (!context || getCachedChat(queryClient, chatId) === context.ownedChat) {
			updateCachedChat(queryClient, chatId, () => updatedChat);
		}
		const replaceChat = (chat: TypesGen.Chat) =>
			chat.id === chatId ? updatedChat : chat;
		updateChatCollectionCaches(queryClient, (chats) => chats.map(replaceChat));
		updateChildInParentCache(queryClient, replaceChat, chatId);
	},
	onSettled: async () => {
		await Promise.all([
			reconcileChatStateMutation(queryClient, chatId),
			invalidateChatDebugRuns(queryClient, chatId),
		]);
	},
});

/**
 * Re-pins the chat to its agent's latest context snapshot, clearing the
 * dirty marker. On success the returned chat (carrying the freshly pinned
 * resources) is written into the open-chat cache, and the lightweight
 * context flags are propagated across the list caches so the dirty
 * indicator clears in the sidebar too.
 */
export const refreshChatContext = (
	queryClient: QueryClient,
	chatId: string,
) => ({
	mutationFn: () => API.experimental.refreshChatContext(chatId),
	onSuccess: (updatedChat: TypesGen.Chat) => {
		updateCachedChat(queryClient, chatId, (cached) =>
			cached ? { ...cached, context: updatedChat.context } : updatedChat,
		);
		const applyContext = (chat: TypesGen.Chat): TypesGen.Chat =>
			chat.id === chatId ? { ...chat, context: updatedChat.context } : chat;
		updateChatCollectionCaches(queryClient, (chats) => {
			let changed = false;
			const next = chats.map((chat) => {
				const updated = applyContext(chat);
				if (updated !== chat) {
					changed = true;
				}
				return updated;
			});
			return changed ? next : chats;
		});
		updateChildInParentCache(queryClient, applyContext, chatId);
	},
});

export const deleteChatQueuedMessage = (
	queryClient: QueryClient,
	chatId: string,
) => ({
	mutationKey: chatStateMutationKey(chatId),
	mutationFn: (queuedMessageId: number) =>
		API.experimental.deleteChatQueuedMessage(chatId, queuedMessageId),
	onMutate: (queuedMessageId: number) =>
		beginChatMessagesMutation(queryClient, chatId, (currentData) =>
			removeQueuedMessageFromData(currentData, queuedMessageId),
		),
	onError: (
		_error: unknown,
		_queuedMessageId: number,
		context: ChatMessagesMutationContext | undefined,
	) => {
		rollbackOwnedChatMessagesMutation(queryClient, chatId, context);
	},
	onSuccess: () => undefined,
	onSettled: () => reconcileChatStateMutation(queryClient, chatId),
});

export const promoteChatQueuedMessage = (
	queryClient: QueryClient,
	chatId: string,
) => ({
	mutationKey: chatStateMutationKey(chatId),
	mutationFn: (queuedMessageId: number) =>
		API.experimental.promoteChatQueuedMessage(chatId, queuedMessageId),
	onMutate: async (
		queuedMessageId: number,
	): Promise<ChatMessagesMutationContext & ChatExecutionMutationContext> => {
		const [messagesContext, executionContext] = await Promise.all([
			beginChatMessagesMutation(queryClient, chatId, (currentData) =>
				removeQueuedMessageFromData(currentData, queuedMessageId),
			),
			beginChatExecutionMutation(queryClient, chatId),
		]);
		return { ...messagesContext, ...executionContext };
	},
	onError: (
		_error: unknown,
		_queuedMessageId: number,
		context:
			| (ChatMessagesMutationContext & ChatExecutionMutationContext)
			| undefined,
	) => {
		rollbackOwnedChatMessagesMutation(queryClient, chatId, context);
		rollbackOwnedChatExecutionMutation(queryClient, chatId, context);
	},
	onSuccess: () => undefined,
	onSettled: async () => {
		await Promise.all([
			reconcileChatStateMutation(queryClient, chatId),
			invalidateChatDebugRuns(queryClient, chatId),
		]);
	},
});

export const chatDiffContents = (chatId: string) =>
	queryOptions({
		queryKey: chatQueryKeys.diffContents(chatId),
		queryFn: () => API.experimental.getChatDiffContents(chatId),
		staleTime: CHAT_AUXILIARY_STALE_MS,
		gcTime: CHAT_QUERY_GC_MS,
		retry: CHAT_QUERY_RETRY_COUNT,
		refetchOnMount: true,
		refetchOnWindowFocus: true,
		refetchOnReconnect: true,
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

const chatPersonalModelOverridesAdminSettingsKey = [
	...chatQueryKeys.all,
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

const chatDebugRetentionDaysKey = ["chat-debug-retention-days"] as const;

export const chatDebugRetentionDays = () => ({
	queryKey: chatDebugRetentionDaysKey,
	queryFn: () => API.experimental.getChatDebugRetentionDays(),
});

export const updateChatDebugRetentionDays = (queryClient: QueryClient) => ({
	mutationFn: API.experimental.updateChatDebugRetentionDays,
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatDebugRetentionDaysKey,
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
	...chatQueryKeys.all,
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

const toChatProviderConfig = (
	provider: TypesGen.AIProvider,
): TypesGen.ChatProviderConfig => ({
	id: provider.id,
	provider: provider.type,
	display_name: provider.display_name || provider.type,
	icon: provider.icon,
	enabled: provider.enabled,
	has_api_key: provider.api_keys.length > 0,
	central_api_key_enabled: true,
	allow_user_api_key: true,
	allow_central_api_key_fallback: true,
	base_url: provider.base_url,
	source: "database",
	created_at: provider.created_at,
	updated_at: provider.updated_at,
});

export const chatProviderConfigs = () => ({
	queryKey: chatProviderConfigsKey,
	queryFn: async (): Promise<TypesGen.ChatProviderConfig[]> => {
		const providers = await API.experimental.listAIProviders();
		return providers.map(toChatProviderConfig);
	},
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
	queryFn: async (): Promise<TypesGen.UserChatProviderConfig[]> => {
		const configs = await API.experimental.getUserAIProviderKeyConfigs();
		return configs.map((config) => ({
			provider_id: config.provider.id,
			provider: config.provider.type,
			display_name: config.provider.display_name || config.provider.type,
			icon: config.provider.icon,
			has_user_api_key: config.has_user_api_key,
			byok_enabled: config.byok_enabled,
			has_central_api_key_fallback: config.has_provider_api_key,
		}));
	},
});

type UpsertUserChatProviderKeyArgs = {
	providerConfigId: string;
	req: TypesGen.CreateUserChatProviderKeyRequest;
};

export const upsertUserChatProviderKey = (queryClient: QueryClient) => ({
	mutationFn: ({ providerConfigId, req }: UpsertUserChatProviderKeyArgs) =>
		API.experimental.upsertUserAIProviderKey(providerConfigId, req),
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
		API.experimental.deleteUserAIProviderKey(providerConfigId),
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
	[...chatQueryKeys.all, "costSummary", user, params] as const;

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
			[...chatQueryKeys.all, "costUsers", payload, pageNumber] as const,
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

export const chatUsageLimitStatusKey = [
	...chatQueryKeys.all,
	"usageLimitStatus",
] as const;

export const chatUsageLimitStatus = () => ({
	queryKey: chatUsageLimitStatusKey,
	queryFn: () => API.experimental.getChatUsageLimitStatus(),
	refetchInterval: 60_000,
});

const chatUsageLimitConfigKey = [
	...chatQueryKeys.all,
	"usageLimitConfig",
] as const;

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

type SetChatUserRoleVariables = {
	chatId: string;
	userId: string;
	role: TypesGen.ChatRole;
};

type SetChatGroupRoleVariables = {
	chatId: string;
	groupId: string;
	role: TypesGen.ChatRole;
};

export const setChatUserRole = (queryClient: QueryClient) => ({
	mutationFn: ({ chatId, userId, role }: SetChatUserRoleVariables) =>
		API.experimental.updateChatACL(chatId, {
			user_roles: { [userId]: role },
		}),
	onSettled: (
		_data: unknown,
		_error: unknown,
		{ chatId }: SetChatUserRoleVariables,
	) => repairChatMetadataAfterMutation(queryClient, chatId, { acl: true }),
});

export const setChatGroupRole = (queryClient: QueryClient) => ({
	mutationFn: ({ chatId, groupId, role }: SetChatGroupRoleVariables) =>
		API.experimental.updateChatACL(chatId, {
			group_roles: { [groupId]: role },
		}),
	onSettled: (
		_data: unknown,
		_error: unknown,
		{ chatId }: SetChatGroupRoleVariables,
	) => repairChatMetadataAfterMutation(queryClient, chatId, { acl: true }),
});
