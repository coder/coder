import isEqual from "lodash/isEqual";
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
import { ChatListSources } from "#/api/typesGenerated";
import {
	projectEditedConversationIntoCache,
	reconcileEditedMessageInCache,
} from "./chatMessageEdits";

const chatCollectionsKey = ["chats", "collections"] as const;

export const chatListFamilyKey = [...chatCollectionsKey, "list"] as const;

const chatSearchFamilyKey = [...chatCollectionsKey, "search"] as const;

const chatsByWorkspaceFamilyKey = [
	...chatCollectionsKey,
	"by-workspace",
] as const;

export const chatEntitiesFamilyKey = ["chats", "entities"] as const;

export const chatEntityKey = (chatId: string) =>
	[...chatEntitiesFamilyKey, chatId] as const;

export const chatFilesKey = ["chats", "files"] as const;

const chatAnalyticsKey = ["chats", "analytics"] as const;

const chatConfigKey = ["chats", "config"] as const;

export type ChatListPRStatusFilter = "draft" | "open" | "merged" | "closed";
export type ChatListStatusFilter = "read" | "unread";

type ChatListParams = Readonly<{
	archived: boolean;
	prStatuses: readonly ChatListPRStatusFilter[];
	status: ChatListStatusFilter | "all";
	sources: readonly TypesGen.ChatListSource[];
}>;

export type ChatListInput = Readonly<{
	archived?: boolean;
	prStatuses?: readonly ChatListPRStatusFilter[];
	chatStatus?: ChatListStatusFilter;
	sources?: readonly TypesGen.ChatListSource[];
}>;

type ChatSearchParams = Readonly<{ q: string }>;

const chatsByWorkspaceKey = (workspaceIds: readonly string[]) =>
	[...chatsByWorkspaceFamilyKey, workspaceIds] as const;

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

const canonicalWorkspaceIds = (
	workspaceIds: readonly string[],
): readonly string[] => {
	return [...new Set(workspaceIds)].sort();
};

export const chatsByWorkspace = (workspaceIds: readonly string[]) => {
	const sorted = canonicalWorkspaceIds(workspaceIds);
	return {
		queryKey: chatsByWorkspaceKey(sorted),
		queryFn: () => API.experimental.getChatsByWorkspace(sorted),
		enabled: sorted.length > 0,
	};
};

/**
 * Writes an updater across every cached chat list entry by targeting the
 * list family prefix. Each filter combination is a separate query whose
 * key starts with that prefix, so setQueriesData hits them all at once;
 * setQueryData on a single key would silently miss the sibling variants.
 */
export const updateInfiniteChatsCache = (
	queryClient: QueryClient,
	updater: (chats: TypesGen.Chat[]) => TypesGen.Chat[],
) => {
	queryClient.setQueriesData<InfiniteChatsCacheData>(
		{ queryKey: chatListFamilyKey },
		(prev) => {
			if (!prev?.pages) return prev;
			const nextPages = prev.pages.map((page) => updater(page));
			// Only return a new reference if something actually changed.
			const changed = nextPages.some((page, i) => page !== prev.pages[i]);
			return changed ? { ...prev, pages: nextPages } : prev;
		},
	);
};

/**
 * Prepends a new chat to the first page of every infinite chats query
 * in the cache, but only if the chat doesn't already exist in any
 * page. This avoids the per-page duplication that would occur if
 * a prepend updater were passed to updateInfiniteChatsCache, which
 * runs independently on each page. Lists whose archived filter
 * conflicts with the chat's archive state are skipped, so an active
 * chat is never inserted into an archived-only list.
 */
export const prependToInfiniteChatsCache = (
	queryClient: QueryClient,
	chat: TypesGen.Chat,
) => {
	const queries = queryClient.getQueriesData<InfiniteChatsCacheData>({
		queryKey: chatListFamilyKey,
	});
	for (const [queryKey] of queries) {
		const archivedFilter = archivedFilterForChatListKey(queryKey);
		if (archivedFilter !== undefined && archivedFilter !== chat.archived) {
			continue;
		}
		queryClient.setQueryData<InfiniteChatsCacheData>(queryKey, (prev) => {
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
	}
};

/**
 * Reads the flat list of chats from the first matching infinite query
 * in the cache. Returns undefined when no data is cached yet.
 */
export const readInfiniteChatsCache = (
	queryClient: QueryClient,
): TypesGen.Chat[] | undefined => {
	const queries = queryClient.getQueriesData<InfiniteChatsCacheData>({
		queryKey: chatListFamilyKey,
	});
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

// Inverse of chatListKey, which builds keys as [...chatListFamilyKey, params].
// The params object lives in the slot immediately after the list family
// prefix, so derive both the expected length and the params index from
// chatListFamilyKey. If chatListKey's shape changes, this must change with
// it; the "chatListKey shape" test in chats.test.ts guards that contract.
const archivedFilterForChatListKey = (
	queryKey: readonly unknown[],
): boolean | undefined => {
	if (queryKey.length !== chatListFamilyKey.length + 1) {
		return undefined;
	}
	const params = queryKey[chatListFamilyKey.length];
	if (!params || typeof params !== "object") {
		return undefined;
	}
	const archived = (params as { archived?: unknown }).archived;
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

/**
 * Applies an accepted archive state to loaded sidebar, search, and
 * detail caches. Removes the chat from any filtered list whose archived
 * filter conflicts with the new state, and resets pin_order to 0 when
 * archiving.
 *
 * Search rows are removed rather than patched: a cached row matched its
 * query's archived filter before the change, so after the change it
 * belongs to a different result set. Search invalidations issued by the
 * callers repopulate any result set that still matches.
 */
export const applyChatArchiveStateToCaches = (
	queryClient: QueryClient,
	chatId: string,
	archived: boolean,
) => {
	queryClient.setQueryData<TypesGen.Chat | undefined>(
		chatEntityKey(chatId),
		(chat) => (chat ? patchChatArchiveState(chat, archived) : chat),
	);

	if (archived) {
		removeChildFromParentInCache(queryClient, chatId);
	} else {
		updateChildInParentCache(
			queryClient,
			(child) => patchChatArchiveState(child, archived),
			chatId,
		);
	}

	const queries = queryClient.getQueriesData<InfiniteChatsCacheData>({
		queryKey: chatListFamilyKey,
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
					if (chat.id !== chatId) {
						nextPage.push(chat);
						continue;
					}

					if (archivedFilter !== undefined && archivedFilter !== archived) {
						pageChanged = true;
						continue;
					}

					const updatedChat = patchChatArchiveState(chat, archived);
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

	const searchQueries = queryClient.getQueriesData<TypesGen.Chat[]>({
		queryKey: chatSearchFamilyKey,
	});
	for (const [queryKey] of searchQueries) {
		queryClient.setQueryData<TypesGen.Chat[]>(queryKey, (prev) => {
			if (!prev) {
				return prev;
			}
			const next = prev.filter((row) => row.id !== chatId);
			return next.length === prev.length ? prev : next;
		});
	}
};

/**
 * Watch-event effect for the `deleted` kind, which the server publishes
 * once per family member when a chat family is archived. Archive is a
 * patch, never an eviction: the entity and its sub-resources stay
 * cached so an open route flips to the archived read-only state without
 * a loading flash or a zombie render.
 */
export const applyWatchedChatArchived = (
	queryClient: QueryClient,
	chat: TypesGen.Chat,
) => {
	void cancelChatListRefetches(queryClient);
	if (queryClient.getQueryData(chatEntityKey(chat.id)) === undefined) {
		void resetUnloadedChatEntity(queryClient, chat.id);
	} else {
		void cancelLoadedChatEntityRefetch(queryClient, chat.id);
	}
	applyChatArchiveStateToCaches(queryClient, chat.id, true);
	removeChatFromChatsByWorkspace(queryClient, chat.id);
	void invalidateChatListQueries(queryClient);
	void invalidateChatsByWorkspace(queryClient);
	void invalidateChatSearches(queryClient);
};

/**
 * Watch-event effect for a root `created` event, which the server
 * publishes both for new chats and for unarchive transitions (one event
 * per family member). A cached entity marked archived identifies the
 * unarchive case; a truly new chat only needs the family invalidations
 * and never gets a speculative entity entry. The caller remains
 * responsible for list prepend and child insertion.
 */
export const applyWatchedChatCreatedOrUnarchived = (
	queryClient: QueryClient,
	chat: TypesGen.Chat,
) => {
	const cachedChat = queryClient.getQueryData<TypesGen.Chat>(
		chatEntityKey(chat.id),
	);
	if (cachedChat === undefined) {
		void resetUnloadedChatEntity(queryClient, chat.id);
	} else if (cachedChat.archived) {
		applyChatArchiveStateToCaches(queryClient, chat.id, false);
	}
	void invalidateChatListQueries(queryClient);
	void invalidateChatsByWorkspace(queryClient);
	void invalidateChatSearches(queryClient);
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
	const isSummaryEvent = eventKind === "summary_change";
	const isChatSummaryEvent = eventKind === "chat_summary_change";
	const isDiffStatusEvent = eventKind === "diff_status_change";
	const isContextDirtyEvent = eventKind === "context_dirty";
	const isCapacityEvent = eventKind === "capacity_change";
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
	// Queue marks keep chats.updated_at, but clears bump it. Apply capacity
	// events at equal timestamps and require status events to be newer,
	// preventing reordered snapshots from clearing or restoring the banner.
	const nextQueuedForCapacityAt =
		(isCapacityEvent && isFreshEnough) ||
		(isStatusEvent && updatedAtComparison < 0)
			? watchedChat.queued_for_capacity_at
			: cachedChat.queued_for_capacity_at;
	const nextWorkspaceId = isFreshEnough
		? (watchedChat.workspace_id ?? cachedChat.workspace_id)
		: cachedChat.workspace_id;
	// Single-chat reads repair agent/build bindings response-only, so watch
	// events can replay stale DB pairs. Adopting build_id with a mismatched
	// agent would split the repaired pair because merge never adopts agent_id.
	const nextBuildId =
		isFreshEnough && watchedChat.agent_id === cachedChat.agent_id
			? (watchedChat.build_id ?? cachedChat.build_id)
			: cachedChat.build_id;
	// All event types carry the current model config from the DB.
	const nextLastModelConfigId = isFreshEnough
		? watchedChat.last_model_config_id
		: cachedChat.last_model_config_id;
	// The summary writes (UpdateChatLastTurnSummary, UpdateChatSummary) never
	// bump chats.updated_at, and both events publish pre-write chat snapshots,
	// so updated_at cannot order summary_change against chat_summary_change and
	// isFreshEnough cannot guard these fields. Scope each field to its own
	// event, else one event's stale snapshot clobbers the other field's value.
	const nextLastTurnSummary = isSummaryEvent
		? watchedChat.last_turn_summary
		: cachedChat.last_turn_summary;
	const nextSummary = isChatSummaryEvent
		? watchedChat.summary
		: cachedChat.summary;
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
		nextLastTurnSummary === cachedChat.last_turn_summary &&
		nextSummary === cachedChat.summary &&
		nextHasUnread === cachedChat.has_unread &&
		nextUpdatedAt === cachedChat.updated_at &&
		nextContext === cachedChat.context &&
		nextQueuedForCapacityAt === cachedChat.queued_for_capacity_at
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
		summary: nextSummary,
		has_unread: nextHasUnread,
		updated_at: nextUpdatedAt,
		context: nextContext,
		queued_for_capacity_at: nextQueuedForCapacityAt,
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
		chatEntityKey(watchedChat.id),
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
		queryKey: chatListFamilyKey,
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

export const invalidateChatEntity = (
	queryClient: QueryClient,
	chatId: string,
) =>
	queryClient.invalidateQueries({
		queryKey: chatEntityKey(chatId),
		exact: true,
	});

export const invalidateChatListQueries = (queryClient: QueryClient) =>
	queryClient.invalidateQueries({
		queryKey: chatListFamilyKey,
	});

// Event kinds that can change which chat is newest for a workspace.
const BY_WORKSPACE_AFFECTING_EVENT_KINDS = new Set<TypesGen.ChatWatchEventKind>(
	["status_change", "action_required"],
);

export const shouldInvalidateChatsByWorkspace = (
	eventKind: TypesGen.ChatWatchEventKind,
): boolean => BY_WORKSPACE_AFFECTING_EVENT_KINDS.has(eventKind);

export const invalidateChatsByWorkspace = (queryClient: QueryClient) =>
	queryClient.invalidateQueries({
		queryKey: chatsByWorkspaceFamilyKey,
	});

// Watch events that change fields rendered in search results (title,
// status, diff status, action-required badge). Summary events are
// deliberately excluded: stale last_turn_summary subtitles are accepted
// until reconciliation lands.
const SEARCH_AFFECTING_EVENT_KINDS = new Set<TypesGen.ChatWatchEventKind>([
	"title_change",
	"status_change",
	"diff_status_change",
	"action_required",
]);

export const shouldInvalidateChatSearches = (
	eventKind: TypesGen.ChatWatchEventKind,
): boolean => SEARCH_AFFECTING_EVENT_KINDS.has(eventKind);

export const invalidateChatSearches = (queryClient: QueryClient) =>
	queryClient.invalidateQueries({
		queryKey: chatSearchFamilyKey,
	});

export const invalidateChatDebugRuns = (
	queryClient: QueryClient,
	chatId: string,
) =>
	queryClient.invalidateQueries({
		queryKey: chatDebugRunsKey(chatId),
	});

export const invalidateChatDiffContents = (
	queryClient: QueryClient,
	chatId: string,
) =>
	queryClient.invalidateQueries({
		queryKey: chatDiffContentsKey(chatId),
		exact: true,
	});

export const invalidateChatPrompts = (
	queryClient: QueryClient,
	chatId: string,
) =>
	queryClient.invalidateQueries({
		queryKey: chatPromptsKey(chatId),
		exact: true,
	});

export const invalidateChatMessages = (
	queryClient: QueryClient,
	chatId: string,
) =>
	queryClient.invalidateQueries({
		queryKey: chatMessagesKey(chatId),
		exact: true,
	});

export const invalidateChatACL = (queryClient: QueryClient, chatId: string) =>
	queryClient.invalidateQueries({
		queryKey: chatACLKey(chatId),
		exact: true,
	});

export const invalidateChatCostTree = (
	queryClient: QueryClient,
	rootChatId: string,
) =>
	queryClient.invalidateQueries({
		queryKey: chatCostTreeKey(rootChatId),
		exact: true,
	});

export const cancelChatListQueries = (queryClient: QueryClient) =>
	queryClient.cancelQueries({
		queryKey: chatListFamilyKey,
	});

/**
 * Cancel background chat-list refetches, leaving pagination fetches alone.
 * Call before applying WebSocket-driven cache updates, or a concurrent
 * refetch may overwrite them with stale data.
 */
export const cancelChatListRefetches = (queryClient: QueryClient) =>
	queryClient.cancelQueries({
		queryKey: chatListFamilyKey,
		predicate: isChatListRefetch,
	});

export const cancelChatEntity = (queryClient: QueryClient, chatId: string) =>
	queryClient.cancelQueries({
		queryKey: chatEntityKey(chatId),
		exact: true,
	});

// Cancelling a first-time fetch leaves the query pending with no retry,
// which the page shows as "Chat not found".
export const cancelLoadedChatEntityRefetch = (
	queryClient: QueryClient,
	chatId: string,
) => {
	if (queryClient.getQueryData(chatEntityKey(chatId)) === undefined) {
		return;
	}
	return queryClient.cancelQueries({
		queryKey: chatEntityKey(chatId),
		exact: true,
	});
};

/**
 * Restarts an active first-time fetch after a durable watch transition.
 * Invalidation reuses the stale initial promise when no data is loaded.
 */
export const resetUnloadedChatEntity = (
	queryClient: QueryClient,
	chatId: string,
) => {
	if (queryClient.getQueryData(chatEntityKey(chatId)) !== undefined) {
		return;
	}
	return queryClient.resetQueries({
		queryKey: chatEntityKey(chatId),
		exact: true,
	});
};

export const cancelChatMessages = (queryClient: QueryClient, chatId: string) =>
	queryClient.cancelQueries({
		queryKey: chatMessagesKey(chatId),
		exact: true,
	});

export const removeChatEntity = (queryClient: QueryClient, chatId: string) =>
	queryClient.removeQueries({
		queryKey: chatEntityKey(chatId),
		exact: true,
	});

export const removeChatFromChatsByWorkspace = (
	queryClient: QueryClient,
	chatId: string,
) =>
	queryClient.setQueriesData<Record<string, string>>(
		{ queryKey: chatsByWorkspaceFamilyKey },
		(prev) => {
			if (!prev) {
				return prev;
			}
			const next = Object.fromEntries(
				Object.entries(prev).filter(([, id]) => id !== chatId),
			);
			return Object.keys(next).length === Object.keys(prev).length
				? prev
				: next;
		},
	);

export const patchChatEntity = (
	queryClient: QueryClient,
	chatId: string,
	updater: (chat: TypesGen.Chat | undefined) => TypesGen.Chat | undefined,
) =>
	queryClient.setQueryData<TypesGen.Chat | undefined>(
		chatEntityKey(chatId),
		updater,
	);

export const patchChatMessages = (
	queryClient: QueryClient,
	chatId: string,
	updater: (
		data: InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
	) => InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
) =>
	queryClient.setQueryData<
		InfiniteData<TypesGen.ChatMessagesResponse> | undefined
	>(chatMessagesKey(chatId), updater);

const replaceMessagesInPage = (
	page: TypesGen.ChatMessagesResponse,
	incomingByID: ReadonlyMap<number, TypesGen.ChatMessage>,
	foundIDs: Set<number>,
): TypesGen.ChatMessagesResponse => {
	let pageChanged = false;

	const nextMessages = page.messages.map((existing) => {
		const incoming = incomingByID.get(existing.id);
		if (!incoming) {
			return existing;
		}

		foundIDs.add(existing.id);
		if (isEqual(existing, incoming)) {
			return existing;
		}

		pageChanged = true;
		return incoming;
	});

	return pageChanged ? { ...page, messages: nextMessages } : page;
};

const upsertMessagesAcrossPages = (
	currentData: InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
	messages: readonly TypesGen.ChatMessage[],
): InfiniteData<TypesGen.ChatMessagesResponse> | undefined => {
	if (!currentData?.pages?.length || messages.length === 0) {
		return currentData;
	}

	const incomingByID = new Map(
		messages.map((message) => [message.id, message]),
	);
	const foundIDs = new Set<number>();
	const nextPages = currentData.pages.map((page) =>
		replaceMessagesInPage(page, incomingByID, foundIDs),
	);
	const pagesChanged = nextPages.some(
		(page, index) => page !== currentData.pages[index],
	);

	const messagesToInsert = [...incomingByID.values()].filter(
		(message) => !foundIDs.has(message.id),
	);
	if (messagesToInsert.length === 0) {
		return pagesChanged ? { ...currentData, pages: nextPages } : currentData;
	}

	const firstPage = nextPages[0];
	const firstPageMessages = [...firstPage.messages, ...messagesToInsert].sort(
		(a, b) => b.id - a.id,
	);

	return {
		...currentData,
		pages: [
			{ ...firstPage, messages: firstPageMessages },
			...nextPages.slice(1),
		],
	};
};

const replaceMessagesHistory = (
	currentData: InfiniteData<TypesGen.ChatMessagesResponse> | undefined,
	messages: readonly TypesGen.ChatMessage[],
): InfiniteData<TypesGen.ChatMessagesResponse> | undefined => {
	if (!currentData?.pages?.length) {
		return currentData;
	}

	const firstPage = currentData.pages[0];
	const nextMessages = [...messages].sort((a, b) => b.id - a.id);
	const alreadyReplaced =
		currentData.pages.length === 1 &&
		!firstPage.has_more &&
		firstPage.messages.length === nextMessages.length &&
		firstPage.messages.every((existing, index) =>
			isEqual(existing, nextMessages[index]),
		);

	if (alreadyReplaced) {
		return currentData;
	}

	return {
		...currentData,
		pages: [{ ...firstPage, messages: nextMessages, has_more: false }],
		pageParams: currentData.pageParams.slice(0, 1),
	};
};

export const upsertChatMessages = (
	queryClient: QueryClient,
	chatId: string,
	messages: readonly TypesGen.ChatMessage[],
) => {
	return patchChatMessages(queryClient, chatId, (currentData) =>
		upsertMessagesAcrossPages(currentData, messages),
	);
};

export const replaceChatMessagesHistory = (
	queryClient: QueryClient,
	chatId: string,
	messages: readonly TypesGen.ChatMessage[],
) => {
	return patchChatMessages(queryClient, chatId, (currentData) =>
		replaceMessagesHistory(currentData, messages),
	);
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

export const CHAT_SOURCE_ORDER = [
	...ChatListSources,
] as const satisfies readonly TypesGen.ChatListSource[];

const chatSourceSet = new Set<TypesGen.ChatListSource>(CHAT_SOURCE_ORDER);

const canonicalizeChatSources = (
	sources: Iterable<unknown>,
): readonly TypesGen.ChatListSource[] => {
	const selected = new Set<TypesGen.ChatListSource>();
	for (const source of sources) {
		if (
			typeof source === "string" &&
			chatSourceSet.has(source as TypesGen.ChatListSource)
		) {
			selected.add(source as TypesGen.ChatListSource);
		}
	}
	return CHAT_SOURCE_ORDER.filter((source) => selected.has(source));
};

export const toChatListParams = (input?: ChatListInput): ChatListParams => ({
	archived: input?.archived ?? false,
	prStatuses: canonicalizeChatListPRStatuses(input?.prStatuses ?? []),
	status: input?.chatStatus ?? "all",
	sources: canonicalizeChatSources(input?.sources ?? []),
});

// Sidebar-emitted query shapes must match TestSearchChatsFrontendEmitted in
// coderd/searchquery/search_test.go.
export const getChatListQueryString = (
	params: ChatListParams,
): string | undefined => {
	const qParts: string[] = [];
	qParts.push(`archived:${params.archived}`);
	if (params.prStatuses.length) {
		qParts.push(`pr_status:${params.prStatuses.join(",")}`);
	}
	if (params.status !== "all") {
		qParts.push(`has_unread:${params.status === "unread"}`);
	}
	if (params.sources.length) {
		qParts.push(`source:${params.sources.join(",")}`);
	}
	return qParts.length > 0 ? qParts.join(" ") : undefined;
};

export const chatListKey = (params: ChatListParams) =>
	[...chatListFamilyKey, params] as const;

export const infiniteChats = (input?: ChatListInput) => {
	const limit = DEFAULT_CHAT_PAGE_LIMIT;
	const params = toChatListParams(input);
	const q = getChatListQueryString(params);

	return {
		queryKey: chatListKey(params),
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

const chatSearchKey = (params: ChatSearchParams) =>
	[...chatSearchFamilyKey, params] as const;

export const chatSearch = (params: ChatSearchParams) =>
	queryOptions({
		queryKey: chatSearchKey(params),
		queryFn: () =>
			API.experimental.getChats({
				limit: CHAT_SEARCH_LIMIT,
				q: params.q,
			}),
	});

export const chat = (chatId: string) => ({
	queryKey: chatEntityKey(chatId),
	queryFn: () => API.experimental.getChat(chatId),
});

export const chatACLKey = (chatId: string) =>
	[...chatEntityKey(chatId), "acl"] as const;

export const chatACL = (chatId: string) => ({
	queryKey: chatACLKey(chatId),
	queryFn: () => API.experimental.getChatACL(chatId),
});

const MESSAGES_PAGE_SIZE = 50;

export const chatMessagesKey = (chatId: string) =>
	[...chatEntityKey(chatId), "messages"] as const;

const chatQueueConvergenceKey = (chatId: string) =>
	[...chatEntityKey(chatId), "queue-convergence"] as const;

// The queued messages ride on the uncursored page of the messages endpoint,
// so settling the queue after a promote needs its own request. Refetching
// chatMessagesForInfiniteScroll would reload every page already scrolled.
export const chatQueueConvergence = (chatId: string) => ({
	queryKey: chatQueueConvergenceKey(chatId),
	queryFn: () => API.experimental.getChatMessages(chatId),
	gcTime: 0,
});

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

// Cap requested prompts to keep the response small; well under the server-side maximum.
const PROMPT_HISTORY_LIMIT = 500;

const PROMPTS_STALE_MS = 30_000;

export const chatPromptsKey = (chatId: string) =>
	[...chatEntityKey(chatId), "prompts"] as const;

export const chatPromptsQuery = (chatId: string) => ({
	queryKey: chatPromptsKey(chatId),
	queryFn: () =>
		API.experimental.getChatPrompts(chatId, { limit: PROMPT_HISTORY_LIMIT }),
	staleTime: PROMPTS_STALE_MS,
	enabled: chatId !== "",
});

export const archiveChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) =>
		API.experimental.updateChat(chatId, { archived: true }),
	onMutate: async (chatId: string) => {
		await cancelChatListQueries(queryClient);
		await cancelChatEntity(queryClient, chatId);
		const previousChat = queryClient.getQueryData<TypesGen.Chat>(
			chatEntityKey(chatId),
		);
		// Flip archived flag in the flat root list; strip the
		// chat from any parent's embedded children (individual
		// child archive). Reuse patchChatArchiveState so the
		// optimistic snapshot matches the confirmed onSuccess state,
		// including the pin_order reset for an archived chat.
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? patchChatArchiveState(chat, true) : chat,
			),
		);
		removeChildFromParentInCache(queryClient, chatId);
		if (previousChat) {
			queryClient.setQueryData<TypesGen.Chat>(
				chatEntityKey(chatId),
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
			patchChatEntity(queryClient, chatId, () => context.previousChat);
		}
	},
	onSuccess: (_data: unknown, chatId: string) => {
		applyChatArchiveStateToCaches(queryClient, chatId, true);
		removeChatFromChatsByWorkspace(queryClient, chatId);
	},
	onSettled: (_data: unknown, _error: unknown, chatId: string) => {
		void invalidateChatListQueries(queryClient);
		void invalidateChatEntity(queryClient, chatId);
		void invalidateChatsByWorkspace(queryClient);
		void invalidateChatSearches(queryClient);
	},
});

export const unarchiveChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) =>
		API.experimental.updateChat(chatId, { archived: false }),
	onMutate: async (chatId: string) => {
		await cancelChatListQueries(queryClient);
		await cancelChatEntity(queryClient, chatId);
		const previousChat = queryClient.getQueryData<TypesGen.Chat>(
			chatEntityKey(chatId),
		);
		// Reuse patchChatArchiveState so the optimistic snapshot
		// matches the confirmed onSuccess state.
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? patchChatArchiveState(chat, false) : chat,
			),
		);
		if (previousChat) {
			queryClient.setQueryData<TypesGen.Chat>(
				chatEntityKey(chatId),
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
			patchChatEntity(queryClient, chatId, () => context.previousChat);
		}
	},
	onSuccess: (_data: unknown, chatId: string) => {
		applyChatArchiveStateToCaches(queryClient, chatId, false);
	},
	onSettled: (_data: unknown, _error: unknown, chatId: string) => {
		void invalidateChatListQueries(queryClient);
		void invalidateChatEntity(queryClient, chatId);
		void invalidateChatsByWorkspace(queryClient);
		void invalidateChatSearches(queryClient);
	},
});

export const updateChatPlanMode = (queryClient: QueryClient) => ({
	mutationFn: ({ chatId, planMode }: UpdateChatPlanModeVariables) =>
		API.experimental.updateChat(chatId, {
			plan_mode: toChatPlanModePayload(planMode),
		}),
	onMutate: async ({ chatId, planMode }: UpdateChatPlanModeVariables) => {
		await cancelChatListQueries(queryClient);
		await cancelChatEntity(queryClient, chatId);
		const previousChat = queryClient.getQueryData<TypesGen.Chat>(
			chatEntityKey(chatId),
		);
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? { ...chat, plan_mode: planMode } : chat,
			),
		);
		if (previousChat) {
			queryClient.setQueryData<TypesGen.Chat>(chatEntityKey(chatId), {
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
		patchChatEntity(queryClient, chatId, () => previousChat);
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
		await cancelChatListQueries(queryClient);
		await cancelChatEntity(queryClient, chatId);
		const previousChat = queryClient.getQueryData<TypesGen.Chat>(
			chatEntityKey(chatId),
		);
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId
					? { ...chat, workspace_id: workspaceId ?? undefined }
					: chat,
			),
		);
		if (previousChat) {
			queryClient.setQueryData<TypesGen.Chat>(chatEntityKey(chatId), {
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
			patchChatEntity(queryClient, chatId, () => previousChat);
		}
	},
	onSettled: async (
		_data: unknown,
		_error: unknown,
		{ chatId }: UpdateChatWorkspaceVariables,
	) => {
		await invalidateChatListQueries(queryClient);
		await invalidateChatEntity(queryClient, chatId);
		await invalidateChatsByWorkspace(queryClient);
	},
});

export const pinChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) =>
		API.experimental.updateChat(chatId, { pin_order: 1 }),
	onMutate: async (chatId: string) => {
		await cancelChatListQueries(queryClient);
		await cancelChatEntity(queryClient, chatId);
		const previousChat = queryClient.getQueryData<TypesGen.Chat>(
			chatEntityKey(chatId),
		);
		const optimisticPinOrder = getNextOptimisticPinOrder(queryClient);
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? { ...chat, pin_order: optimisticPinOrder } : chat,
			),
		);
		if (previousChat) {
			queryClient.setQueryData<TypesGen.Chat>(chatEntityKey(chatId), {
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
			patchChatEntity(queryClient, chatId, () => context.previousChat);
		}
	},
	onSettled: async (_data: unknown, _error: unknown, chatId: string) => {
		await invalidateChatListQueries(queryClient);
		await invalidateChatEntity(queryClient, chatId);
	},
});

export const unpinChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) =>
		API.experimental.updateChat(chatId, { pin_order: 0 }),
	onMutate: async (chatId: string) => {
		await cancelChatListQueries(queryClient);
		await cancelChatEntity(queryClient, chatId);
		const previousChat = queryClient.getQueryData<TypesGen.Chat>(
			chatEntityKey(chatId),
		);
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) =>
				chat.id === chatId ? { ...chat, pin_order: 0 } : chat,
			),
		);
		if (previousChat) {
			queryClient.setQueryData<TypesGen.Chat>(chatEntityKey(chatId), {
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
			patchChatEntity(queryClient, chatId, () => context.previousChat);
		}
	},
	onSettled: async (_data: unknown, _error: unknown, chatId: string) => {
		await invalidateChatListQueries(queryClient);
		await invalidateChatEntity(queryClient, chatId);
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
		await cancelChatListQueries(queryClient);
		await cancelChatEntity(queryClient, chatId);

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
		await invalidateChatEntity(queryClient, chatId);
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
		patchChatEntity(queryClient, chatId, (chat) =>
			chat ? { ...chat, title } : chat,
		);
		updateInfiniteChatsCache(queryClient, (chats) =>
			chats.map((chat) => (chat.id === chatId ? { ...chat, title } : chat)),
		);
	},

	onSettled: (
		_data: unknown,
		_error: unknown,
		{ chatId }: UpdateChatTitleVariables,
	) => {
		void invalidateChatListQueries(queryClient);
		void invalidateChatEntity(queryClient, chatId);
		void invalidateChatSearches(queryClient);
	},
});

export const chatDebugRunsKey = (chatId: string) =>
	[...chatEntityKey(chatId), "debug-runs"] as const;

export const chatDebugRunKey = (chatId: string, runId: string) =>
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

export const createChat = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.CreateChatRequest) =>
		API.experimental.createChat(req),
	onSuccess: () => {
		void invalidateChatListQueries(queryClient);
		void invalidateChatsByWorkspace(queryClient);
		void invalidateChatSearches(queryClient);
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
		void invalidateChatEntity(queryClient, chatId);
		void invalidateChatPrompts(queryClient, chatId);
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
		await cancelChatMessages(queryClient, chatId);

		const previousData = queryClient.getQueryData<
			InfiniteData<TypesGen.ChatMessagesResponse>
		>(chatMessagesKey(chatId));

		patchChatMessages(queryClient, chatId, (current) =>
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
			patchChatMessages(queryClient, chatId, () => context.previousData);
		}
		// Invalidate messages as a safety net: the restored snapshot
		// may be missing WebSocket-delivered messages that arrived
		// during the mutation's flight time.
		void invalidateChatMessages(queryClient, chatId);
	},
	onSuccess: (
		response: TypesGen.EditChatMessageResponse,
		variables: EditChatMessageMutationArgs,
	) => {
		patchChatMessages(queryClient, chatId, (current) =>
			reconcileEditedMessageInCache({
				currentData: current,
				optimisticMessageId: variables.messageId,
				responseMessages: response.messages ?? [response.message],
				deletedMessageIds: response.deleted_message_ids,
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
		void invalidateChatEntity(queryClient, chatId);
		void invalidateChatPrompts(queryClient, chatId);
		void invalidateChatDebugRuns(queryClient, chatId);
		void invalidateChatSearches(queryClient);
	},
});

export const interruptChat = (queryClient: QueryClient, chatId: string) => ({
	mutationFn: () => API.experimental.interruptChat(chatId),
	onSuccess: () => {
		void invalidateChatDebugRuns(queryClient, chatId);
	},
});

export const compactChat = (queryClient: QueryClient, chatId: string) => ({
	mutationFn: () => API.experimental.compactChat(chatId),
	onSuccess: () => {
		// The compaction transitions the chat to running; the summary
		// rows stream in over the websocket like any other turn.
		void invalidateChatEntity(queryClient, chatId);
		void invalidateChatDebugRuns(queryClient, chatId);
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
		patchChatEntity(queryClient, chatId, (cached) =>
			cached ? { ...cached, context: updatedChat.context } : updatedChat,
		);
		const applyContext = (chat: TypesGen.Chat): TypesGen.Chat =>
			chat.id === chatId ? { ...chat, context: updatedChat.context } : chat;
		updateInfiniteChatsCache(queryClient, (chats) => {
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
	mutationFn: (queuedMessageId: number) =>
		API.experimental.deleteChatQueuedMessage(chatId, queuedMessageId),
	onSuccess: async () => {
		await invalidateChatEntity(queryClient, chatId);
		await invalidateChatMessages(queryClient, chatId);
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
	[...chatEntityKey(chatId), "diff-contents"] as const;

export const chatDiffContents = (chatId: string) => ({
	queryKey: chatDiffContentsKey(chatId),
	queryFn: () => API.experimental.getChatDiffContents(chatId),
});

const chatSystemPromptKey = [...chatConfigKey, "system-prompt"] as const;

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

const chatPlanModeInstructionsKey = [
	...chatConfigKey,
	"plan-mode-instructions",
] as const;

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
	...chatConfigKey,
	"personal-model-overrides",
	"admin",
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

const chatDebugLoggingAdminKey = [
	...chatConfigKey,
	"debug-logging",
	"admin",
] as const;
const chatDebugLoggingMeKey = [
	...chatConfigKey,
	"debug-logging",
	"me",
] as const;

export const chatDebugLogging = () => ({
	queryKey: chatDebugLoggingAdminKey,
	queryFn: () => API.experimental.getChatDebugLogging(),
});

export const userChatDebugLogging = () => ({
	queryKey: chatDebugLoggingMeKey,
	queryFn: () => API.experimental.getUserChatDebugLogging(),
});

export const updateChatDebugLogging = (queryClient: QueryClient) => ({
	mutationFn: API.experimental.updateChatDebugLogging,
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatDebugLoggingAdminKey,
		});
		await queryClient.invalidateQueries({
			queryKey: chatDebugLoggingMeKey,
		});
	},
});

export const updateUserChatDebugLogging = (queryClient: QueryClient) => ({
	mutationFn: API.experimental.updateUserChatDebugLogging,
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatDebugLoggingMeKey,
		});
	},
});
export const chatAdvisorConfigKey = [...chatConfigKey, "advisor"] as const;

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

const chatComputerUseProviderKey = [
	...chatConfigKey,
	"computer-use-provider",
] as const;

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

const chatWorkspaceTTLKey = [...chatConfigKey, "workspace-ttl"] as const;

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

const chatRetentionDaysKey = [...chatConfigKey, "retention-days"] as const;

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

const chatDebugRetentionDaysKey = [
	...chatConfigKey,
	"debug-retention-days",
] as const;

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

const chatAutoArchiveDaysKey = [...chatConfigKey, "auto-archive-days"] as const;

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

const chatUserCustomPromptKey = [...chatConfigKey, "prompt", "me"] as const;

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
	...chatConfigKey,
	"personal-model-overrides",
	"me",
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
	...chatConfigKey,
	"compaction-thresholds",
	"me",
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

export const chatModelsKey = [...chatConfigKey, "models", "catalog"] as const;

export const chatModels = () => ({
	queryKey: chatModelsKey,
	queryFn: (): Promise<TypesGen.ChatModelsResponse> =>
		API.experimental.getChatModels(),
});

export const chatModelConfigsKey = [
	...chatConfigKey,
	"models",
	"definitions",
] as const;

export const chatModelConfigs = () => ({
	queryKey: chatModelConfigsKey,
	queryFn: (): Promise<TypesGen.ChatModelConfig[]> =>
		API.experimental.getChatModelConfigs(),
});

export const userChatProviderConfigsKey = [
	"ai",
	"provider-keys",
	"me",
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
			enabled: config.provider.enabled,
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
		queryClient.invalidateQueries({ queryKey: chatModelConfigsKey }),
		queryClient.invalidateQueries({ queryKey: chatModelsKey }),
	]);
};

// Called after AI provider mutations so open model pickers refresh.
export const invalidateChatProviderDependentQueries = async (
	queryClient: QueryClient,
) => {
	await Promise.all([
		invalidateChatConfigurationQueries(queryClient),
		queryClient.invalidateQueries({ queryKey: userChatProviderConfigsKey }),
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

export const chatFileTextKey = (fileId: string) =>
	[...chatFilesKey, fileId, "text"] as const;

const GATEWAY_REQUEST_STALE_MS = 30_000;

export const chatCostTreeKey = (rootChatId: string) =>
	[...chatAnalyticsKey, "cost", "tree", rootChatId] as const;

export const chatCost = (rootChatId: string) => ({
	queryKey: chatCostTreeKey(rootChatId),
	queryFn: () => API.experimental.getChatCost(rootChatId),
	staleTime: GATEWAY_REQUEST_STALE_MS,
});

const chatModelOverrideKey = (context: TypesGen.ChatModelOverrideContext) =>
	[...chatConfigKey, "model-overrides", context] as const;

export const chatModelOverride = (
	context: TypesGen.ChatModelOverrideContext,
) => ({
	queryKey: chatModelOverrideKey(context),
	queryFn: () => API.experimental.getChatModelOverride(context),
});

export const updateChatModelOverride = (
	queryClient: QueryClient,
	context: TypesGen.ChatModelOverrideContext,
) => ({
	mutationFn: (req: TypesGen.UpdateChatModelOverrideRequest) =>
		API.experimental.updateChatModelOverride(context, req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: chatModelOverrideKey(context),
			exact: true,
		});
	},
});

// ── MCP Server Configs ───────────────────────────────────────

export const mcpServersKey = ["mcp", "servers"] as const;

export const mcpServerConfigs = () => ({
	queryKey: mcpServersKey,
	queryFn: (): Promise<TypesGen.MCPServerConfig[]> =>
		API.experimental.getMCPServerConfigs(),
});

const invalidateMCPServerConfigQueries = async (queryClient: QueryClient) => {
	await queryClient.invalidateQueries({ queryKey: mcpServersKey });
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

export const disconnectMCPServerOAuth2 = (queryClient: QueryClient) => ({
	mutationFn: (id: string) => API.experimental.disconnectMCPServerOAuth2(id),
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
	onSuccess: async (_data: unknown, { chatId }: SetChatUserRoleVariables) => {
		await invalidateChatACL(queryClient, chatId);
	},
});

export const setChatGroupRole = (queryClient: QueryClient) => ({
	mutationFn: ({ chatId, groupId, role }: SetChatGroupRoleVariables) =>
		API.experimental.updateChatACL(chatId, {
			group_roles: { [groupId]: role },
		}),
	onSuccess: async (_data: unknown, { chatId }: SetChatGroupRoleVariables) => {
		await invalidateChatACL(queryClient, chatId);
	},
});
