import { infiniteQueryOptions, type QueryClient } from "react-query";
import { API } from "#/api/api";
import {
	cancelChatListRefetches,
	cancelLoadedChatEntityRefetch,
	chatListFamilyKey,
	getChatListQueryString,
	invalidateChatEntity,
	invalidateChatListQueries,
	patchChatEntity,
	toChatListParams,
	updateInfiniteChatsCache,
} from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";

/** The sidebar's default filters: every unarchived chat. */
const params = toChatListParams();

/**
 * Under the list family, in the shape chatListKey builds: the params
 * object one slot after the prefix, so the archive helpers read its
 * `archived` filter and drop archived chats from the board too. The extra
 * `board` field keeps it distinct from the sidebar's key: the board
 * partitions every chat into columns, so it needs the whole list, not the
 * sidebar's pages.
 */
export const boardChatsKey = [
	...chatListFamilyKey,
	{ ...params, board: true },
] as const;

// Limit 0 means the server default, not unlimited.
const PAGE = 200;

const allChats = async (): Promise<Chat[]> => {
	const q = getChatListQueryString(params);
	const chats: Chat[] = [];
	for (let offset = 0; ; offset += PAGE) {
		const page = await API.experimental.getChats({ limit: PAGE, offset, q });
		chats.push(...page);
		if (page.length < PAGE) return chats;
	}
};

// An infinite query with a single page: the list helpers (label patches,
// watch merges, prepends) iterate `pages` and would skip a flat array.
// Focus refetch and retries match infiniteChats; the app defaults are off.
export const boardChats = () =>
	infiniteQueryOptions({
		queryKey: boardChatsKey,
		queryFn: allChats,
		initialPageParam: 0,
		getNextPageParam: () => undefined,
		refetchOnWindowFocus: true,
		retry: 3,
	});

type UpdateChatLabelsVariables = {
	chatId: string;
	labels: Record<string, string>;
	/** The request goes out only once this resolves; a rejection skips it. */
	after?: Promise<unknown>;
};

// Label writes in flight per client. The list is invalidated only when the
// last one settles: a response that predates a pending write would replace
// its optimistic patch. Not `isMutating`: two writes settling in one tick
// both see 2.
const pending = new WeakMap<QueryClient, { count: number }>();

const pendingFor = (queryClient: QueryClient) => {
	let entry = pending.get(queryClient);
	if (!entry) {
		entry = { count: 0 };
		pending.set(queryClient, entry);
	}
	return entry;
};

// Labels replace the whole map server-side, so callers pass the complete
// desired map. The caches are patched before the request so board drags
// settle instantly; the settled invalidation corrects any divergence.
export const updateChatLabels = (queryClient: QueryClient) => ({
	mutationFn: async ({ chatId, labels, after }: UpdateChatLabelsVariables) => {
		// A skipped write sends nothing; the settled refetch reverts its
		// optimistic patch.
		if (
			after &&
			!(await after.then(
				() => true,
				() => false,
			))
		) {
			return;
		}
		return API.experimental.updateChat(chatId, { labels });
	},

	onMutate: async ({ chatId, labels }: UpdateChatLabelsVariables) => {
		pendingFor(queryClient).count += 1;
		// A list refetch already in flight carries pre-write labels. If it lands
		// after the patch below it replaces the list, the board reverts, and
		// the next write on this chat is built from the stale map. Initial loads
		// and pagination fetches are left alone by these helpers.
		await Promise.all([
			cancelChatListRefetches(queryClient),
			cancelLoadedChatEntityRefetch(queryClient, chatId),
		]);
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
		const inFlight = pendingFor(queryClient);
		inFlight.count -= 1;
		if (inFlight.count === 0) void invalidateChatListQueries(queryClient);
		void invalidateChatEntity(queryClient, chatId);
	},
});
