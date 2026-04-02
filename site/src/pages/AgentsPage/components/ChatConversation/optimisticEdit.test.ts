import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { createChatStore } from "./chatStore";
import {
	buildOptimisticEditedMessage,
	getSavingMessageId,
	getVisibleConversation,
	hasConvergedOptimisticEditSession,
	isExpectedOptimisticEditDivergence,
	projectAuthoritativeEditedConversation,
	truncateMessagesForEdit,
} from "./optimisticEdit";

const makeMessage = (
	id: number,
	role: TypesGen.ChatMessage["role"],
	text: string,
	chatID = "chat-1",
): TypesGen.ChatMessage => ({
	id,
	chat_id: chatID,
	created_at: `2025-01-01T00:00:${String(id).padStart(2, "0")}Z`,
	role,
	content: [{ type: "text", text }],
});

describe("optimisticEdit", () => {
	it("derives the visible conversation slice from a store snapshot", () => {
		const store = createChatStore();
		const user = makeMessage(1, "user", "question");
		const assistant = makeMessage(2, "assistant", "answer");
		const queued = {
			id: 10,
			chat_id: "chat-1",
			created_at: "2025-01-01T00:00:10Z",
			content: [{ type: "text", text: "queued" }],
		} satisfies TypesGen.ChatQueuedMessage;
		const streamState = {
			blocks: [{ type: "response" as const, text: "streaming" }],
			toolCalls: {},
			toolResults: {},
			sources: [],
		};

		store.replaceMessages([assistant, user]);
		store.setQueuedMessages([queued]);
		store.setChatStatus("pending");
		store.setStreamState(streamState);

		const visibleConversation = getVisibleConversation(store.getSnapshot());
		expect(visibleConversation.messages.map((message) => message.id)).toEqual([
			1, 2,
		]);
		expect(visibleConversation.queuedMessages).toEqual([queued]);
		expect(visibleConversation.chatStatus).toBe("pending");
		expect(visibleConversation.streamState).toBe(streamState);
	});

	it("truncates every message at or after the edited message id", () => {
		const ascendingMessages = [1, 2, 3, 4].map((id) =>
			makeMessage(id, id % 2 === 0 ? "assistant" : "user", `msg ${id}`),
		);
		const descendingMessages = [...ascendingMessages].reverse();

		expect(
			truncateMessagesForEdit(ascendingMessages, 3).map((m) => m.id),
		).toEqual([1, 2]);
		expect(
			truncateMessagesForEdit(descendingMessages, 3).map((m) => m.id),
		).toEqual([2, 1]);
	});

	it("builds the optimistic and authoritative replacement messages", () => {
		const originalMessage = makeMessage(3, "user", "question");
		const optimisticMessage = buildOptimisticEditedMessage(originalMessage, [
			{ type: "text", text: "edited question" },
		]);
		const responseMessage = makeMessage(9, "user", "authoritative question");

		expect(optimisticMessage.id).toBe(originalMessage.id);
		expect(optimisticMessage.content).toEqual([
			{ type: "text", text: "edited question" },
		]);
		expect(
			projectAuthoritativeEditedConversation(
				truncateMessagesForEdit(
					[makeMessage(1, "user", "before"), originalMessage],
					3,
				),
				responseMessage,
			).map((message) => message.id),
		).toEqual([1, 9]);
	});

	it("recognizes the optimistic-phase divergence", () => {
		const visibleMessages = [
			makeMessage(1, "user", "before"),
			makeMessage(2, "assistant", "answer"),
			buildOptimisticEditedMessage(makeMessage(3, "user", "question"), [
				{ type: "text", text: "edited question" },
			]),
		];

		expect(
			isExpectedOptimisticEditDivergence({
				visibleMessages,
				fetchedMessages: truncateMessagesForEdit(visibleMessages, 3),
				optimisticEditSession: {
					token: Symbol("optimistic-edit"),
					editedMessageId: 3,
					visibleMessageId: 3,
					phase: "optimistic",
				},
			}),
		).toBe(true);
		expect(
			getSavingMessageId({
				token: Symbol("optimistic-edit"),
				editedMessageId: 3,
				visibleMessageId: 3,
				phase: "optimistic",
			}),
		).toBe(3);
	});

	it("recognizes the authoritative-phase divergence while the cache is still truncated", () => {
		const visibleMessages = [
			makeMessage(1, "user", "before"),
			makeMessage(2, "assistant", "answer"),
			makeMessage(9, "user", "authoritative question"),
		];
		const optimisticEditSession = {
			token: Symbol("optimistic-edit"),
			editedMessageId: 3,
			visibleMessageId: 9,
			phase: "authoritative" as const,
		};

		expect(
			isExpectedOptimisticEditDivergence({
				visibleMessages,
				fetchedMessages: [
					makeMessage(1, "user", "before"),
					makeMessage(2, "assistant", "answer"),
				],
				optimisticEditSession,
			}),
		).toBe(true);
		expect(getSavingMessageId(optimisticEditSession)).toBeNull();
	});

	it("detects when fetched data has caught up to the visible replacement message", () => {
		const optimisticEditSession = {
			token: Symbol("optimistic-edit"),
			editedMessageId: 3,
			visibleMessageId: 9,
			phase: "authoritative" as const,
		};

		expect(
			hasConvergedOptimisticEditSession({
				fetchedMessages: [
					makeMessage(1, "user", "before"),
					makeMessage(2, "assistant", "answer"),
					makeMessage(9, "user", "authoritative question"),
				],
				optimisticEditSession,
			}),
		).toBe(true);
		expect(
			hasConvergedOptimisticEditSession({
				fetchedMessages: [
					makeMessage(1, "user", "before"),
					makeMessage(2, "assistant", "answer"),
				],
				optimisticEditSession,
			}),
		).toBe(false);
	});

	it("does not match unrelated stale-entry differences", () => {
		const visibleMessages = [
			makeMessage(1, "user", "before"),
			makeMessage(2, "assistant", "answer"),
			makeMessage(9, "user", "authoritative question"),
		];

		expect(
			isExpectedOptimisticEditDivergence({
				visibleMessages,
				fetchedMessages: [
					makeMessage(1, "user", "before"),
					makeMessage(4, "assistant", "unexpected"),
				],
				optimisticEditSession: {
					token: Symbol("optimistic-edit"),
					editedMessageId: 3,
					visibleMessageId: 9,
					phase: "authoritative",
				},
			}),
		).toBe(false);
	});
});
