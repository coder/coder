import { infiniteQueryOptions, type QueryClient } from "react-query";
import { API } from "#/api/api";
import {
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

const allChats = async (signal: AbortSignal): Promise<Chat[]> => {
	const q = getChatListQueryString(params);
	const chats: Chat[] = [];
	for (let offset = 0; ; offset += PAGE) {
		// Aborted when the query is cancelled: a cancelled request left
		// running answered a later identical request with its older body.
		const page = await API.experimental.getChats(
			{ limit: PAGE, offset, q },
			signal,
		);
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
		queryFn: async ({ client, signal }) => {
			const settledBefore = settledWrites;
			const chats = await allChats(signal);
			if (!isBoardWriting(client) && settledWrites === settledBefore) {
				return chats;
			}
			// The board's labels are authoritative while it writes: this
			// response may predate a queued write or one that landed while it
			// was in flight. The refresh after the last write syncs them.
			const cached = new Map(
				client
					.getQueryData<{ pages: Chat[][] }>(boardChatsKey)
					?.pages.flat()
					.map((chat) => [chat.id, chat.labels]),
			);
			return chats.map((chat) => {
				const labels = cached.get(chat.id);
				return labels ? { ...chat, labels } : chat;
			});
		},
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

// Board writes run one at a time in call order, so a map built before a
// newer write cannot land after it. onMutate still patches at once.
export const boardWriteScope = { id: "chat-board-write" };

/** Whether a board write is queued or in flight. */
export const isBoardWriting = (client: QueryClient) =>
	client.isMutating({
		predicate: (m) => m.options.scope?.id === boardWriteScope.id,
	}) > 0;

// Counts settled board writes, so a list response can tell whether one
// landed while it was in flight.
let settledWrites = 0;

// The list is refreshed once the board has been quiet this long, so a
// burst of label writes costs one refetch instead of one per write.
const REFRESH_DELAY_MS = 1500;
let refreshTimer: ReturnType<typeof setTimeout> | undefined;

// Labels replace the whole map server-side, so callers pass the complete
// desired map. The caches are patched before the request so board drags
// settle instantly; the refresh after the last write corrects any
// divergence.
export const updateChatLabels = (queryClient: QueryClient) => ({
	scope: boardWriteScope,
	mutationFn: async ({ chatId, labels, after }: UpdateChatLabelsVariables) => {
		// A skipped write sends nothing; the refresh after the last write
		// reverts its optimistic patch.
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
		clearTimeout(refreshTimer);
		// A board refetch in flight may predate this write. Cancelling aborts
		// it and rolls the cache back to where it started, then the patch
		// applies on top; the refresh after the last write replaces it.
		await queryClient.cancelQueries({ queryKey: boardChatsKey, exact: true });
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
		settledWrites += 1;
		clearTimeout(refreshTimer);
		refreshTimer = setTimeout(
			() => void invalidateChatListQueries(queryClient),
			REFRESH_DELAY_MS,
		);
		void invalidateChatEntity(queryClient, chatId);
	},
});
