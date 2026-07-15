import type { ChatWatchProjection } from "#/api/queries/chats";
import type { OneWayMessageEvent } from "#/utils/OneWayWebSocket";
import {
	type ChatProjectionHintReconcilerPorts,
	createChatProjectionHintFreshnessCoordinator,
	type DirtyChatProjectionHints,
	decodeChatProjectionHint,
	reconcileChatProjectionHint,
	shouldInvalidateFilteredChatList,
	subscribeChatProjectionHints,
} from "./chatProjectionHints";

const makeChat = (overrides: Record<string, unknown> = {}) =>
	({
		id: "chat-1",
		status: "waiting",
		archived: false,
		parent_chat_id: null,
		...overrides,
	}) as unknown as ChatWatchProjection["chat"];

const makeHint = (
	kind: ChatWatchProjection["kind"],
	chat = makeChat(),
): ChatWatchProjection => ({ kind, chat });

const createMockSocket = () => {
	const listeners = new Map<string, Array<(payload: never) => void>>();
	return {
		url: "ws://test.invalid/api/experimental/chats/watch",
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
		emitHint(hint: ChatWatchProjection) {
			this.emit("message", {
				parsedMessage: hint,
				parseError: undefined,
				sourceEvent: new MessageEvent("message"),
			} satisfies OneWayMessageEvent<ChatWatchProjection>);
		},
	};
};

const createPorts = (): ChatProjectionHintReconcilerPorts => ({
	getPreviousStatus: vi.fn(() => "running" as const),
	playChime: vi.fn(),
	removeDeletedChat: vi.fn(),
	invalidateDiff: vi.fn(),
	cancelListRefetches: vi.fn(),
	hasCachedDetail: vi.fn(() => true),
	cancelDetailRefetch: vi.fn(),
	addChild: vi.fn(),
	prependRoot: vi.fn(),
	mergeProjection: vi.fn(),
	invalidateCollections: vi.fn(),
	repairParent: vi.fn(),
	invalidateDetail: vi.fn(),
});

describe("decodeChatProjectionHint", () => {
	it("accepts a generated projection hint", () => {
		const hint = makeHint("status_change");
		expect(decodeChatProjectionHint(hint)).toBe(hint);
	});

	it.each([
		null,
		{},
		{ kind: "unknown", chat: makeChat() },
		{ kind: "created" },
	])("rejects malformed input %#", (value) => {
		expect(decodeChatProjectionHint(value)).toBeInstanceOf(Error);
	});
});

describe("subscribeChatProjectionHints", () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("fences late messages from superseded connections", () => {
		const sockets = [createMockSocket(), createMockSocket()];
		const connect = vi.fn(() => sockets[connect.mock.calls.length - 1]);
		const onHint = vi.fn();
		const onOpen = vi.fn();
		const dispose = subscribeChatProjectionHints({
			connect,
			onHint,
			onOpen,
			baseMs: 1,
			jitter: 0,
		});

		sockets[0].emit("open");
		sockets[0].emitHint(makeHint("created"));
		expect(onOpen).toHaveBeenCalledWith(1);
		expect(onHint).toHaveBeenCalledTimes(1);

		sockets[0].emit("close");
		vi.advanceTimersByTime(1);
		sockets[1].emit("open");
		sockets[0].emitHint(makeHint("deleted"));
		sockets[1].emitHint(makeHint("title_change"));

		expect(onOpen).toHaveBeenLastCalledWith(2);
		expect(onHint).toHaveBeenCalledTimes(2);
		expect(onHint).toHaveBeenLastCalledWith(makeHint("title_change"), 2);
		dispose();
	});

	it("ignores messages after disposal", () => {
		const socket = createMockSocket();
		const onHint = vi.fn();
		const dispose = subscribeChatProjectionHints({
			connect: () => socket,
			onHint,
		});
		dispose();
		socket.emitHint(makeHint("created"));
		expect(onHint).not.toHaveBeenCalled();
	});
});

const createDeferred = () => {
	let resolve!: () => void;
	const promise = new Promise<void>((resolvePromise) => {
		resolve = resolvePromise;
	});
	return { promise, resolve };
};

describe(createChatProjectionHintFreshnessCoordinator.name, () => {
	it("records dirty IDs and kinds without replaying baseline payloads", async () => {
		const baseline = createDeferred();
		const reconcileLiveHint = vi.fn();
		const resynchronizeDirty = vi.fn(
			async (_dirty: DirtyChatProjectionHints) => undefined,
		);
		const coordinator = createChatProjectionHintFreshnessCoordinator({
			reconcileLiveHint,
			resynchronizeBaseline: () => baseline.promise,
			resynchronizeDirty,
		});

		coordinator.onOpen(1);
		coordinator.onHint(makeHint("title_change"), 1);
		coordinator.onHint(makeHint("status_change"), 1);
		expect(reconcileLiveHint).not.toHaveBeenCalled();

		baseline.resolve();
		await vi.waitFor(() => expect(resynchronizeDirty).toHaveBeenCalledOnce());
		const dirty = resynchronizeDirty.mock.calls[0][0];
		expect(dirty.get("chat-1")).toEqual(
			new Set(["title_change", "status_change"]),
		);

		await vi.waitFor(() => {
			coordinator.onHint(makeHint("context_dirty"), 1);
			expect(reconcileLiveHint).toHaveBeenCalledWith(makeHint("context_dirty"));
		});
	});

	it("stays synchronized-off when the REST baseline fails", async () => {
		const reconcileLiveHint = vi.fn();
		const onError = vi.fn();
		const coordinator = createChatProjectionHintFreshnessCoordinator({
			reconcileLiveHint,
			resynchronizeBaseline: async () => {
				throw new Error("baseline failed");
			},
			resynchronizeDirty: async () => undefined,
			onError,
		});

		coordinator.onOpen(1);
		await vi.waitFor(() => expect(onError).toHaveBeenCalledOnce());
		coordinator.onHint(makeHint("title_change"), 1);
		expect(reconcileLiveHint).not.toHaveBeenCalled();
	});

	it("drains hints that race the dirty refetch before entering live mode", async () => {
		const baseline = createDeferred();
		const firstDirtyPass = createDeferred();
		const reconcileLiveHint = vi.fn();
		const resynchronizeDirty = vi
			.fn<(dirty: DirtyChatProjectionHints) => Promise<void>>()
			.mockImplementationOnce(() => firstDirtyPass.promise)
			.mockResolvedValue(undefined);
		const coordinator = createChatProjectionHintFreshnessCoordinator({
			reconcileLiveHint,
			resynchronizeBaseline: () => baseline.promise,
			resynchronizeDirty,
		});

		coordinator.onOpen(1);
		coordinator.onHint(makeHint("title_change"), 1);
		baseline.resolve();
		await vi.waitFor(() => expect(resynchronizeDirty).toHaveBeenCalledTimes(1));

		coordinator.onHint(
			makeHint("status_change", makeChat({ id: "chat-2" })),
			1,
		);
		firstDirtyPass.resolve();
		await vi.waitFor(() => expect(resynchronizeDirty).toHaveBeenCalledTimes(2));
		expect(resynchronizeDirty.mock.calls[1][0].get("chat-2")).toEqual(
			new Set(["status_change"]),
		);
		expect(reconcileLiveHint).not.toHaveBeenCalled();
	});

	it("does not let an older connection baseline release a newer epoch", async () => {
		const baselines = [createDeferred(), createDeferred()];
		const reconcileLiveHint = vi.fn();
		const resynchronizeDirty = vi.fn(
			async (_dirty: DirtyChatProjectionHints) => undefined,
		);
		let baselineIndex = 0;
		const coordinator = createChatProjectionHintFreshnessCoordinator({
			reconcileLiveHint,
			resynchronizeBaseline: () => baselines[baselineIndex++].promise,
			resynchronizeDirty,
		});

		coordinator.onOpen(1);
		coordinator.onHint(makeHint("title_change"), 1);
		coordinator.onOpen(2);
		coordinator.onHint(
			makeHint("status_change", makeChat({ id: "chat-2" })),
			2,
		);
		baselines[0].resolve();
		await Promise.resolve();
		expect(resynchronizeDirty).not.toHaveBeenCalled();
		expect(reconcileLiveHint).not.toHaveBeenCalled();

		baselines[1].resolve();
		await vi.waitFor(() => expect(resynchronizeDirty).toHaveBeenCalledOnce());
		expect(resynchronizeDirty.mock.calls[0][0].has("chat-1")).toBe(false);
		expect(resynchronizeDirty.mock.calls[0][0].get("chat-2")).toEqual(
			new Set(["status_change"]),
		);
	});
});

describe("reconcileChatProjectionHint", () => {
	it("maps root creation to prepend and list invalidation", () => {
		const ports = createPorts();
		const hint = makeHint("created");
		reconcileChatProjectionHint({ hint, activeChatID: "other", ports });
		expect(ports.playChime).toHaveBeenCalledWith(
			"running",
			"waiting",
			"chat-1",
			"other",
		);
		expect(ports.prependRoot).toHaveBeenCalledWith(hint.chat);
		expect(ports.invalidateCollections).toHaveBeenCalled();
		expect(ports.mergeProjection).not.toHaveBeenCalled();
	});

	it("maps child creation without a chime", () => {
		const ports = createPorts();
		const hint = makeHint("created", makeChat({ parent_chat_id: "parent-1" }));
		reconcileChatProjectionHint({ hint, activeChatID: undefined, ports });
		expect(ports.playChime).not.toHaveBeenCalled();
		expect(ports.addChild).toHaveBeenCalledWith(hint.chat, "parent-1");
	});

	it("maps deletion without applying later merge effects", () => {
		const ports = createPorts();
		const hint = makeHint("deleted");
		reconcileChatProjectionHint({ hint, activeChatID: undefined, ports });
		expect(ports.removeDeletedChat).toHaveBeenCalledWith(hint.chat);
		expect(ports.cancelListRefetches).not.toHaveBeenCalled();
		expect(ports.mergeProjection).not.toHaveBeenCalled();
	});

	it("treats action_required as an explicit membership-affecting hint", () => {
		const ports = createPorts();
		const hint = makeHint(
			"action_required",
			makeChat({ status: "requires_action" }),
		);
		reconcileChatProjectionHint({ hint, activeChatID: "chat-1", ports });
		expect(ports.mergeProjection).toHaveBeenCalledWith(
			hint.chat,
			"action_required",
			"chat-1",
		);
		expect(ports.invalidateCollections).toHaveBeenCalled();
	});
});

describe(shouldInvalidateFilteredChatList.name, () => {
	it.each([
		["status_change", true],
		["diff_status_change", true],
		["action_required", true],
		["title_change", false],
	] as const)("handles %s", (kind, expected) => {
		expect(shouldInvalidateFilteredChatList(makeChat(), kind)).toBe(expected);
	});

	it("excludes child chats", () => {
		expect(
			shouldInvalidateFilteredChatList(
				makeChat({ parent_chat_id: "parent-1" }),
				"status_change",
			),
		).toBe(false);
	});
});
