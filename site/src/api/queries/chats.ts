import {
	type InfiniteData,
	type QueryClient,
	queryOptions,
	replaceEqualDeep,
	type UseInfiniteQueryOptions,
} from "react-query";
import {
	API,
	type ChatPlanModeOrClear,
	type CreateChatMessageRequestWithClearablePlanMode,
} from "#/api/api";
import * as TypesGen from "#/api/typesGenerated";
import type { UsePaginatedQueryOptions } from "#/hooks/usePaginatedQuery";
import {
	projectEditedConversationIntoCache,
	reconcileEditedMessageInCache,
} from "./chatMessageEdits";

export type ChatListPRStatusFilter = "draft" | "open" | "merged" | "closed";
export type ChatListStatusFilter = "read" | "unread";

export type InfiniteChatsFilters = Readonly<{
	archived?: boolean;
	prStatuses?: readonly ChatListPRStatusFilter[];
	chatStatus?: ChatListStatusFilter;
	sources?: readonly TypesGen.ChatListSource[];
}>;

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

/** Shared ordering keeps URL serialization stable. */
const canonicalizeChatListSources = (
	sources: Iterable<unknown>,
): readonly TypesGen.ChatListSource[] => {
	const selected = new Set<unknown>(sources);
	return TypesGen.ChatListSources.filter((source) => selected.has(source));
};

/**
 * Canonical filter form shared by the sidebar list key and its request query
 * string. Equivalent inputs must produce one cache entry and one `q`, so
 * absent fields and empty arrays are dropped and array values are ordered.
 * Only `undefined` is dropped: `archived: false` is a real filter value.
 */
export const canonicalizeChatListFilters = (
	filters?: InfiniteChatsFilters,
): InfiniteChatsFilters => {
	const canonical: {
		archived?: boolean;
		prStatuses?: readonly ChatListPRStatusFilter[];
		chatStatus?: ChatListStatusFilter;
		sources?: readonly TypesGen.ChatListSource[];
	} = {};

	if (filters?.archived !== undefined) {
		canonical.archived = filters.archived;
	}
	if (filters?.chatStatus !== undefined) {
		canonical.chatStatus = filters.chatStatus;
	}
	const prStatuses = canonicalizeChatListPRStatuses(filters?.prStatuses ?? []);
	if (prStatuses.length > 0) {
		canonical.prStatuses = prStatuses;
	}
	const sources = canonicalizeChatListSources(filters?.sources ?? []);
	if (sources.length > 0) {
		canonical.sources = sources;
	}

	return canonical;
};

/**
 * Query keys for the Agents feature. Slot 1 is always a literal, so every
 * prefix is unambiguous: `list` for the sidebar's infinite filtered lists,
 * `detail` for a chat and its sub-resources, and one literal per remaining
 * resource.
 *
 * `search` and `byWorkspace` are siblings of `list`, not children. They cache
 * plain arrays and records rather than InfiniteData, so the `lists()` prefix
 * must never reach them.
 */
export const chatKeys = {
	all: ["chats"] as const,

	lists: () => [...chatKeys.all, "list"] as const,
	list: (filters?: InfiniteChatsFilters) =>
		[
			...chatKeys.lists(),
			{ filters: canonicalizeChatListFilters(filters) },
		] as const,

	details: () => [...chatKeys.all, "detail"] as const,
	detail: (chatId: string) => [...chatKeys.details(), chatId] as const,
	messages: (chatId: string) =>
		[...chatKeys.detail(chatId), "messages"] as const,
	prompts: (chatId: string) => [...chatKeys.detail(chatId), "prompts"] as const,
	queueConvergence: (chatId: string) =>
		[...chatKeys.detail(chatId), "queue-convergence"] as const,
	acl: (chatId: string) => [...chatKeys.detail(chatId), "acl"] as const,
	diffContents: (chatId: string) =>
		[...chatKeys.detail(chatId), "diff-contents"] as const,
	cost: (chatId: string) => [...chatKeys.detail(chatId), "cost"] as const,
	debugRuns: (chatId: string) =>
		[...chatKeys.detail(chatId), "debug-runs"] as const,
	debugRun: (chatId: string, runId: string) =>
		[...chatKeys.debugRuns(chatId), runId] as const,

	searches: () => [...chatKeys.all, "search"] as const,
	search: (q: string) => [...chatKeys.searches(), { q }] as const,
	byWorkspacePrefix: () => [...chatKeys.all, "by-workspace"] as const,
	byWorkspace: (workspaceIds: string[]) =>
		[...chatKeys.byWorkspacePrefix(), workspaceIds.toSorted()] as const,
	costSummary: (user: string, params?: ChatCostDateParams) =>
		[...chatKeys.all, "cost-summary", user, params ?? {}] as const,
	costUsers: (payload: PaginatedChatCostUsersPayload, pageNumber: number) =>
		[...chatKeys.all, "cost-users", payload, pageNumber] as const,
	usageLimitStatus: () => [...chatKeys.all, "usage-limit-status"] as const,
	usageLimitConfig: () => [...chatKeys.all, "usage-limit-config"] as const,
	adminPersonalModelOverrides: () =>
		[...chatKeys.all, "admin-personal-model-overrides"] as const,
	userPersonalModelOverrides: () =>
		[...chatKeys.all, "user-personal-model-overrides"] as const,
};

export const chatsByWorkspace = (workspaceIds: string[]) => {
	const sorted = workspaceIds.toSorted();
	return {
		queryKey: chatKeys.byWorkspace(sorted),
		queryFn: () => API.experimental.getChatsByWorkspace(sorted),
		enabled: workspaceIds.length > 0,
	};
};

/**
 * Updates a single chat inside every page of the infinite chats query
 * cache. Use this instead of writing one list key directly, which would
 * leave the other cached filter combinations stale.
 */
export const updateInfiniteChatsCache = (
	queryClient: QueryClient,
	updater: (chats: TypesGen.Chat[]) => TypesGen.Chat[],
) => {
	// Update ALL infinite chat queries regardless of their filter opts.
	queryClient.setQueriesData<InfiniteChatsCacheData>(
		{ queryKey: chatKeys.lists() },
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
 * Reads the flat list of chats from the first matching infinite query
 * in the cache. Returns undefined when no data is cached yet.
 */
export const readInfiniteChatsCache = (
	queryClient: QueryClient,
): TypesGen.Chat[] | undefined => {
	const queries = queryClient.getQueriesData<InfiniteChatsCacheData>({
		queryKey: chatKeys.lists(),
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

const isPlainObject = (value: unknown): value is Record<string, unknown> =>
	typeof value === "object" && value !== null && !Array.isArray(value);

/**
 * Inverse of chatKeys.list, which appends a single { filters } wrapper to the
 * chatKeys.lists() prefix. The length and shape checks narrow the readonly
 * unknown[] the query cache hands back, and reject any other list-prefixed
 * key. Returns the canonical filters a cached list was fetched with, so
 * event handlers can decide which variants an event can change membership
 * of instead of invalidating every list.
 */
export const listFiltersFromKey = (
	queryKey: readonly unknown[],
): InfiniteChatsFilters | undefined => {
	if (queryKey.length !== chatKeys.lists().length + 1) {
		return undefined;
	}
	const wrapper = queryKey[chatKeys.lists().length];
	if (!isPlainObject(wrapper) || !isPlainObject(wrapper.filters)) {
		return undefined;
	}
	const { archived, chatStatus, prStatuses, sources } = wrapper.filters;
	return canonicalizeChatListFilters({
		archived: typeof archived === "boolean" ? archived : undefined,
		chatStatus:
			chatStatus === "read" || chatStatus === "unread" ? chatStatus : undefined,
		prStatuses: Array.isArray(prStatuses)
			? canonicalizeChatListPRStatuses(prStatuses)
			: undefined,
		sources: Array.isArray(sources)
			? canonicalizeChatListSources(sources)
			: undefined,
	});
};

const archivedFilterFromListKey = (
	queryKey: readonly unknown[],
): boolean | undefined => listFiltersFromKey(queryKey)?.archived;

const isInfiniteChatsCacheData = (
	data: unknown,
): data is InfiniteChatsCacheData => {
	if (!data || typeof data !== "object") {
		return false;
	}
	const maybeData = data as { pages?: unknown; pageParams?: unknown };
	return Array.isArray(maybeData.pages) && Array.isArray(maybeData.pageParams);
};

/**
 * Applies a patch to one chat in every cache layer that can hold it: the
 * detail cache, every cached list variant, and any parent's embedded
 * `children`. Writing a single layer leaves the others showing the old
 * value until a refetch.
 *
 * `patch` must return the chat it was given when it changes nothing, so
 * untouched caches keep their reference and skip a rerender.
 */
export const patchChatEverywhere = (
	queryClient: QueryClient,
	chatId: string,
	patch: (chat: TypesGen.Chat) => TypesGen.Chat,
) => {
	queryClient.setQueryData<TypesGen.Chat | undefined>(
		chatKeys.detail(chatId),
		(chat) => (chat ? patch(chat) : chat),
	);
	updateInfiniteChatsCache(queryClient, (chats) => {
		let changed = false;
		const next = chats.map((chat) => {
			if (chat.id !== chatId) {
				return chat;
			}
			const patched = patch(chat);
			if (patched !== chat) {
				changed = true;
			}
			return patched;
		});
		return changed ? next : chats;
	});
	updateChildInParentCache(queryClient, patch, chatId);
};

/** Matches only queries that already hold data. */
const hasLoadedData = (query: { state: { data: unknown } }): boolean =>
	query.state.data !== undefined;

// ---------------------------------------------------------------------------
// Chat status ordering
//
// Three writers change a chat's status: the per-chat stream socket, the global
// watch socket, and every REST payload that carries a chat. `snapshot_version`
// is the only key they share that orders those writes: the server bumps it
// while holding the chat row lock, so version order is commit order.
// `updated_at` cannot order status, it comes from transaction_timestamp() and
// is evaluated before the lock, so it can invert against commit order.
//
// The cached `snapshot_version` is the version of the STATUS the entry holds,
// not of the whole payload: a rejected payload's other fields are still
// adopted, so advancing the version for them would strand the older status
// with no way to correct it.
// ---------------------------------------------------------------------------

/**
 * Coerces an absent `snapshot_version` to the zero version. Chats built before
 * the field existed, and fixtures that omit it, still have to compare: every
 * comparison against a raw `undefined` is false, which would silently reject
 * every status write instead of accepting it.
 */
const chatSnapshotVersion = (version?: number): number => version ?? 0;

/**
 * Provenance of a status the client wrote ahead of the server. `token`
 * identifies the write so its own rollback can tell whether it is still the
 * latest one, and `previousStatus` is the status to restore, carried per cache
 * entry because layers can hold different statuses.
 */
type OptimisticChatStatus = {
	readonly token: number;
	readonly previousStatus: TypesGen.ChatStatus;
};

/**
 * A chat as held in the query cache. `__optimistic` is client-only provenance:
 * it is colocated with the entry so it is garbage collected with it, and it is
 * cleared as soon as a strictly newer server snapshot lands. It never appears
 * on a payload from the server.
 */
type CachedChat = TypesGen.Chat & {
	readonly __optimistic?: OptimisticChatStatus;
};

/**
 * Narrows a cached payload to a chat record. The detail key is also the prefix
 * of every per-chat sub-resource key, and `structuralSharing` is typed against
 * `unknown`, so the shape has to be checked before a merge or a patch runs.
 */
const isCachedChat = (value: unknown): value is CachedChat =>
	isPlainObject(value) &&
	typeof value.id === "string" &&
	typeof value.status === "string";

type WritableCachedChat = TypesGen.Chat & {
	__optimistic?: OptimisticChatStatus;
};

/**
 * Orders a server-backed status against the one already cached.
 *
 * A server payload owns the status only when its `snapshot_version` is
 * STRICTLY greater. Equal versions describe the same committed snapshot, so a
 * duplicate is a no-op and an optimistic status pinned at that version
 * survives until its request settles or rolls back. Older payloads lose.
 *
 * Polarity, spelled out because the guard this replaces reads the other way
 * round: the old code accepted a watched status when
 * `compareUpdatedAtInstants(cached, watched) <= 0`, that is when the CACHED
 * instant was not newer than the incoming one. Here the incoming version must
 * be greater than the cached one, which is the same direction with the equal
 * case moved from accept to reject.
 */
export const shouldApplyServerChatStatus = (
	cachedVersion: number | undefined,
	incomingVersion: number | undefined,
): boolean =>
	chatSnapshotVersion(incomingVersion) > chatSnapshotVersion(cachedVersion);

/**
 * Builds the entry for an accepted server status. A strictly newer server
 * snapshot supersedes any optimistic status, so the provenance token goes with
 * it.
 */
const withServerChatStatus = (
	chat: CachedChat,
	status: TypesGen.ChatStatus,
	snapshotVersion: number,
): CachedChat => {
	const next: WritableCachedChat = {
		...chat,
		status,
		snapshot_version: snapshotVersion,
	};
	delete next.__optimistic;
	return next;
};

/**
 * Commit-time merge for a single chat, run from `structuralSharing` so it also
 * covers the payloads react-query writes without asking: a resolving `queryFn`
 * calls setData unconditionally, so a REST response issued before a socket
 * status would otherwise regress it.
 *
 * Client writes are passed through: they carry the optimistic token and their
 * writer already applied the ordering rules.
 */
const mergeCommittedChatStatus = (
	prev: CachedChat | undefined,
	next: CachedChat,
): CachedChat => {
	if (!prev || prev === next) {
		return next;
	}
	if (next.__optimistic !== undefined) {
		return next;
	}
	if (
		shouldApplyServerChatStatus(prev.snapshot_version, next.snapshot_version)
	) {
		return next;
	}
	// Older or duplicate snapshot: keep the cached status, its version, and any
	// optimistic provenance, take everything else from the fresh payload.
	const preserved: CachedChat = {
		...next,
		status: prev.status,
		snapshot_version: prev.snapshot_version,
	};
	return prev.__optimistic
		? { ...preserved, __optimistic: prev.__optimistic }
		: preserved;
};

/**
 * Commit merge for the detail cache, covering the record and the children it
 * embeds. A parent's detail payload carries its children's statuses, so a
 * parent fetch issued before a child's status event would otherwise regress
 * that child.
 *
 * A custom `structuralSharing` REPLACES the default one, so it has to call
 * `replaceEqualDeep` itself. Skipping that would hand every consumer a brand
 * new Chat object on each fetch.
 */
export const mergeCommittedChatDetail = (
	prev: CachedChat | undefined,
	next: CachedChat,
): CachedChat => replaceEqualDeep(prev, mergeCommittedChatRecord(prev, next));

const mergeCommittedChatRecord = (
	prev: CachedChat | undefined,
	next: CachedChat,
): CachedChat => {
	const merged = mergeCommittedChatStatus(prev, next);
	const nextChildren = next.children;
	if (!nextChildren?.length) {
		return merged;
	}
	const cachedChildren = new Map(
		(prev?.children ?? []).map((child) => [child.id, child]),
	);
	if (cachedChildren.size === 0) {
		return merged;
	}
	let changed = false;
	const children = nextChildren.map((child) => {
		const mergedChild = mergeCommittedChatStatus(
			cachedChildren.get(child.id),
			child,
		);
		if (mergedChild !== child) {
			changed = true;
		}
		return mergedChild;
	});
	return changed ? { ...merged, children } : merged;
};

// react-query types structuralSharing as (unknown, unknown) => unknown, so the
// option is a thin adapter over the typed merge.
const chatDetailStructuralSharing = (prev: unknown, next: unknown): unknown =>
	isCachedChat(next)
		? mergeCommittedChatDetail(isCachedChat(prev) ? prev : undefined, next)
		: replaceEqualDeep(prev, next);

/**
 * Picks which of two cached copies of the same chat holds the status a
 * committed payload has to be ordered against: the strictly newer version,
 * and on a tie the optimistic one, whose status is the one on screen.
 */
const preferCachedChatStatus = (a: CachedChat, b: CachedChat): CachedChat => {
	if (shouldApplyServerChatStatus(a.snapshot_version, b.snapshot_version)) {
		return b;
	}
	if (shouldApplyServerChatStatus(b.snapshot_version, a.snapshot_version)) {
		return a;
	}
	return b.__optimistic && !a.__optimistic ? b : a;
};

/**
 * Indexes cached list rows and embedded children by id. Pagination can place
 * the same chat on more than one loaded page, so the copy with the most
 * advanced status wins rather than whichever page is scanned last.
 */
const indexChatsById = (
	data: InfiniteChatsCacheData | undefined,
): Map<string, CachedChat> => {
	const byId = new Map<string, CachedChat>();
	if (!isInfiniteChatsCacheData(data)) {
		return byId;
	}
	const keepNewest = (chat: CachedChat) => {
		const existing = byId.get(chat.id);
		if (!existing || preferCachedChatStatus(existing, chat) === chat) {
			byId.set(chat.id, chat);
		}
	};
	for (const page of data.pages) {
		for (const chat of page) {
			keepNewest(chat);
			for (const child of chat.children ?? []) {
				keepNewest(child);
			}
		}
	}
	return byId;
};

/**
 * The list version of {@link mergeCommittedChatDetail}. A list refetch also
 * commits unconditionally, and its rows and embedded children hold the status
 * a shared viewer's sidebar renders, so they need the same ordering the detail
 * cache gets.
 */
export const mergeCommittedChatList = (
	prev: InfiniteChatsCacheData | undefined,
	next: InfiniteChatsCacheData,
): InfiniteChatsCacheData => {
	if (!prev || prev === next || !isInfiniteChatsCacheData(next)) {
		return replaceEqualDeep(prev, next);
	}
	const cachedById = indexChatsById(prev);
	if (cachedById.size === 0) {
		return replaceEqualDeep(prev, next);
	}
	const mergeRow = (chat: CachedChat): CachedChat =>
		mergeCommittedChatStatus(cachedById.get(chat.id), chat);
	const merged: InfiniteChatsCacheData = {
		...next,
		pages: next.pages.map((page) =>
			page.map((chat) => {
				const mergedChat = mergeRow(chat);
				if (!chat.children?.length) {
					return mergedChat;
				}
				return { ...mergedChat, children: chat.children.map(mergeRow) };
			}),
		),
	};
	return replaceEqualDeep(prev, merged);
};

const chatListStructuralSharing = (prev: unknown, next: unknown): unknown =>
	isInfiniteChatsCacheData(next)
		? mergeCommittedChatList(
				isInfiniteChatsCacheData(prev) ? prev : undefined,
				next,
			)
		: replaceEqualDeep(prev, next);

const isCachedChatArray = (value: unknown): value is CachedChat[] =>
	Array.isArray(value) && value.every(isCachedChat);

/**
 * The search version of {@link mergeCommittedChatDetail}. Search results are a
 * plain array under a sibling key, and the sidebar search panel renders the
 * status each row carries, so a search response has to be ordered too.
 */
export const mergeCommittedChatSearch = (
	prev: readonly CachedChat[] | undefined,
	next: readonly CachedChat[],
): readonly CachedChat[] => {
	if (!prev || prev === next || prev.length === 0) {
		return replaceEqualDeep(prev, next);
	}
	const cachedById = new Map(prev.map((chat) => [chat.id, chat]));
	return replaceEqualDeep(
		prev,
		next.map((chat) => mergeCommittedChatStatus(cachedById.get(chat.id), chat)),
	);
};

const chatSearchStructuralSharing = (prev: unknown, next: unknown): unknown =>
	isCachedChatArray(next)
		? mergeCommittedChatSearch(isCachedChatArray(prev) ? prev : undefined, next)
		: replaceEqualDeep(prev, next);

/** Applies `patch` to a chat held by a detail record, root or embedded child. */
const patchChatInRecord = (
	record: CachedChat,
	chatId: string,
	patch: (chat: CachedChat) => CachedChat,
): CachedChat => {
	if (record.id === chatId) {
		return patch(record);
	}
	if (!record.children?.length) {
		return record;
	}
	let changed = false;
	const children = record.children.map((child) => {
		if (child.id !== chatId) {
			return child;
		}
		const patched = patch(child);
		if (patched !== child) {
			changed = true;
		}
		return patched;
	});
	return changed ? { ...record, children } : record;
};

/**
 * Applies `patch` to the chat in every cache layer that can hold it, keeping
 * each matched query's own `dataUpdatedAt`. The layers are the chat's own
 * detail record, any loaded parent detail that embeds it as a child, every
 * cached list variant's rows and embedded children, and every cached search
 * result.
 *
 * The freshness stamp is preserved because the writers that use this push a
 * subset of a chat's fields over a socket. Refreshing the stamp would postpone
 * the refetch that catches up the fields the socket does not push. Mutation
 * writes want the opposite and use {@link patchChatEverywhere}.
 */
const patchChatEverywherePreservingFreshness = (
	queryClient: QueryClient,
	chatId: string,
	patch: (chat: CachedChat) => CachedChat,
): void => {
	// The details() prefix also matches every per-chat sub-resource key, so
	// both the key length and the payload shape are checked.
	const detailKeyLength = chatKeys.details().length + 1;
	const detailQueries = queryClient.getQueriesData({
		queryKey: chatKeys.details(),
	});
	for (const [queryKey, data] of detailQueries) {
		if (queryKey.length !== detailKeyLength || !isCachedChat(data)) {
			continue;
		}
		if (data.id !== chatId && !data.children?.some((c) => c.id === chatId)) {
			continue;
		}
		queryClient.setQueryData<CachedChat | undefined>(
			queryKey,
			(record) => (record ? patchChatInRecord(record, chatId, patch) : record),
			{ updatedAt: queryClient.getQueryState(queryKey)?.dataUpdatedAt },
		);
	}

	const listQueries = queryClient.getQueriesData<InfiniteChatsCacheData>({
		queryKey: chatKeys.lists(),
	});
	for (const [queryKey, data] of listQueries) {
		if (!isInfiniteChatsCacheData(data)) {
			continue;
		}
		queryClient.setQueryData<InfiniteChatsCacheData>(
			queryKey,
			(prev) => {
				if (!isInfiniteChatsCacheData(prev)) {
					return prev;
				}
				let changed = false;
				const pages = prev.pages.map((page) => {
					let pageChanged = false;
					const nextPage = page.map((chat) => {
						const patched = patchChatInRecord(chat, chatId, patch);
						if (patched !== chat) {
							pageChanged = true;
						}
						return patched;
					});
					if (!pageChanged) {
						return page;
					}
					changed = true;
					return nextPage;
				});
				return changed ? { ...prev, pages } : prev;
			},
			{ updatedAt: queryClient.getQueryState(queryKey)?.dataUpdatedAt },
		);
	}

	const searchQueries = queryClient.getQueriesData({
		queryKey: chatKeys.searches(),
	});
	for (const [queryKey, data] of searchQueries) {
		if (!isCachedChatArray(data)) {
			continue;
		}
		queryClient.setQueryData<CachedChat[]>(
			queryKey,
			(prev) => {
				if (!isCachedChatArray(prev)) {
					return prev;
				}
				let changed = false;
				const next = prev.map((chat) => {
					if (chat.id !== chatId) {
						return chat;
					}
					const patched = patch(chat);
					if (patched !== chat) {
						changed = true;
					}
					return patched;
				});
				return changed ? next : prev;
			},
			{ updatedAt: queryClient.getQueryState(queryKey)?.dataUpdatedAt },
		);
	}
};

/**
 * Writes a server-reported status into every cache layer, per layer ordered by
 * `snapshot_version`. Status and version are written together so the next
 * comparison is against the key of the status actually held.
 *
 * Returns whether the chat's own detail entry took the status. That entry is
 * the one the open chat renders from, so it decides whether the caller may run
 * the side effects that belong to an authoritative status: a superseded or
 * versionless payload reports false and must leave stream state alone.
 */
export const applyServerChatStatusToCaches = (
	queryClient: QueryClient,
	chatId: string,
	status: TypesGen.ChatStatus,
	snapshotVersion: number | undefined,
): boolean => {
	const cachedDetail = queryClient.getQueryData<CachedChat>(
		chatKeys.detail(chatId),
	);
	const accepted =
		cachedDetail !== undefined &&
		shouldApplyServerChatStatus(cachedDetail.snapshot_version, snapshotVersion);
	patchChatEverywherePreservingFreshness(queryClient, chatId, (chat) =>
		shouldApplyServerChatStatus(chat.snapshot_version, snapshotVersion)
			? withServerChatStatus(chat, status, chatSnapshotVersion(snapshotVersion))
			: chat,
	);
	return accepted;
};

/** Reads the status the cache holds for a chat, or null when it holds none. */
export const readCachedChatStatus = (
	queryClient: QueryClient,
	chatId: string | undefined,
): TypesGen.ChatStatus | null => {
	if (!chatId) {
		return null;
	}
	return (
		queryClient.getQueryData<CachedChat>(chatKeys.detail(chatId))?.status ??
		null
	);
};

/** Reads the snapshot version the cache holds for a chat's status. */
export const readCachedChatSnapshotVersion = (
	queryClient: QueryClient,
	chatId: string | undefined,
): number => {
	if (!chatId) {
		return 0;
	}
	return chatSnapshotVersion(
		queryClient.getQueryData<CachedChat>(chatKeys.detail(chatId))
			?.snapshot_version,
	);
};

let lastOptimisticChatStatusToken = 0;

/**
 * Writes a status the client expects the server to reach, so the running
 * indicator appears without waiting for a round trip.
 *
 * The write deliberately does NOT advance `snapshot_version`: no server
 * snapshot backs it, and advancing would make the send-path fence read its own
 * write as a server response. Its provenance token lets the matching rollback
 * tell whether it is still the write in effect. Returns the token.
 */
export const writeOptimisticChatStatus = (
	queryClient: QueryClient,
	chatId: string,
	status: TypesGen.ChatStatus,
): number => {
	lastOptimisticChatStatusToken += 1;
	const token = lastOptimisticChatStatusToken;
	patchChatEverywherePreservingFreshness(queryClient, chatId, (chat) => ({
		...chat,
		status,
		// Keep the status the server last reported, not the one an earlier
		// optimistic write put there, so consecutive writes roll back to a
		// server-backed value.
		__optimistic: {
			token,
			previousStatus: chat.__optimistic?.previousStatus ?? chat.status,
		},
	}));
	return token;
};

/**
 * Undoes {@link writeOptimisticChatStatus} when its request failed. Restores
 * only where that exact write is still the one in effect: a newer optimistic
 * write replaced the token, and a strictly newer server status dropped it, and
 * in both cases the cached status is no longer this write's to undo.
 *
 * The token is left in place. It is inert once the status matches the restored
 * value again, and it keeps a same-version server payload from re-applying the
 * status this rollback just took back.
 */
export const rollbackOptimisticChatStatus = (
	queryClient: QueryClient,
	chatId: string,
	token: number,
): void => {
	patchChatEverywherePreservingFreshness(queryClient, chatId, (chat) => {
		const optimistic = chat.__optimistic;
		if (!optimistic || optimistic.token !== token) {
			return chat;
		}
		if (chat.status === optimistic.previousStatus) {
			return chat;
		}
		return { ...chat, status: optimistic.previousStatus };
	});
};

/**
 * Runs a detail-query cancellation without losing the status the cache holds.
 *
 * Cancelling a fetch reverts the query to the state it had when that fetch
 * started, which silently undoes any status the socket wrote while the fetch
 * was in flight. The status is re-applied afterwards through the same ordered
 * write the socket uses, so it is a no-op when no revert happened: the
 * re-applied version is then equal to the cached one, and equal versions lose.
 *
 * An optimistic status pinned at the cached version is not restored, because
 * it is not server-backed and shares the version the revert restored. It
 * self-heals when the request that wrote it reports its own status.
 */
export const cancelChatDetailPreservingStatus = async (
	queryClient: QueryClient,
	chatId: string,
	cancel: () => Promise<void>,
): Promise<void> => {
	const cached = queryClient.getQueryData<CachedChat>(chatKeys.detail(chatId));
	await cancel();
	if (!cached) {
		return;
	}
	applyServerChatStatusToCaches(
		queryClient,
		chatId,
		cached.status,
		cached.snapshot_version,
	);
};

/**
 * Cancels the in-flight sidebar list and chat detail fetches that a
 * mutation's optimistic write would otherwise race, in parallel.
 *
 * Unlike cancelChatListRefetches this deliberately cancels pagination
 * fetches too: fetchNextPage captured its page snapshot before the
 * mutation, so letting it land would overwrite the optimistic update.
 * Queries that have never loaded are left alone, because reverting a
 * first-ever fetch wedges the query at pending/idle with no data and no
 * automatic recovery.
 */
export const cancelChatMutationRefetches = (
	queryClient: QueryClient,
	chatId: string,
) =>
	Promise.all([
		queryClient.cancelQueries({
			queryKey: chatKeys.lists(),
			predicate: hasLoadedData,
		}),
		cancelChatDetailPreservingStatus(queryClient, chatId, () =>
			queryClient.cancelQueries({
				queryKey: chatKeys.detail(chatId),
				exact: true,
				predicate: hasLoadedData,
			}),
		),
	]);

/**
 * Reads a chat from whichever cache layer holds it: the detail cache
 * first, then every cached list variant, then parents' embedded
 * children. Sidebar mutations routinely run against a chat the user
 * never opened, so the detail cache alone cannot supply the prior field
 * value an inverse-patch rollback needs.
 */
export const findChatInCaches = (
	queryClient: QueryClient,
	chatId: string,
): TypesGen.Chat | undefined => {
	const cachedDetail = queryClient.getQueryData<TypesGen.Chat>(
		chatKeys.detail(chatId),
	);
	if (cachedDetail) {
		return cachedDetail;
	}

	const queries = queryClient.getQueriesData<InfiniteChatsCacheData>({
		queryKey: chatKeys.lists(),
	});

	for (const [, data] of queries) {
		if (!isInfiniteChatsCacheData(data)) {
			continue;
		}
		for (const page of data.pages) {
			for (const chat of page) {
				if (chat.id === chatId) {
					return chat;
				}
			}
		}
	}

	// Children are searched last: a top-level row is the authoritative
	// summary, an embedded child snapshot only a fallback.
	for (const [, data] of queries) {
		if (!isInfiniteChatsCacheData(data)) {
			continue;
		}
		for (const page of data.pages) {
			for (const chat of page) {
				const child = chat.children?.find((c) => c.id === chatId);
				if (child) {
					return child;
				}
			}
		}
	}

	return undefined;
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
 * Applies an accepted archive state to loaded sidebar and detail caches.
 * Removes the chat from any filtered list whose archived filter conflicts
 * with the new state, and resets pin_order to 0 when archiving.
 */
export const applyChatArchiveStateToCaches = (
	queryClient: QueryClient,
	chatId: string,
	archived: boolean,
) => {
	queryClient.setQueryData<TypesGen.Chat | undefined>(
		chatKeys.detail(chatId),
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
		queryKey: chatKeys.lists(),
	});

	for (const [queryKey, data] of queries) {
		if (!isInfiniteChatsCacheData(data)) {
			continue;
		}
		const archivedFilter = archivedFilterFromListKey(queryKey);
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
};

/**
 * Applies the optimistic read state for the chat the user just opened.
 * A list filtered to unread chats no longer contains the chat, so it is
 * dropped from those variants rather than patched; every other loaded
 * variant keeps the chat with has_unread cleared. Deliberately does not
 * invalidate: a refetch racing the server-side read marker returns
 * has_unread:true and undoes the clear. The finite list staleTime and the
 * focus refetch reconcile a failed server write instead.
 */
export const clearChatUnreadInCaches = (
	queryClient: QueryClient,
	chatId: string,
) => {
	const queries = queryClient.getQueriesData<InfiniteChatsCacheData>({
		queryKey: chatKeys.lists(),
	});

	for (const [queryKey, data] of queries) {
		if (!isInfiniteChatsCacheData(data)) {
			continue;
		}
		const isUnreadOnlyList =
			listFiltersFromKey(queryKey)?.chatStatus === "unread";
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
					if (isUnreadOnlyList) {
						pageChanged = true;
						continue;
					}
					if (!chat.has_unread) {
						nextPage.push(chat);
						continue;
					}
					pageChanged = true;
					nextPage.push({ ...chat, has_unread: false });
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

/**
 * Mirrors the sidebar ordering the list endpoint applies
 * (`(pin_order > 0) DESC, -pin_order DESC, updated_at DESC, id DESC`,
 * GetChats in chats.sql). WebSocket merges patch a cached chat in place,
 * so without this the sidebar keeps the ordering of the last fetch until
 * a refetch reorders it.
 */
export const compareSidebarChats = (
	a: TypesGen.Chat,
	b: TypesGen.Chat,
): number => {
	const aPinned = a.pin_order > 0 ? 1 : 0;
	const bPinned = b.pin_order > 0 ? 1 : 0;
	if (aPinned !== bPinned) {
		return bPinned - aPinned;
	}
	if (aPinned === 1 && a.pin_order !== b.pin_order) {
		// -pin_order DESC puts the lowest pin_order first.
		return a.pin_order - b.pin_order;
	}
	const byUpdatedAt = compareUpdatedAtInstants(b.updated_at, a.updated_at);
	if (byUpdatedAt !== 0) {
		return byUpdatedAt;
	}
	if (a.id === b.id) {
		return 0;
	}
	return a.id < b.id ? 1 : -1;
};

/**
 * Flattens the paginated sidebar list and restores the server ordering at
 * the render layer. Sorting the cache instead would either be limited to a
 * single page (updateInfiniteChatsCache maps per page) or desync the client
 * page boundaries from the server offset windows. Only loaded chats can be
 * promoted; a chat past the last loaded page enters the window on the next
 * refetch.
 */
export const selectSortedChatList = (
	data: InfiniteData<TypesGen.Chat[]>,
): TypesGen.Chat[] => data.pages.flat().toSorted(compareSidebarChats);

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
 * Status is the exception: it is ordered by snapshot_version, the only key
 * that agrees with commit order.
 */
export const mergeWatchedChatSummary = (
	cachedChat: CachedChat,
	watchedChat: TypesGen.Chat,
	{ eventKind, activeChatId }: MergeWatchedChatOptions,
): TypesGen.Chat => {
	const isTitleEvent = eventKind === "title_change";
	const isStatusEvent = eventKind === "status_change";
	const isSummaryEvent = eventKind === "summary_change";
	const isChatSummaryEvent = eventKind === "chat_summary_change";
	const isDiffStatusEvent = eventKind === "diff_status_change";
	const isContextDirtyEvent = eventKind === "context_dirty";
	const updatedAtComparison = compareUpdatedAtInstants(
		cachedChat.updated_at,
		watchedChat.updated_at,
	);
	const isFreshEnough = updatedAtComparison <= 0;
	// Status keeps its own guard: updated_at is not monotonic, so it cannot
	// order a status against one the per-chat socket already applied. Every
	// event kind publishes a coherent chat row, so any of them may carry the
	// status forward as long as the version comparator accepts it.
	const appliesStatus = shouldApplyServerChatStatus(
		cachedChat.snapshot_version,
		watchedChat.snapshot_version,
	);
	const nextStatus = appliesStatus ? watchedChat.status : cachedChat.status;
	// The version travels with the status it labels. A payload that only
	// advances the version still has to advance the cached one, otherwise a
	// later delayed event compares against a key that was already superseded.
	const nextSnapshotVersion = appliesStatus
		? chatSnapshotVersion(watchedChat.snapshot_version)
		: cachedChat.snapshot_version;
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
	// Unread is a property of the status transition the server announced, not
	// of a status picked up in passing, so it stays scoped to status_change.
	const nextHasUnread =
		isStatusEvent && appliesStatus && watchedChat.id !== activeChatId
			? true
			: cachedChat.has_unread;
	const nextUpdatedAt =
		updatedAtComparison > 0 ? cachedChat.updated_at : watchedChat.updated_at;

	// Keep updated_at in the no-op guard. This gives up the old streaming
	// rerender shortcut so later stale events cannot pass isFreshEnough
	// against a timestamp that should already have been superseded.
	if (
		nextStatus === cachedChat.status &&
		nextSnapshotVersion === cachedChat.snapshot_version &&
		nextTitle === cachedChat.title &&
		diffStatusEqual(nextDiffStatus, cachedChat.diff_status) &&
		nextWorkspaceId === cachedChat.workspace_id &&
		nextBuildId === cachedChat.build_id &&
		nextLastModelConfigId === cachedChat.last_model_config_id &&
		nextLastTurnSummary === cachedChat.last_turn_summary &&
		nextSummary === cachedChat.summary &&
		nextHasUnread === cachedChat.has_unread &&
		nextUpdatedAt === cachedChat.updated_at &&
		nextContext === cachedChat.context
	) {
		return cachedChat;
	}

	const merged: WritableCachedChat = {
		...cachedChat,
		status: nextStatus,
		snapshot_version: nextSnapshotVersion,
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
	};
	if (appliesStatus) {
		// A strictly newer server status supersedes an optimistic one.
		delete merged.__optimistic;
	}
	return merged;
};

/**
 * Applies the same event-scoped merge and stale guard to every cache layer
 * that can hold the chat: list rows, embedded children, detail records, and
 * search results. Each write keeps its query's own `dataUpdatedAt`, because a
 * watch event pushes a subset of the fields and must not postpone the refetch
 * that catches up the rest.
 */
export const mergeWatchedChatIntoCaches = (
	queryClient: QueryClient,
	watchedChat: TypesGen.Chat,
	options: MergeWatchedChatOptions,
) => {
	patchChatEverywherePreservingFreshness(
		queryClient,
		watchedChat.id,
		(cachedChat) => mergeWatchedChatSummary(cachedChat, watchedChat, options),
	);
};

const getNextOptimisticPinOrder = (queryClient: QueryClient): number => {
	let maxPinOrder = 0;
	const queries = queryClient.getQueriesData<InfiniteChatsCacheData>({
		queryKey: chatKeys.lists(),
	});

	for (const [, data] of queries) {
		if (!data) {
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

export const invalidateChatListQueries = (queryClient: QueryClient) => {
	return queryClient.invalidateQueries({
		queryKey: chatKeys.lists(),
	});
};

/**
 * Invalidates only the list variants filtered by pull request status.
 * A diff_status_change can move a chat in or out of those result sets;
 * no other list filter depends on diff status, and the chat's own
 * diff_status field is merged into every cached list from the event
 * payload.
 */
export const invalidatePRStatusChatListQueries = (queryClient: QueryClient) => {
	return queryClient.invalidateQueries({
		queryKey: chatKeys.lists(),
		predicate: (query) => {
			const prStatuses = listFiltersFromKey(query.queryKey)?.prStatuses;
			return prStatuses !== undefined && prStatuses.length > 0;
		},
	});
};

/** Window a burst of list invalidations collapses into. */
export const CHAT_LIST_INVALIDATE_COALESCE_MS = 500;

/**
 * Collapses a burst of chat list invalidations into a single trailing
 * refetch. A running agent emits status events continuously and each one
 * bumps updated_at, which every list sorts by, so the list still has to be
 * invalidated; the window bounds that to one refetch per interval instead
 * of one per event. The first request opens the window and later requests
 * ride along, so an invalidation always lands within the window rather
 * than being pushed back indefinitely by a sustained burst.
 */
export const createCoalescedChatListInvalidator = (
	queryClient: QueryClient,
	delayMs: number = CHAT_LIST_INVALIDATE_COALESCE_MS,
) => {
	let timeout: ReturnType<typeof setTimeout> | undefined;
	return {
		schedule: () => {
			if (timeout !== undefined) {
				return;
			}
			timeout = setTimeout(() => {
				timeout = undefined;
				void invalidateChatListQueries(queryClient);
			}, delayMs);
		},
		cancel: () => {
			if (timeout === undefined) {
				return;
			}
			clearTimeout(timeout);
			timeout = undefined;
		},
	};
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
 * Mutation onMutate handlers should keep cancelling the whole
 * chatKeys.lists() prefix instead: mutations are infrequent
 * and must cancel pagination fetches to protect optimistic
 * updates from being overwritten by the oldPages snapshot
 * that fetchNextPage captured before the mutation.
 */
export const cancelChatListRefetches = (queryClient: QueryClient) => {
	return queryClient.cancelQueries({
		queryKey: chatKeys.lists(),
		predicate: isChatListRefetch,
	});
};

const DEFAULT_CHAT_PAGE_LIMIT = 50;
export const CHAT_SEARCH_LIMIT = 50;

/**
 * How long a fetched chat summary stays fresh. The watch socket pushes
 * status, title, summary, and diff status, but PATCH-only fields
 * (pin_order, plan_mode, labels, mcp_server_ids, workspace_id) and
 * has_unread have no push path, so these caches must still be
 * refetchable. Two minutes is short enough that a rename or pin from
 * another tab converges on the next window focus, and long enough that
 * mounting the page or navigating between chats no longer refetches every
 * loaded page.
 */
export const CHAT_SUMMARY_STALE_MS = 2 * 60 * 1000;

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
	const limit = DEFAULT_CHAT_PAGE_LIMIT;
	// One canonical object feeds both the key and the request, so equivalent
	// filter inputs can never share a cache entry while issuing different `q`.
	const canonicalFilters = canonicalizeChatListFilters(filters);
	const q = getInfiniteChatsQueryString(canonicalFilters);

	return {
		queryKey: chatKeys.list(canonicalFilters),
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
		// refetchOnWindowFocus only fires for a stale query, so a finite
		// staleTime turns focus into the bounded catch-up path for the
		// fields the watch socket does not push.
		staleTime: CHAT_SUMMARY_STALE_MS,
		select: selectSortedChatList,
		// Rows and embedded children carry status too, and a list refetch
		// commits as unconditionally as the detail one does.
		structuralSharing: chatListStructuralSharing,
		retry: 3,
	} satisfies UseInfiniteQueryOptions<TypesGen.Chat[]>;
};

export const chatSearch = (q: string) =>
	queryOptions({
		queryKey: chatKeys.search(q),
		queryFn: () =>
			API.experimental.getChats({
				limit: CHAT_SEARCH_LIMIT,
				q,
			}),
		// Search rows carry status too, and the response commits as
		// unconditionally as the detail one does.
		structuralSharing: chatSearchStructuralSharing,
	});

export const chat = (chatId: string) => ({
	queryKey: chatKeys.detail(chatId),
	queryFn: () => API.experimental.getChat(chatId),
	// Same freshness policy as the sidebar list: the open chat's detail
	// carries the same non-pushed fields, plus files, context, and
	// children that the watch payloads do not fully cover.
	staleTime: CHAT_SUMMARY_STALE_MS,
	refetchOnWindowFocus: true,
	// A resolving fetch commits unconditionally, so ordering the status it
	// carries against the cached one has to happen at commit time.
	structuralSharing: chatDetailStructuralSharing,
});

export const chatACL = (chatId: string) => ({
	queryKey: chatKeys.acl(chatId),
	queryFn: () => API.experimental.getChatACL(chatId),
});

const MESSAGES_PAGE_SIZE = 50;

// The queued messages ride on the uncursored page of the messages endpoint,
// so settling the queue after a promote needs its own request. Refetching
// chatMessagesForInfiniteScroll would reload every page already scrolled.
export const chatQueueConvergence = (chatId: string) => ({
	queryKey: chatKeys.queueConvergence(chatId),
	queryFn: () => API.experimental.getChatMessages(chatId),
	gcTime: 0,
});

export const chatMessagesForInfiniteScroll = (chatId: string) => ({
	queryKey: chatKeys.messages(chatId),
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
	// The per-chat socket is authoritative for messages: on every connect
	// it replays messages after the client's cursor, emits history_reset
	// with the replacement history for edits and deletions below it, and
	// sends an authoritative queue_update snapshot (including an empty
	// array once the queue drains). A background refetch would only
	// re-download pages the socket already reconciled, and default gcTime
	// evicts the cache once the chat is inactive.
	staleTime: Number.POSITIVE_INFINITY,
});

// Cap requested prompts to keep the response small; well under the server-side maximum.
const PROMPT_HISTORY_LIMIT = 500;

const PROMPTS_STALE_MS = 30_000;

export const chatPromptsQuery = (chatId: string) => ({
	queryKey: chatKeys.prompts(chatId),
	queryFn: () =>
		API.experimental.getChatPrompts(chatId, { limit: PROMPT_HISTORY_LIMIT }),
	staleTime: PROMPTS_STALE_MS,
	enabled: chatId !== "",
});

type ChatArchiveContext = {
	previousArchived: boolean;
	previousPinOrder: number;
	found: boolean;
};

export const archiveChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) =>
		API.experimental.updateChat(chatId, { archived: true }),
	onMutate: async (chatId: string): Promise<ChatArchiveContext> => {
		await cancelChatMutationRefetches(queryClient, chatId);
		// The sidebar can archive a chat that was never opened, so the
		// prior state has to come from whichever cache holds it.
		const previousChat = findChatInCaches(queryClient, chatId);
		// Reuse patchChatArchiveState so the optimistic state matches the
		// confirmed onSuccess state, including the pin_order reset the
		// server applies when archiving.
		patchChatEverywhere(queryClient, chatId, (chat) =>
			patchChatArchiveState(chat, true),
		);
		return {
			previousArchived: previousChat?.archived ?? false,
			previousPinOrder: previousChat?.pin_order ?? 0,
			found: previousChat !== undefined,
		};
	},
	onError: (
		_error: unknown,
		chatId: string,
		context: ChatArchiveContext | undefined,
	) => {
		if (!context?.found) {
			return;
		}
		const { previousArchived, previousPinOrder } = context;
		// Inverse patch, guarded on the optimistic values still being in
		// place, so a WebSocket write that landed mid-flight is not undone.
		patchChatEverywhere(queryClient, chatId, (chat) =>
			chat.archived && chat.pin_order === 0
				? { ...chat, archived: previousArchived, pin_order: previousPinOrder }
				: chat,
		);
	},
	onSuccess: (_data: unknown, chatId: string) => {
		applyChatArchiveStateToCaches(queryClient, chatId, true);
	},
	onSettled: (_data: unknown, _error: unknown, chatId: string) => {
		void invalidateChatListQueries(queryClient);
		void queryClient.invalidateQueries({
			queryKey: chatKeys.detail(chatId),
			exact: true,
		});
		void queryClient.invalidateQueries({
			queryKey: chatKeys.byWorkspacePrefix(),
		});
	},
});

export const unarchiveChat = (queryClient: QueryClient) => ({
	mutationFn: (chatId: string) =>
		API.experimental.updateChat(chatId, { archived: false }),
	onMutate: async (chatId: string): Promise<ChatArchiveContext> => {
		await cancelChatMutationRefetches(queryClient, chatId);
		const previousChat = findChatInCaches(queryClient, chatId);
		patchChatEverywhere(queryClient, chatId, (chat) =>
			patchChatArchiveState(chat, false),
		);
		return {
			previousArchived: previousChat?.archived ?? true,
			// Unarchiving leaves pin_order alone, so there is nothing to
			// restore for it.
			previousPinOrder: previousChat?.pin_order ?? 0,
			found: previousChat !== undefined,
		};
	},
	onError: (
		_error: unknown,
		chatId: string,
		context: ChatArchiveContext | undefined,
	) => {
		if (!context?.found) {
			return;
		}
		const { previousArchived } = context;
		patchChatEverywhere(queryClient, chatId, (chat) =>
			chat.archived ? chat : { ...chat, archived: previousArchived },
		);
	},
	onSuccess: (_data: unknown, chatId: string) => {
		applyChatArchiveStateToCaches(queryClient, chatId, false);
	},
	onSettled: (_data: unknown, _error: unknown, chatId: string) => {
		void invalidateChatListQueries(queryClient);
		void queryClient.invalidateQueries({
			queryKey: chatKeys.detail(chatId),
			exact: true,
		});
		void queryClient.invalidateQueries({
			queryKey: chatKeys.byWorkspacePrefix(),
		});
	},
});

type UpdateChatPlanModeContext = {
	previousPlanMode: TypesGen.ChatPlanMode | undefined;
	found: boolean;
};

export const updateChatPlanMode = (queryClient: QueryClient) => ({
	mutationFn: ({ chatId, planMode }: UpdateChatPlanModeVariables) =>
		API.experimental.updateChat(chatId, {
			plan_mode: toChatPlanModePayload(planMode),
		}),
	onMutate: async ({
		chatId,
		planMode,
	}: UpdateChatPlanModeVariables): Promise<UpdateChatPlanModeContext> => {
		await cancelChatMutationRefetches(queryClient, chatId);
		const previousChat = findChatInCaches(queryClient, chatId);
		patchChatEverywhere(queryClient, chatId, (chat) =>
			chat.plan_mode === planMode ? chat : { ...chat, plan_mode: planMode },
		);
		return {
			// undefined is a real plan mode (cleared), so the `found` flag
			// rather than the value decides whether a rollback can run.
			previousPlanMode: previousChat?.plan_mode,
			found: previousChat !== undefined,
		};
	},
	onError: (
		_error: unknown,
		{ chatId, planMode }: UpdateChatPlanModeVariables,
		context: UpdateChatPlanModeContext | undefined,
	) => {
		if (!context?.found) {
			return;
		}
		const { previousPlanMode } = context;
		patchChatEverywhere(queryClient, chatId, (chat) =>
			chat.plan_mode === planMode
				? { ...chat, plan_mode: previousPlanMode }
				: chat,
		);
	},
	onSettled: (
		_data: unknown,
		_error: unknown,
		{ chatId }: UpdateChatPlanModeVariables,
	) => {
		void invalidateChatListQueries(queryClient);
		void queryClient.invalidateQueries({
			queryKey: chatKeys.detail(chatId),
			exact: true,
		});
	},
});

type UpdateChatWorkspaceContext = {
	previousWorkspaceId: string | undefined;
	found: boolean;
};

export const updateChatWorkspace = (queryClient: QueryClient) => ({
	mutationFn: ({ chatId, workspaceId }: UpdateChatWorkspaceVariables) =>
		API.experimental.updateChat(chatId, {
			workspace_id:
				workspaceId ??
				// The API uses the nil UUID to clear the workspace association.
				"00000000-0000-0000-0000-000000000000",
		}),
	onMutate: async ({
		chatId,
		workspaceId,
	}: UpdateChatWorkspaceVariables): Promise<UpdateChatWorkspaceContext> => {
		await cancelChatMutationRefetches(queryClient, chatId);
		const previousChat = findChatInCaches(queryClient, chatId);
		const optimisticWorkspaceId = workspaceId ?? undefined;
		patchChatEverywhere(queryClient, chatId, (chat) =>
			chat.workspace_id === optimisticWorkspaceId
				? chat
				: { ...chat, workspace_id: optimisticWorkspaceId },
		);
		return {
			previousWorkspaceId: previousChat?.workspace_id,
			found: previousChat !== undefined,
		};
	},
	onError: (
		_error: unknown,
		{ chatId, workspaceId }: UpdateChatWorkspaceVariables,
		context: UpdateChatWorkspaceContext | undefined,
	) => {
		if (!context?.found) {
			return;
		}
		const { previousWorkspaceId } = context;
		const optimisticWorkspaceId = workspaceId ?? undefined;
		patchChatEverywhere(queryClient, chatId, (chat) =>
			chat.workspace_id === optimisticWorkspaceId
				? { ...chat, workspace_id: previousWorkspaceId }
				: chat,
		);
	},
	onSettled: (
		_data: unknown,
		_error: unknown,
		{ chatId }: UpdateChatWorkspaceVariables,
	) => {
		void invalidateChatListQueries(queryClient);
		void queryClient.invalidateQueries({
			queryKey: chatKeys.detail(chatId),
			exact: true,
		});
		void queryClient.invalidateQueries({
			queryKey: chatKeys.byWorkspacePrefix(),
		});
	},
});

/**
 * Shared mutation key for the pin, unpin, and reorder mutations. They
 * all renumber the same pin_order sequence, so each one's settle
 * invalidation has to know whether another is still in flight.
 */
const CHAT_PIN_MUTATION_KEY = [...chatKeys.all, "pin"] as const;

/**
 * How many pin-family mutations the client still counts as pending. A
 * mutation is still counted while its own onError and onSettled handlers
 * run, so one means "I am the only one left"; zero only happens when a
 * handler is invoked outside a mutation.
 */
const pendingPinMutationCount = (queryClient: QueryClient): number =>
	queryClient.isMutating({ mutationKey: CHAT_PIN_MUTATION_KEY });

/**
 * True when no other pin-family mutation is still pending. Anything
 * higher than the settling mutation itself means a later optimistic
 * write is outstanding and a refetch now would overwrite it.
 */
const isLastPinMutation = (queryClient: QueryClient): boolean =>
	pendingPinMutationCount(queryClient) <= 1;

/**
 * True when another pin-family mutation is still pending, so this one's
 * optimistic write is no longer the top layer in the cache.
 */
const isPinMutationSuperseded = (queryClient: QueryClient): boolean =>
	pendingPinMutationCount(queryClient) > 1;

type ChatPinOrderContext = {
	previousPinOrder: number;
	optimisticPinOrder: number;
	found: boolean;
};

/**
 * Terminal reconciliation for the pin family. Pinning, unpinning, and
 * reordering all renumber neighbouring chats server side, so the target
 * chat is never the only one whose pin_order goes stale: every loaded
 * chat detail has to be reconciled, not just this mutation's target.
 *
 * The whole thing stays behind the last-settle gate because an earlier
 * refetch would land on top of a later mutation's optimistic write.
 */
const settlePinMutation = (queryClient: QueryClient) => {
	if (!isLastPinMutation(queryClient)) {
		return;
	}
	void invalidateChatListQueries(queryClient);
	void queryClient.invalidateQueries({
		queryKey: chatKeys.details(),
		// Base chat details only. The nested messages, prompts, and acl
		// caches share the prefix but hold nothing a pin_order change can
		// invalidate.
		predicate: (query) =>
			query.queryKey.length === chatKeys.details().length + 1 &&
			hasLoadedData(query),
	});
};

export const pinChat = (queryClient: QueryClient) => ({
	mutationKey: CHAT_PIN_MUTATION_KEY,
	mutationFn: (chatId: string) =>
		API.experimental.updateChat(chatId, { pin_order: 1 }),
	onMutate: async (chatId: string): Promise<ChatPinOrderContext> => {
		await cancelChatMutationRefetches(queryClient, chatId);
		const previousChat = findChatInCaches(queryClient, chatId);
		const optimisticPinOrder = getNextOptimisticPinOrder(queryClient);
		patchChatEverywhere(queryClient, chatId, (chat) =>
			chat.pin_order === optimisticPinOrder
				? chat
				: { ...chat, pin_order: optimisticPinOrder },
		);
		return {
			previousPinOrder: previousChat?.pin_order ?? 0,
			optimisticPinOrder,
			found: previousChat !== undefined,
		};
	},
	onError: (
		_error: unknown,
		chatId: string,
		context: ChatPinOrderContext | undefined,
	) => {
		if (!context?.found) {
			return;
		}
		const { previousPinOrder, optimisticPinOrder } = context;
		patchChatEverywhere(queryClient, chatId, (chat) =>
			chat.pin_order === optimisticPinOrder
				? { ...chat, pin_order: previousPinOrder }
				: chat,
		);
	},
	onSettled: (_data: unknown, _error: unknown, _chatId: string) => {
		settlePinMutation(queryClient);
	},
});

export const unpinChat = (queryClient: QueryClient) => ({
	mutationKey: CHAT_PIN_MUTATION_KEY,
	mutationFn: (chatId: string) =>
		API.experimental.updateChat(chatId, { pin_order: 0 }),
	onMutate: async (chatId: string): Promise<ChatPinOrderContext> => {
		await cancelChatMutationRefetches(queryClient, chatId);
		const previousChat = findChatInCaches(queryClient, chatId);
		patchChatEverywhere(queryClient, chatId, (chat) =>
			chat.pin_order === 0 ? chat : { ...chat, pin_order: 0 },
		);
		return {
			previousPinOrder: previousChat?.pin_order ?? 0,
			optimisticPinOrder: 0,
			found: previousChat !== undefined,
		};
	},
	onError: (
		_error: unknown,
		chatId: string,
		context: ChatPinOrderContext | undefined,
	) => {
		if (!context?.found) {
			return;
		}
		const { previousPinOrder } = context;
		patchChatEverywhere(queryClient, chatId, (chat) =>
			chat.pin_order === 0 ? { ...chat, pin_order: previousPinOrder } : chat,
		);
	},
	onSettled: (_data: unknown, _error: unknown, _chatId: string) => {
		settlePinMutation(queryClient);
	},
});

type ReorderPinnedChatVariables = {
	chatId: string;
	pinOrder: number;
	/**
	 * The pinned chats the sidebar is currently rendering, in render
	 * order, including a drag the parent has not rerendered with yet.
	 * The pinned set is derived from this rather than from the first
	 * cached list variant, which can be an archived or filtered list
	 * that holds no pinned chats at all.
	 */
	visibleChats: readonly TypesGen.Chat[];
};

type ReorderPinnedChatContext = {
	previousOrders: Map<string, number>;
	optimisticOrders: Map<string, number>;
};

const applyPinOrders = (
	queryClient: QueryClient,
	resolveOrder: (chat: TypesGen.Chat) => number | undefined,
) => {
	updateInfiniteChatsCache(queryClient, (chats) => {
		let changed = false;
		const next = chats.map((chat) => {
			const order = resolveOrder(chat);
			if (order === undefined || order === chat.pin_order) {
				return chat;
			}
			changed = true;
			return { ...chat, pin_order: order };
		});
		return changed ? next : chats;
	});
};

export const reorderPinnedChat = (queryClient: QueryClient) => ({
	mutationKey: CHAT_PIN_MUTATION_KEY,
	// Overlapping drags share one scope so their PATCHes run one at a
	// time. onMutate still runs immediately for every drag, before the
	// scope gate pauses mutationFn, so the sidebar never drops a drop.
	scope: { id: "chat-reorder" },
	mutationFn: ({ chatId, pinOrder }: ReorderPinnedChatVariables) =>
		API.experimental.updateChat(chatId, { pin_order: pinOrder }),
	onMutate: async ({
		chatId,
		pinOrder,
		visibleChats,
	}: ReorderPinnedChatVariables): Promise<
		ReorderPinnedChatContext | undefined
	> => {
		await cancelChatMutationRefetches(queryClient, chatId);

		// Mirror the server's deterministic renumbering so the sidebar
		// shows the new order immediately. The panel's local drag order
		// is cleared by any chats reference change, and a running agent
		// changes that reference constantly, so the cache has to already
		// hold the new order when the handoff happens.
		const pinned = visibleChats.filter((chat) => chat.pin_order > 0);
		const oldIdx = pinned.findIndex((chat) => chat.id === chatId);
		if (oldIdx === -1) {
			return undefined;
		}

		const reordered = [...pinned];
		const [moved] = reordered.splice(oldIdx, 1);
		reordered.splice(pinOrder - 1, 0, moved);

		const previousOrders = new Map(
			pinned.map((chat) => [chat.id, chat.pin_order]),
		);
		const optimisticOrders = new Map(
			reordered.map((chat, index) => [chat.id, index + 1]),
		);
		applyPinOrders(queryClient, (chat) => optimisticOrders.get(chat.id));
		return { previousOrders, optimisticOrders };
	},
	onError: (
		_error: unknown,
		_variables: ReorderPinnedChatVariables,
		context: ReorderPinnedChatContext | undefined,
	) => {
		if (!context) {
			return;
		}
		if (isPinMutationSuperseded(queryClient)) {
			// A later pin-family mutation has already written its own order
			// on top of this one. Optimistic layers are last-writer-wins,
			// and the value guard below cannot tell a pin_order this
			// mutation assigned from the same value a later mutation
			// assigned, so rolling back here would corrupt the newer order.
			// Skipping is safe: the last pin-family mutation to settle
			// refetches the list and every loaded chat detail, so the
			// server's order always lands.
			return;
		}
		const { previousOrders, optimisticOrders } = context;
		// Restore each chat only where it still holds the pin_order this
		// mutation assigned it, so a later reorder is left in place.
		applyPinOrders(queryClient, (chat) =>
			chat.pin_order === optimisticOrders.get(chat.id)
				? previousOrders.get(chat.id)
				: undefined,
		);
	},
	onSettled: (
		_data: unknown,
		_error: unknown,
		_variables: ReorderPinnedChatVariables,
	) => {
		settlePinMutation(queryClient);
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
		// The endpoint answers 204, so there is no entity to write back.
		// Mirror the server's own TrimSpaces so the optimistic title cannot
		// differ from what the next fetch returns.
		const trimmedTitle = title.trim();
		patchChatEverywhere(queryClient, chatId, (chat) =>
			chat.title === trimmedTitle ? chat : { ...chat, title: trimmedTitle },
		);
	},

	onSettled: (
		_data: unknown,
		_error: unknown,
		{ chatId }: UpdateChatTitleVariables,
	) => {
		void invalidateChatListQueries(queryClient);
		void queryClient.invalidateQueries({
			queryKey: chatKeys.detail(chatId),
			exact: true,
		});
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
		queryKey: chatKeys.debugRuns(chatId),
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
		queryKey: chatKeys.debugRun(chatId, runId),
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
		queryKey: chatKeys.debugRuns(chatId),
	});
};

export const createChat = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.CreateChatRequest) =>
		API.experimental.createChat(req),
	onSuccess: () => {
		void invalidateChatListQueries(queryClient);
		void queryClient.invalidateQueries({
			queryKey: chatKeys.byWorkspacePrefix(),
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
		void queryClient.invalidateQueries({
			queryKey: chatKeys.detail(chatId),
			exact: true,
		});
		void queryClient.invalidateQueries({
			queryKey: chatKeys.prompts(chatId),
			exact: true,
		});
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
			queryKey: chatKeys.messages(chatId),
			exact: true,
		});

		const previousData = queryClient.getQueryData<
			InfiniteData<TypesGen.ChatMessagesResponse>
		>(chatKeys.messages(chatId));

		queryClient.setQueryData<
			InfiniteData<TypesGen.ChatMessagesResponse> | undefined
		>(chatKeys.messages(chatId), (current) =>
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
			queryClient.setQueryData(chatKeys.messages(chatId), context.previousData);
		}
		// Invalidate messages as a safety net: the restored snapshot
		// may be missing WebSocket-delivered messages that arrived
		// during the mutation's flight time.
		void queryClient.invalidateQueries({
			queryKey: chatKeys.messages(chatId),
			exact: true,
		});
	},
	onSuccess: (
		response: TypesGen.EditChatMessageResponse,
		variables: EditChatMessageMutationArgs,
	) => {
		queryClient.setQueryData<
			InfiniteData<TypesGen.ChatMessagesResponse> | undefined
		>(chatKeys.messages(chatId), (current) =>
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
		// Invalidating the messages key would trigger a redundant
		// refetch that causes extra store mutations while the
		// sticky user message is settling after the optimistic
		// truncation.
		void queryClient.invalidateQueries({
			queryKey: chatKeys.detail(chatId),
			exact: true,
		});
		void queryClient.invalidateQueries({
			queryKey: chatKeys.prompts(chatId),
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

export const compactChat = (queryClient: QueryClient, chatId: string) => ({
	mutationFn: () => API.experimental.compactChat(chatId),
	onSuccess: () => {
		// The compaction transitions the chat to running; the summary
		// rows stream in over the websocket like any other turn.
		void queryClient.invalidateQueries({
			queryKey: chatKeys.detail(chatId),
			exact: true,
		});
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
		// Only `context` is authoritative on this response: the endpoint
		// computes Context.Resources the same way getChat does, but omits
		// diff_status, files, and children. Merge that one field rather
		// than writing the entity.
		patchChatEverywhere(queryClient, chatId, (chat) =>
			chat.context === updatedChat.context
				? chat
				: { ...chat, context: updatedChat.context },
		);
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
			queryKey: chatKeys.detail(chatId),
			exact: true,
		});
		await queryClient.invalidateQueries({
			queryKey: chatKeys.messages(chatId),
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

export const chatDiffContents = (chatId: string) => ({
	queryKey: chatKeys.diffContents(chatId),
	queryFn: () => API.experimental.getChatDiffContents(chatId),
});

export const chatFile = (fileId: string) =>
	queryOptions({
		queryKey: ["chatFile", fileId] as const,
		queryFn: () => API.experimental.getChatFileText(fileId),
		staleTime: Number.POSITIVE_INFINITY,
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

const chatPersonalModelOverridesAdminSettingsKey =
	chatKeys.adminPersonalModelOverrides();

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

const userChatPersonalModelOverridesKey = chatKeys.userPersonalModelOverrides();

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

export const chatModelConfigsKey = ["chat-model-configs"] as const;

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
		queryClient.invalidateQueries({ queryKey: chatProviderConfigsKey }),
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

type ChatCostDateParams = {
	start_date?: string;
	end_date?: string;
};

export const chatCostSummary = (user = "me", params?: ChatCostDateParams) => ({
	queryKey: chatKeys.costSummary(user, params),
	queryFn: () => API.experimental.getChatCostSummary(user, params),
	staleTime: 60_000,
});

const GATEWAY_REQUEST_STALE_MS = 30_000;

export const chatCost = (rootChatId: string) => ({
	queryKey: chatKeys.cost(rootChatId),
	queryFn: () => API.experimental.getChatCost(rootChatId),
	staleTime: GATEWAY_REQUEST_STALE_MS,
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
			chatKeys.costUsers(payload, pageNumber),
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

export const chatUsageLimitStatus = () => ({
	queryKey: chatKeys.usageLimitStatus(),
	queryFn: () => API.experimental.getChatUsageLimitStatus(),
	refetchInterval: 60_000,
});

const chatUsageLimitConfigKey = chatKeys.usageLimitConfig();

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
		await queryClient.invalidateQueries({
			queryKey: chatKeys.acl(chatId),
			exact: true,
		});
	},
});

export const setChatGroupRole = (queryClient: QueryClient) => ({
	mutationFn: ({ chatId, groupId, role }: SetChatGroupRoleVariables) =>
		API.experimental.updateChatACL(chatId, {
			group_roles: { [groupId]: role },
		}),
	onSuccess: async (_data: unknown, { chatId }: SetChatGroupRoleVariables) => {
		await queryClient.invalidateQueries({
			queryKey: chatKeys.acl(chatId),
			exact: true,
		});
	},
});
