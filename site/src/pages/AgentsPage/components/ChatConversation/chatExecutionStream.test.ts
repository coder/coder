import type { ChatExecutionSnapshotEvent } from "#/api/queries/chats";
import type { OneWayMessageEvent } from "#/utils/OneWayWebSocket";
import {
	subscribeChatExecutionStream,
	toChatPreviewPart,
} from "./chatExecutionStream";

const createMockSocket = () => {
	const listeners = new Map<string, Array<(payload: never) => void>>();
	return {
		url: "ws://test.invalid/api/experimental/chats/chat-1/stream",
		addEventListener: vi.fn(
			(event: string, callback: (payload: never) => void) => {
				const callbacks = listeners.get(event) ?? [];
				callbacks.push(callback);
				listeners.set(event, callbacks);
			},
		),
		removeEventListener: vi.fn(),
		close: vi.fn(),
		emit(event: string, payload?: unknown) {
			for (const callback of listeners.get(event) ?? []) {
				callback(payload as never);
			}
		},
		emitBatch(events: readonly ChatExecutionSnapshotEvent[]) {
			this.emit("message", {
				parsedMessage: [...events],
				parseError: undefined,
				sourceEvent: new MessageEvent("message"),
			} satisfies OneWayMessageEvent<ChatExecutionSnapshotEvent[]>);
		},
	};
};

const statusEvent = (chatID = "chat-1"): ChatExecutionSnapshotEvent => ({
	type: "status",
	chat_id: chatID,
	status: { status: "running" },
});

describe("subscribeChatExecutionStream", () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("reads the latest committed message ID on reconnect", () => {
		const sockets = [createMockSocket(), createMockSocket()];
		let afterMessageID = 10;
		const connect = vi.fn(() => sockets[connect.mock.calls.length - 1]);
		const dispose = subscribeChatExecutionStream({
			chatID: "chat-1",
			getAfterMessageID: () => afterMessageID,
			connect,
			onBatch: vi.fn(),
			baseMs: 1,
			jitter: 0,
		});
		expect(connect).toHaveBeenCalledWith("chat-1", 10);
		afterMessageID = 20;
		sockets[0].emit("close");
		vi.advanceTimersByTime(1);
		expect(connect).toHaveBeenLastCalledWith("chat-1", 20);
		dispose();
	});

	it("uses a caller-provided monotonic connection epoch", () => {
		const socket = createMockSocket();
		const onBatch = vi.fn();
		const dispose = subscribeChatExecutionStream({
			chatID: "chat-1",
			getAfterMessageID: () => undefined,
			connect: () => socket,
			nextConnectionEpoch: () => 7,
			onBatch,
		});

		socket.emitBatch([statusEvent()]);
		expect(onBatch).toHaveBeenCalledWith([statusEvent()], 7);
		dispose();
	});

	it("fences late events from superseded connections", () => {
		const sockets = [createMockSocket(), createMockSocket()];
		const connect = vi.fn(() => sockets[connect.mock.calls.length - 1]);
		const onBatch = vi.fn();
		const dispose = subscribeChatExecutionStream({
			chatID: "chat-1",
			getAfterMessageID: () => undefined,
			connect,
			onBatch,
			baseMs: 1,
			jitter: 0,
		});
		sockets[0].emitBatch([statusEvent()]);
		sockets[0].emit("close");
		vi.advanceTimersByTime(1);
		sockets[0].emitBatch([statusEvent()]);
		sockets[1].emitBatch([statusEvent()]);
		expect(onBatch).toHaveBeenCalledTimes(2);
		expect(onBatch).toHaveBeenLastCalledWith([statusEvent()], 2);
		dispose();
	});

	it("closes and fences a connection after a decode failure", () => {
		const socket = createMockSocket();
		const onBatch = vi.fn();
		const onDecodeError = vi.fn();
		const dispose = subscribeChatExecutionStream({
			chatID: "chat-1",
			getAfterMessageID: () => undefined,
			connect: () => socket,
			onBatch,
			onDecodeError,
		});

		socket.emit("message", {
			parsedMessage: undefined,
			parseError: new Error("broken"),
			sourceEvent: new MessageEvent("message"),
		});
		socket.emitBatch([statusEvent()]);

		expect(onDecodeError).toHaveBeenCalledOnce();
		expect(socket.close).toHaveBeenCalledOnce();
		expect(onBatch).not.toHaveBeenCalled();
		dispose();
	});

	it.each([
		[{ type: "unknown", chat_id: "chat-1" }],
		[{ type: "status", chat_id: "chat-1" }],
		[{ type: "message", chat_id: "chat-1" }],
	])("closes on a contract-invalid event %#", (event) => {
		const socket = createMockSocket();
		const onDecodeError = vi.fn();
		const dispose = subscribeChatExecutionStream({
			chatID: "chat-1",
			getAfterMessageID: () => undefined,
			connect: () => socket,
			onBatch: vi.fn(),
			onDecodeError,
		});

		socket.emitBatch([event as ChatExecutionSnapshotEvent]);
		expect(onDecodeError).toHaveBeenCalledOnce();
		expect(socket.close).toHaveBeenCalledOnce();
		dispose();
	});

	it("rejects foreign chat events instead of treating them as subagent state", () => {
		const socket = createMockSocket();
		const onBatch = vi.fn();
		const dispose = subscribeChatExecutionStream({
			chatID: "chat-1",
			getAfterMessageID: () => undefined,
			connect: () => socket,
			onBatch,
		});
		socket.emitBatch([statusEvent("child-chat")]);
		expect(onBatch).not.toHaveBeenCalled();
		dispose();
	});
});

describe("toChatPreviewPart", () => {
	it("preserves connection and backend episode identity", () => {
		const event: ChatExecutionSnapshotEvent = {
			type: "message_part",
			chat_id: "chat-1",
			message_part: {
				role: "assistant",
				history_version: 4,
				generation_attempt: 2,
				seq: 9,
				part: { type: "text", text: "hello" },
			},
		};
		expect(toChatPreviewPart(event, 3)).toEqual({
			connectionEpoch: 3,
			historyVersion: 4,
			generationAttempt: 2,
			seq: 9,
			role: "assistant",
			part: { type: "text", text: "hello" },
		});
	});
});
