import { act, render, renderHook, waitFor } from "@testing-library/react";
import type { FC, PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { describe, expect, it } from "vitest";
import { chatKeys } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	type ChatStore,
	createChatStore,
	selectMessagesByID,
	selectOrderedMessageIDs,
	selectQueuedMessages,
	useChatSelector,
} from "./chatStore";
import {
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
	it("reads messages and the queue from the store and status from the cache", () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		seedChatDetail(queryClient, "running");
		const messages = [
			buildMessage(1, "user", "hello"),
			buildMessage(2, "assistant", "hi"),
		];
		store.replaceMessages(messages);
		store.setQueuedMessages([buildQueuedMessage(10, "later")]);

		const { result } = renderHook(
			() => ({
				facadeStatus: useDurableChatStatus({ store, chatId: CHAT_ID }),
				facadeMessages: useDurableMessageList({ store, chatId: CHAT_ID }),
				selectorMessagesByID: useChatSelector(store, selectMessagesByID),
				selectorOrderedMessageIDs: useChatSelector(
					store,
					selectOrderedMessageIDs,
				),
				facadeCount: useDurableMessageCount({ store, chatId: CHAT_ID }),
				facadeQueued: useDurableQueuedMessages({ store, chatId: CHAT_ID }),
				selectorQueued: useChatSelector(store, selectQueuedMessages),
			}),
			{ wrapper: createWrapper(queryClient) },
		);

		const current = result.current;
		expect(current.facadeStatus).toBe("running");
		expect(current.facadeMessages).toEqual(
			current.selectorOrderedMessageIDs.map((id) =>
				current.selectorMessagesByID.get(id),
			),
		);
		expect(current.facadeMessages).toEqual(messages);
		expect(current.facadeCount).toBe(current.selectorMessagesByID.size);
		expect(current.facadeQueued).toBe(current.selectorQueued);
	});

	it("returns null when no chat is selected", () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		seedChatDetail(queryClient, "running");

		const { result } = renderHook(
			() => useDurableChatStatus({ store, chatId: undefined }),
			{ wrapper: createWrapper(queryClient) },
		);

		expect(result.current).toBeNull();
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
		store.replaceMessages([buildMessage(1, "user", "hello")]);

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

		act(() => {
			store.upsertDurableMessage(buildMessage(2, "assistant", "hi"));
		});

		expect(result.current.messages).not.toBe(firstMessages);
		expect(result.current.messages).toHaveLength(2);
	});

	it("does not re-render a queued-message consumer on a message_part update", () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		store.replaceMessages([buildMessage(1, "user", "hello")]);

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
		store.replaceMessages([buildMessage(1, "user", "hello")]);

		const { result } = renderHook(
			() => useIsAwaitingFirstStreamChunk({ store, chatId: CHAT_ID }),
			{ wrapper: createWrapper(queryClient) },
		);

		// The store half is satisfied (user message, no stream), the cached
		// status is not.
		expect(result.current).toBe(false);

		seedChatDetail(queryClient, "running", 2);

		await waitFor(() => {
			expect(result.current).toBe(true);
		});
	});

	it("re-renders the awaiting-first-chunk consumer only when the flag flips", () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		seedChatDetail(queryClient, "running");
		store.replaceMessages([buildMessage(1, "user", "hello")]);

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

	it("re-renders the message-count consumer only when the count changes", () => {
		const store = createChatStore();
		const queryClient = createTestQueryClient();
		store.replaceMessages([buildMessage(1, "user", "hello")]);

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

		act(() => {
			store.upsertDurableMessage(buildMessage(1, "user", "hello edited"));
		});

		expect(result.current).toBe(1);
		expect(renderCount).toBe(baseline);

		act(() => {
			store.upsertDurableMessage(buildMessage(2, "assistant", "hi"));
		});

		expect(result.current).toBe(2);
		expect(renderCount).toBe(baseline + 1);
	});
});
