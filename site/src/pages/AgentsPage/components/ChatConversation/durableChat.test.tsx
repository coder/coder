import { act, render, renderHook } from "@testing-library/react";
import type { FC } from "react";
import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	type ChatStore,
	createChatStore,
	selectChatStatus,
	selectIsAwaitingFirstStreamChunk,
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
	it("matches the underlying store selectors", () => {
		const store = createChatStore();
		const messages = [
			buildMessage(1, "user", "hello"),
			buildMessage(2, "assistant", "hi"),
		];
		store.replaceMessages(messages);
		store.setQueuedMessages([buildQueuedMessage(10, "later")]);
		store.setChatStatus("running");

		const { result } = renderHook(() => ({
			facadeStatus: useDurableChatStatus({ store, chatId: CHAT_ID }),
			selectorStatus: useChatSelector(store, selectChatStatus),
			facadeMessages: useDurableMessageList({ store, chatId: CHAT_ID }),
			selectorMessagesByID: useChatSelector(store, selectMessagesByID),
			selectorOrderedMessageIDs: useChatSelector(
				store,
				selectOrderedMessageIDs,
			),
			facadeCount: useDurableMessageCount({ store, chatId: CHAT_ID }),
			facadeQueued: useDurableQueuedMessages({ store, chatId: CHAT_ID }),
			selectorQueued: useChatSelector(store, selectQueuedMessages),
			facadeAwaiting: useIsAwaitingFirstStreamChunk({
				store,
				chatId: CHAT_ID,
			}),
			selectorAwaiting: useChatSelector(
				store,
				selectIsAwaitingFirstStreamChunk,
			),
		}));

		const current = result.current;
		expect(current.facadeStatus).toBe(current.selectorStatus);
		expect(current.facadeMessages).toEqual(
			current.selectorOrderedMessageIDs.map((id) =>
				current.selectorMessagesByID.get(id),
			),
		);
		expect(current.facadeMessages).toEqual(messages);
		expect(current.facadeCount).toBe(current.selectorMessagesByID.size);
		expect(current.facadeQueued).toBe(current.selectorQueued);
		expect(current.facadeAwaiting).toBe(current.selectorAwaiting);
	});

	it("keeps the message list reference stable when an unrelated field changes", () => {
		const store = createChatStore();
		store.replaceMessages([buildMessage(1, "user", "hello")]);

		const { result } = renderHook(() => ({
			messages: useDurableMessageList({ store, chatId: CHAT_ID }),
			status: useDurableChatStatus({ store, chatId: CHAT_ID }),
		}));

		const firstMessages = result.current.messages;

		act(() => {
			store.setChatStatus("running");
		});

		expect(result.current.status).toBe("running");
		expect(result.current.messages).toBe(firstMessages);

		act(() => {
			store.upsertDurableMessage(buildMessage(2, "assistant", "hi"));
		});

		expect(result.current.messages).not.toBe(firstMessages);
		expect(result.current.messages).toHaveLength(2);
	});

	it("does not re-render a queued-message consumer on a message_part update", () => {
		const store = createChatStore();
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

		render(
			<>
				<QueueProbe store={store} />
				<MessageProbe store={store} />
			</>,
		);

		const queueBaseline = queueRenderCount;
		const messageBaseline = messageRenderCount;

		act(() => {
			store.applyMessagePart(buildTextPart("partial"));
		});

		expect(queueRenderCount).toBe(queueBaseline);
		expect(messageRenderCount).toBe(messageBaseline);
	});

	it("re-renders the awaiting-first-chunk consumer only when the flag flips", () => {
		const store = createChatStore();
		store.replaceMessages([buildMessage(1, "user", "hello")]);
		store.setChatStatus("running");

		let renderCount = 0;
		const { result } = renderHook(() => {
			const isAwaiting = useIsAwaitingFirstStreamChunk({
				store,
				chatId: CHAT_ID,
			});
			renderCount += 1;
			return isAwaiting;
		});

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
		store.replaceMessages([buildMessage(1, "user", "hello")]);

		let renderCount = 0;
		const { result } = renderHook(() => {
			const count = useDurableMessageCount({ store, chatId: CHAT_ID });
			renderCount += 1;
			return count;
		});

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
