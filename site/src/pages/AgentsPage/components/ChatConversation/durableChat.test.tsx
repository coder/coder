import { act, render, renderHook, waitFor } from "@testing-library/react";
import type { FC, PropsWithChildren } from "react";
import {
	type InfiniteData,
	QueryClient,
	QueryClientProvider,
} from "react-query";
import { describe, expect, it } from "vitest";
import { chatKeys } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	type ChatStore,
	createChatStore,
	selectSuppressedQueuedMessageIDs,
	useChatSelector,
} from "./chatStore";
import {
	readCanonicalQueuedMessages,
	readEffectiveQueuedMessages,
	shouldSuppressFinalizedOverlay,
	useDurableChatStatus,
	useDurableMessageCount,
	useDurableMessageList,
	useDurableQueuedMessages,
	useIsAwaitingFirstStreamChunk,
} from "./durableChat";

const CHAT_ID = "chat-1";

const createTestQueryClient = (): QueryClient =>
	new QueryClient({
		defaultOptions: {
			queries: {
				retry: false,
				gcTime: Number.POSITIVE_INFINITY,
				refetchOnWindowFocus: false,
				networkMode: "offlineFirst",
			},
		},
	});

/**
 * The facade reads status from the detail cache, so every status assertion
 * needs a seeded entry and a provider around the hook.
 */
const seedChatDetail = (
	queryClient: QueryClient,
	status: TypesGen.ChatStatus,
	snapshotVersion = 1,
): void => {
	queryClient.setQueryData<TypesGen.Chat>(chatKeys.detail(CHAT_ID), {
		...MockChat,
		id: CHAT_ID,
		status,
		snapshot_version: snapshotVersion,
	});
};

/**
 * Durable messages are canonical in the paginated messages cache, so the facade
 * reads whatever the socket and the REST pages put there. Seeding mirrors the
 * API's newest-first page order.
 */
const seedMessages = (
	queryClient: QueryClient,
	messages: readonly TypesGen.ChatMessage[],
	extraPage?: readonly TypesGen.ChatMessage[],
	queuedMessages: readonly TypesGen.ChatQueuedMessage[] = [],
): void => {
	const toPage = (
		pageMessages: readonly TypesGen.ChatMessage[],
		hasMore: boolean,
		pageQueuedMessages: readonly TypesGen.ChatQueuedMessage[] = [],
	): TypesGen.ChatMessagesResponse => ({
		messages: [...pageMessages].sort((left, right) => right.id - left.id),
		queued_messages: pageQueuedMessages,
		has_more: hasMore,
	});
	queryClient.setQueryData<InfiniteData<TypesGen.ChatMessagesResponse>>(
		chatKeys.messages(CHAT_ID),
		{
			pages: extraPage
				? [toPage(messages, true, queuedMessages), toPage(extraPage, false)]
				: [toPage(messages, false, queuedMessages)],
			pageParams: extraPage ? [undefined, messages[0]?.id] : [undefined],
		},
	);
};

const createWrapper =
	(queryClient: QueryClient): FC<PropsWithChildren> =>
	({ children }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);

const buildMessage = (
	id: number,
	role: TypesGen.ChatMessageRole,
	text: string,
): TypesGen.ChatMessage => ({
	id,
	chat_id: CHAT_ID,
	created_at: "2025-01-01T00:00:00.000Z",
	role,
	content: [{ type: "text", text }],
});

const buildQueuedMessage = (
	id: number,
	text: string,
): TypesGen.ChatQueuedMessage => ({
	id,
	chat_id: CHAT_ID,
	created_at: "2025-01-01T00:00:00.000Z",
	content: [{ type: "text", text }],
});

const buildTextPart = (text: string): TypesGen.ChatMessagePart => ({
	type: "text",
	text,
});

describe("durableChat facade", () => {
	it("reads messages, status, and the queue from the cache", () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		seedChatDetail(queryClient, "running");
		const messages = [
			buildMessage(1, "user", "hello"),
			buildMessage(2, "assistant", "hi"),
		];
		const queued = buildQueuedMessage(10, "later");
		seedMessages(queryClient, messages, undefined, [queued]);

		const { result } = renderHook(
			() => ({
				facadeStatus: useDurableChatStatus({ store, chatId: CHAT_ID }),
				facadeMessages: useDurableMessageList({ store, chatId: CHAT_ID }),
				facadeCount: useDurableMessageCount({ store, chatId: CHAT_ID }),
				facadeQueued: useDurableQueuedMessages({ store, chatId: CHAT_ID }),
			}),
			{ wrapper: createWrapper(queryClient) },
		);

		const current = result.current;
		expect(current.facadeStatus).toBe("running");
		// Ascending by ID, which is the only ordering signal a message carries.
		expect(current.facadeMessages).toEqual(messages);
		expect(current.facadeCount).toBe(messages.length);
		expect(current.facadeQueued).toEqual([queued]);
	});

	it("hides a suppressed queued message without touching the cache", async () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		const kept = buildQueuedMessage(10, "kept");
		const hidden = buildQueuedMessage(11, "hidden");
		seedMessages(queryClient, [buildMessage(1, "user", "hello")], undefined, [
			kept,
			hidden,
		]);

		const { result } = renderHook(
			() => ({
				queued: useDurableQueuedMessages({ store, chatId: CHAT_ID }),
				suppressed: useChatSelector(store, selectSuppressedQueuedMessageIDs),
			}),
			{ wrapper: createWrapper(queryClient) },
		);

		expect(result.current.queued).toEqual([kept, hidden]);

		act(() => {
			store.suppressQueuedMessageIDs([hidden.id]);
		});

		await waitFor(() => {
			expect(result.current.queued).toEqual([kept]);
		});
		// The marker subtracts at read time; the cache still holds server truth.
		expect(
			queryClient.getQueryData<InfiniteData<TypesGen.ChatMessagesResponse>>(
				chatKeys.messages(CHAT_ID),
			)?.pages[0].queued_messages,
		).toEqual([kept, hidden]);

		act(() => {
			store.unsuppressQueuedMessageIDs([hidden.id]);
		});

		await waitFor(() => {
			expect(result.current.queued).toEqual([kept, hidden]);
		});
	});

	it("deduplicates a cross-page copy, keeping the page 0 values", () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		const canonical = buildMessage(2, "assistant", "revised");
		seedMessages(
			queryClient,
			[buildMessage(3, "user", "newest"), canonical],
			// An older page still holding a superseded copy of ID 2.
			[
				buildMessage(1, "user", "oldest"),
				buildMessage(2, "assistant", "stale"),
			],
		);

		const { result } = renderHook(
			() => ({
				messages: useDurableMessageList({ store, chatId: CHAT_ID }),
				count: useDurableMessageCount({ store, chatId: CHAT_ID }),
			}),
			{ wrapper: createWrapper(queryClient) },
		);

		expect(result.current.messages.map((message) => message.id)).toEqual([
			1, 2, 3,
		]);
		expect(result.current.messages[1]).toEqual(canonical);
		expect(result.current.count).toBe(3);
	});

	it("returns null when no chat is selected", () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		seedChatDetail(queryClient, "running");

		const { result } = renderHook(
			() => ({
				status: useDurableChatStatus({ store, chatId: undefined }),
				messages: useDurableMessageList({ store, chatId: undefined }),
				count: useDurableMessageCount({ store, chatId: undefined }),
			}),
			{ wrapper: createWrapper(queryClient) },
		);

		expect(result.current.status).toBeNull();
		expect(result.current.messages).toEqual([]);
		expect(result.current.count).toBe(0);
	});

	it("re-renders the status consumer when the cached status changes", async () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		seedChatDetail(queryClient, "waiting");

		const { result } = renderHook(
			() => useDurableChatStatus({ store, chatId: CHAT_ID }),
			{ wrapper: createWrapper(queryClient) },
		);

		expect(result.current).toBe("waiting");

		seedChatDetail(queryClient, "running", 2);

		await waitFor(() => {
			expect(result.current).toBe("running");
		});
	});

	it("keeps the message list reference stable when the status changes", async () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		seedChatDetail(queryClient, "waiting");
		seedMessages(queryClient, [buildMessage(1, "user", "hello")]);

		const { result } = renderHook(
			() => ({
				messages: useDurableMessageList({ store, chatId: CHAT_ID }),
				status: useDurableChatStatus({ store, chatId: CHAT_ID }),
			}),
			{ wrapper: createWrapper(queryClient) },
		);

		const firstMessages = result.current.messages;

		seedChatDetail(queryClient, "running", 2);

		await waitFor(() => {
			expect(result.current.status).toBe("running");
		});
		expect(result.current.messages).toBe(firstMessages);

		seedMessages(queryClient, [
			buildMessage(1, "user", "hello"),
			buildMessage(2, "assistant", "hi"),
		]);

		await waitFor(() => {
			expect(result.current.messages).toHaveLength(2);
		});
		expect(result.current.messages).not.toBe(firstMessages);
	});

	it("does not re-render message or queue consumers on a message_part update", () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		seedMessages(queryClient, [buildMessage(1, "user", "hello")]);

		let queueRenderCount = 0;
		let messageRenderCount = 0;

		const QueueProbe: FC<{ store: ChatStore }> = ({ store: probeStore }) => {
			useDurableQueuedMessages({ store: probeStore, chatId: CHAT_ID });
			queueRenderCount += 1;
			return null;
		};

		const MessageProbe: FC<{ store: ChatStore }> = ({ store: probeStore }) => {
			useDurableMessageList({ store: probeStore, chatId: CHAT_ID });
			messageRenderCount += 1;
			return null;
		};

		const Wrapper = createWrapper(queryClient);
		render(
			<Wrapper>
				<QueueProbe store={store} />
				<MessageProbe store={store} />
			</Wrapper>,
		);

		const queueBaseline = queueRenderCount;
		const messageBaseline = messageRenderCount;

		act(() => {
			store.applyMessagePart(buildTextPart("partial"));
		});

		expect(queueRenderCount).toBe(queueBaseline);
		expect(messageRenderCount).toBe(messageBaseline);
	});

	it("shows the Thinking indicator only while the cached status is running", async () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		seedChatDetail(queryClient, "waiting");
		seedMessages(queryClient, [buildMessage(1, "user", "hello")]);

		const { result } = renderHook(
			() => useIsAwaitingFirstStreamChunk({ store, chatId: CHAT_ID }),
			{ wrapper: createWrapper(queryClient) },
		);

		// The cache half is satisfied (a user message is newest, no stream), the
		// status half is not.
		expect(result.current).toBe(false);

		seedChatDetail(queryClient, "running", 2);

		await waitFor(() => {
			expect(result.current).toBe(true);
		});
	});

	it("hides the Thinking indicator once the newest durable message is an assistant reply", async () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		seedChatDetail(queryClient, "running");
		seedMessages(queryClient, [buildMessage(1, "user", "hello")]);

		const { result } = renderHook(
			() => useIsAwaitingFirstStreamChunk({ store, chatId: CHAT_ID }),
			{ wrapper: createWrapper(queryClient) },
		);

		expect(result.current).toBe(true);

		seedMessages(queryClient, [
			buildMessage(1, "user", "hello"),
			buildMessage(2, "assistant", "hi"),
		]);

		await waitFor(() => {
			expect(result.current).toBe(false);
		});
	});

	it("hides the Thinking indicator while a finalizing overlay is still on screen", () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		seedChatDetail(queryClient, "running");
		seedMessages(queryClient, [buildMessage(1, "user", "hello")]);
		store.applyMessagePart(buildTextPart("streamed answer"));
		// The durable message is not readable yet, so without counting the
		// handoff the indicator would flash underneath the finalized tail.
		store.beginStreamFinalization(2);

		const { result } = renderHook(
			() => useIsAwaitingFirstStreamChunk({ store, chatId: CHAT_ID }),
			{ wrapper: createWrapper(queryClient) },
		);

		expect(result.current).toBe(false);
	});

	it("re-renders the awaiting-first-chunk consumer only when the flag flips", () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		seedChatDetail(queryClient, "running");
		seedMessages(queryClient, [buildMessage(1, "user", "hello")]);

		let renderCount = 0;
		const { result } = renderHook(
			() => {
				const isAwaiting = useIsAwaitingFirstStreamChunk({
					store,
					chatId: CHAT_ID,
				});
				renderCount += 1;
				return isAwaiting;
			},
			{ wrapper: createWrapper(queryClient) },
		);

		expect(result.current).toBe(true);
		const baseline = renderCount;

		act(() => {
			store.applyMessagePart(buildTextPart("first"));
		});

		expect(result.current).toBe(false);
		expect(renderCount).toBe(baseline + 1);

		act(() => {
			store.applyMessagePart(buildTextPart(" second"));
		});
		act(() => {
			store.applyMessagePart(buildTextPart(" third"));
		});

		expect(result.current).toBe(false);
		expect(renderCount).toBe(baseline + 1);
	});

	it("re-renders the message-count consumer only when the count changes", async () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		seedMessages(queryClient, [buildMessage(1, "user", "hello")]);

		let renderCount = 0;
		const { result } = renderHook(
			() => {
				const count = useDurableMessageCount({ store, chatId: CHAT_ID });
				renderCount += 1;
				return count;
			},
			{ wrapper: createWrapper(queryClient) },
		);

		expect(result.current).toBe(1);
		const baseline = renderCount;

		seedMessages(queryClient, [buildMessage(1, "user", "hello edited")]);

		// The cache notified, but the deduplicated size did not change, so the
		// numeric select keeps the consumer from re-rendering.
		await waitFor(() => {
			expect(
				queryClient.getQueryState(chatKeys.messages(CHAT_ID))?.dataUpdateCount,
			).toBe(2);
		});
		expect(result.current).toBe(1);
		expect(renderCount).toBe(baseline);

		seedMessages(queryClient, [
			buildMessage(1, "user", "hello edited"),
			buildMessage(2, "assistant", "hi"),
		]);

		await waitFor(() => {
			expect(result.current).toBe(2);
		});
		expect(renderCount).toBe(baseline + 1);
	});
});

describe("queue read projections", () => {
	it("keeps a suppressed head in the canonical read and out of the effective one", () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		const head = buildQueuedMessage(10, "head");
		const tail = buildQueuedMessage(11, "tail");
		seedMessages(queryClient, [buildMessage(1, "user", "hello")], undefined, [
			head,
			tail,
		]);
		store.suppressQueuedMessageIDs([head.id]);

		// The send captures the head from the canonical read because the server
		// promotes ITS head by position, which still includes the hidden row.
		expect(readCanonicalQueuedMessages(queryClient, CHAT_ID)).toEqual([
			head,
			tail,
		]);
		expect(readEffectiveQueuedMessages(queryClient, store, CHAT_ID)).toEqual([
			tail,
		]);
	});

	it("reports no canonical queue for a chat with no cache entry", () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();

		expect(readCanonicalQueuedMessages(queryClient, CHAT_ID)).toBeUndefined();
		expect(readCanonicalQueuedMessages(queryClient, undefined)).toBeUndefined();
		expect(readEffectiveQueuedMessages(queryClient, store, CHAT_ID)).toEqual(
			[],
		);
	});
});

describe("shouldSuppressFinalizedOverlay", () => {
	it("keeps the overlay while no finalization is in progress", () => {
		expect(
			shouldSuppressFinalizedOverlay(null, [
				buildMessage(7, "assistant", "done"),
			]),
		).toBe(false);
	});

	it("suppresses the overlay once the exact finalized message is durable", () => {
		expect(
			shouldSuppressFinalizedOverlay(7, [
				buildMessage(6, "user", "ask"),
				buildMessage(7, "assistant", "done"),
			]),
		).toBe(true);
	});

	it("keeps the overlay while the finalized message is absent", () => {
		expect(
			shouldSuppressFinalizedOverlay(7, [
				buildMessage(5, "user", "ask"),
				buildMessage(6, "assistant", "earlier"),
			]),
		).toBe(false);
	});

	// A `maxDurableID >= finalizingMessageID` check would blank the tail here,
	// which is why membership is by exact ID.
	it("does not let a newer durable message suppress the overlay", () => {
		expect(
			shouldSuppressFinalizedOverlay(7, [
				buildMessage(6, "user", "ask"),
				buildMessage(9, "assistant", "newer"),
			]),
		).toBe(false);
	});

	it("keeps the overlay for an empty durable list", () => {
		expect(shouldSuppressFinalizedOverlay(7, [])).toBe(false);
	});
});
