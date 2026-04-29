import { QueryClient } from "react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import type {
	ChatStore,
	ChatStoreState,
} from "../components/ChatConversation/chatStore";
import { ChatSession } from "./ChatSession";
import {
	type ChatSessionFactory,
	ChatSessionManager,
} from "./ChatSessionManager";
import type {
	ChatSessionManagerRuntimeDeps,
	ChatSessionSnapshot,
	EnterForegroundParams,
	ReleaseVisibleParams,
} from "./types";

const createTestQueryClient = (): QueryClient =>
	new QueryClient({
		defaultOptions: {
			queries: {
				retry: false,
				gcTime: 0,
				refetchOnWindowFocus: false,
				networkMode: "offlineFirst",
			},
		},
	});

const makeRuntimeDeps = (): ChatSessionManagerRuntimeDeps => ({
	queryClient: createTestQueryClient(),
	setChatErrorReason: vi.fn(),
	clearChatErrorReason: vi.fn(),
});

const createStoreState = (
	chatStatus: TypesGen.ChatStatus | null = null,
): ChatStoreState => ({
	messagesByID: new Map(),
	orderedMessageIDs: [],
	streamState: null,
	chatStatus,
	streamError: null,
	retryState: null,
	reconnectState: null,
	queuedMessages: [],
	subagentStatusOverrides: new Map(),
});

const createSessionSnapshot = (): ChatSessionSnapshot => ({
	lifecycleMode: "inactive",
	followMode: true,
	viewportAnchor: null,
	hasNewOffscreenContent: false,
});

class FakeChatSession extends ChatSession {
	private storeState = createStoreState();

	private metadataSnapshot = createSessionSnapshot();

	private readonly fakeMetadataListeners = new Set<() => void>();

	private currentRetentionTimer: ReturnType<typeof setTimeout> | null = null;

	public readonly enterForegroundSpy =
		vi.fn<(params?: EnterForegroundParams) => void>();

	public readonly enterBackgroundNoReadSpy =
		vi.fn<(params?: ReleaseVisibleParams) => void>();

	public readonly disconnectSpy = vi.fn<() => void>();

	public readonly disposeSpy = vi.fn<() => void>();

	public readonly setRetentionTimerSpy =
		vi.fn<(handle: ReturnType<typeof setTimeout> | null) => void>();

	public readonly updateRuntimeDepsSpy =
		vi.fn<(next: ChatSessionManagerRuntimeDeps) => void>();

	public override readonly store: ChatStore = {
		getSnapshot: () => this.storeState,
		subscribe: () => () => {},
		batch: (fn) => {
			fn();
		},
		replaceMessages: () => {},
		upsertDurableMessage: () => ({ isDuplicate: false, changed: false }),
		upsertDurableMessages: () => {},
		applyMessagePart: () => {},
		applyMessageParts: () => {},
		setQueuedMessages: (queuedMessages) => {
			this.storeState = {
				...this.storeState,
				queuedMessages: queuedMessages ?? [],
			};
		},
		setChatStatus: (chatStatus) => {
			this.storeState = { ...this.storeState, chatStatus };
		},
		setStreamState: (streamState) => {
			this.storeState = { ...this.storeState, streamState };
		},
		setStreamError: (streamError) => {
			this.storeState = { ...this.storeState, streamError };
		},
		clearStreamError: () => {
			this.storeState = { ...this.storeState, streamError: null };
		},
		setRetryState: (retryState) => {
			this.storeState = { ...this.storeState, retryState };
		},
		clearRetryState: () => {
			this.storeState = { ...this.storeState, retryState: null };
		},
		setReconnectState: (reconnectState) => {
			this.storeState = { ...this.storeState, reconnectState };
		},
		clearReconnectState: () => {
			this.storeState = { ...this.storeState, reconnectState: null };
		},
		clearStreamState: () => {
			this.storeState = { ...this.storeState, streamState: null };
		},
		resetTransportReplayState: () => {},
		setSubagentStatusOverride: (chatID, status) => {
			const subagentStatusOverrides = new Map(
				this.storeState.subagentStatusOverrides,
			);
			subagentStatusOverrides.set(chatID, status);
			this.storeState = { ...this.storeState, subagentStatusOverrides };
		},
		resetTransientState: () => {
			this.storeState = {
				...this.storeState,
				streamState: null,
				streamError: null,
				retryState: null,
				reconnectState: null,
				subagentStatusOverrides: new Map(),
			};
		},
	};

	public override getSnapshot(): ChatSessionSnapshot {
		return this.metadataSnapshot;
	}

	public override subscribe(listener: () => void): () => void {
		this.fakeMetadataListeners.add(listener);
		return () => {
			this.fakeMetadataListeners.delete(listener);
		};
	}

	public override setFollowMode(followMode: boolean): void {
		if (this.metadataSnapshot.followMode === followMode) {
			return;
		}
		this.metadataSnapshot = { ...this.metadataSnapshot, followMode };
		this.emitMetadata();
	}

	public override enterForeground(params: EnterForegroundParams = {}): void {
		this.enterForegroundSpy(params);
		const now = params.now ?? Date.now();
		const { backgroundedAt: _backgroundedAt, ...nextSnapshot } =
			this.metadataSnapshot;
		this.metadataSnapshot = {
			...nextSnapshot,
			lifecycleMode: "foreground",
			lastVisibleAt: now,
		};
		this.emitMetadata();
	}

	public override enterBackgroundNoRead(
		params: ReleaseVisibleParams = {},
	): void {
		this.enterBackgroundNoReadSpy(params);
		const now = params.now ?? Date.now();
		this.metadataSnapshot = {
			...this.metadataSnapshot,
			lifecycleMode: "background",
			backgroundedAt: now,
		};
		this.emitMetadata();
	}

	public override disconnect(): void {
		this.disconnectSpy();
		if (this.metadataSnapshot.lifecycleMode === "inactive") {
			return;
		}
		const { backgroundedAt: _backgroundedAt, ...nextSnapshot } =
			this.metadataSnapshot;
		this.metadataSnapshot = {
			...nextSnapshot,
			lifecycleMode: "inactive",
		};
		this.emitMetadata();
	}

	public override dispose(): void {
		this.disposeSpy();
		if (this.currentRetentionTimer !== null) {
			clearTimeout(this.currentRetentionTimer);
			this.currentRetentionTimer = null;
		}
		this.fakeMetadataListeners.clear();
	}

	public override setRetentionTimer(
		handle: ReturnType<typeof setTimeout> | null,
	): void {
		this.setRetentionTimerSpy(handle);
		if (this.currentRetentionTimer !== null) {
			clearTimeout(this.currentRetentionTimer);
		}
		this.currentRetentionTimer = handle;
	}

	public override updateRuntimeDeps(next: ChatSessionManagerRuntimeDeps): void {
		this.updateRuntimeDepsSpy(next);
	}

	private emitMetadata(): void {
		for (const listener of this.fakeMetadataListeners) {
			listener();
		}
	}
}

type ManagerHarness = {
	manager: ChatSessionManager;
	sessions: Map<string, FakeChatSession>;
};

const setupManager = (): ManagerHarness => {
	const sessions = new Map<string, FakeChatSession>();
	const sessionFactory: ChatSessionFactory = (chatId, deps) => {
		const session = new FakeChatSession(chatId, deps);
		sessions.set(chatId, session);
		return session;
	};

	return {
		manager: new ChatSessionManager(makeRuntimeDeps(), sessionFactory),
		sessions,
	};
};

const getSession = (
	sessions: Map<string, FakeChatSession>,
	chatId: string,
): FakeChatSession => {
	const session = sessions.get(chatId);
	if (!session) {
		throw new Error(`Expected ${chatId} to have been created.`);
	}
	return session;
};

beforeEach(() => {
	vi.useFakeTimers();
	vi.setSystemTime(new Date("2025-01-01T00:00:00.000Z"));
});

afterEach(() => {
	vi.clearAllTimers();
	vi.useRealTimers();
	vi.restoreAllMocks();
});

describe("ChatSessionManager", () => {
	it("returns the same session on repeated getOrCreate calls", () => {
		const { manager } = setupManager();

		const first = manager.getOrCreate("chat-1");
		const second = manager.getOrCreate("chat-1");

		expect(second).toBe(first);
	});

	it("cancels pending retention and enters foreground when marked visible", () => {
		const { manager, sessions } = setupManager();
		const session = manager.getOrCreate("chat-1");
		session.store.setChatStatus("running");

		manager.releaseVisible("chat-1");
		expect(vi.getTimerCount()).toBe(1);

		manager.markVisible("chat-1");
		const fake = getSession(sessions, "chat-1");

		expect(vi.getTimerCount()).toBe(0);
		expect(fake.setRetentionTimerSpy).toHaveBeenLastCalledWith(null);
		expect(fake.enterForegroundSpy).toHaveBeenCalledTimes(1);

		vi.advanceTimersByTime(500);
		expect(fake.enterBackgroundNoReadSpy).not.toHaveBeenCalled();
	});

	it("recomputes retention eligibility from current follow mode and status", () => {
		const { manager, sessions } = setupManager();
		const session = manager.getOrCreate("chat-1");
		const fake = getSession(sessions, "chat-1");

		manager.releaseVisible("chat-1");
		expect(fake.disconnectSpy).toHaveBeenCalledTimes(1);
		expect(fake.enterBackgroundNoReadSpy).not.toHaveBeenCalled();

		session.store.setChatStatus("running");
		manager.releaseVisible("chat-1");
		expect(fake.enterBackgroundNoReadSpy).not.toHaveBeenCalled();
		vi.advanceTimersByTime(500);
		expect(fake.enterBackgroundNoReadSpy).toHaveBeenCalledTimes(1);

		fake.setFollowMode(false);
		manager.releaseVisible("chat-1");
		expect(fake.disconnectSpy).toHaveBeenCalledTimes(2);

		fake.setFollowMode(true);
		session.store.setChatStatus("completed");
		manager.releaseVisible("chat-1");
		expect(fake.disconnectSpy).toHaveBeenCalledTimes(3);

		vi.advanceTimersByTime(500);
		expect(fake.enterBackgroundNoReadSpy).toHaveBeenCalledTimes(1);
	});

	it("disconnects inactive sessions instead of retaining them when follow mode is false", () => {
		const { manager, sessions } = setupManager();
		const session = manager.getOrCreate("chat-1");
		const fake = getSession(sessions, "chat-1");
		session.store.setChatStatus("running");
		fake.setFollowMode(false);

		manager.releaseVisible("chat-1");

		expect(fake.disconnectSpy).toHaveBeenCalledTimes(1);
		expect(fake.enterBackgroundNoReadSpy).not.toHaveBeenCalled();
		expect(vi.getTimerCount()).toBe(0);

		vi.advanceTimersByTime(500);
		expect(fake.enterBackgroundNoReadSpy).not.toHaveBeenCalled();
	});

	it("keeps a visible chat out of the background during the debounce", () => {
		const { manager, sessions } = setupManager();
		const session = manager.getOrCreate("chat-1");
		session.store.setChatStatus("running");
		const fake = getSession(sessions, "chat-1");

		manager.releaseVisible("chat-1");
		vi.advanceTimersByTime(499);
		manager.markVisible("chat-1");
		vi.advanceTimersByTime(1);

		expect(fake.enterBackgroundNoReadSpy).not.toHaveBeenCalled();
		expect(fake.enterForegroundSpy).toHaveBeenCalledTimes(1);
	});

	it("moves eligible chats to background and returns them to foreground", () => {
		const { manager, sessions } = setupManager();
		const session = manager.getOrCreate("chat-1");
		session.store.setChatStatus("pending");
		const fake = getSession(sessions, "chat-1");

		manager.releaseVisible("chat-1");
		vi.advanceTimersByTime(500);
		expect(fake.enterBackgroundNoReadSpy).toHaveBeenCalledTimes(1);
		expect(fake.getSnapshot().lifecycleMode).toBe("background");

		manager.markVisible("chat-1");
		expect(fake.enterForegroundSpy).toHaveBeenCalledTimes(1);
		expect(fake.getSnapshot().lifecycleMode).toBe("foreground");
	});

	it("demotes the oldest active retained session when active capacity is exceeded", () => {
		const { manager, sessions } = setupManager();
		for (let index = 1; index <= 6; index += 1) {
			const chatId = `chat-${index}`;
			const session = manager.getOrCreate(chatId);
			session.store.setChatStatus("running");
			manager.releaseVisible(chatId);
			vi.advanceTimersByTime(500);
		}

		const oldest = getSession(sessions, "chat-1");
		expect(oldest.disconnectSpy).toHaveBeenCalledTimes(1);
		expect(oldest.disposeSpy).not.toHaveBeenCalled();
		expect(manager.getOrCreate("chat-1")).toBe(oldest);
	});

	it("disposes the oldest active retained session when warm capacity is full", () => {
		const { manager, sessions } = setupManager();
		for (let index = 1; index <= 10; index += 1) {
			manager.releaseVisible(`warm-${index}`);
		}
		for (let index = 1; index <= 6; index += 1) {
			const chatId = `active-${index}`;
			const session = manager.getOrCreate(chatId);
			session.store.setChatStatus("running");
			manager.releaseVisible(chatId);
			vi.advanceTimersByTime(500);
		}

		const oldest = getSession(sessions, "active-1");
		expect(oldest.disconnectSpy).toHaveBeenCalledTimes(1);
		expect(oldest.disposeSpy).toHaveBeenCalledTimes(1);
		expect(manager.getOrCreate("active-1")).not.toBe(oldest);
	});

	it("disposes the oldest warm inactive session when warm capacity is exceeded", () => {
		const { manager, sessions } = setupManager();
		for (let index = 1; index <= 11; index += 1) {
			manager.releaseVisible(`chat-${index}`);
		}

		const oldest = getSession(sessions, "chat-1");
		expect(oldest.disposeSpy).toHaveBeenCalledTimes(1);
		expect(manager.getOrCreate("chat-1")).not.toBe(oldest);
	});

	it("disposes all sessions and cancels pending retention timers", () => {
		const { manager, sessions } = setupManager();
		for (let index = 1; index <= 3; index += 1) {
			const chatId = `chat-${index}`;
			const session = manager.getOrCreate(chatId);
			session.store.setChatStatus("running");
			manager.releaseVisible(chatId);
		}

		expect(vi.getTimerCount()).toBe(3);
		manager.dispose();
		expect(vi.getTimerCount()).toBe(0);

		for (let index = 1; index <= 3; index += 1) {
			const fake = getSession(sessions, `chat-${index}`);
			expect(fake.setRetentionTimerSpy).toHaveBeenLastCalledWith(null);
			expect(fake.disposeSpy).toHaveBeenCalledTimes(1);
		}

		vi.advanceTimersByTime(500);
		for (let index = 1; index <= 3; index += 1) {
			const fake = getSession(sessions, `chat-${index}`);
			expect(fake.enterBackgroundNoReadSpy).not.toHaveBeenCalled();
		}
	});
});
