import {
	createContext,
	type Dispatch,
	type MutableRefObject,
	type ReactNode,
	type SetStateAction,
	useCallback,
	useContext,
	useEffect,
	useMemo,
	useRef,
	useState,
	useSyncExternalStore,
} from "react";
import {
	type ChatModelOption,
	type ChatPreferenceStore,
	type ChatRuntime,
	type ChatStore,
	type ChatStoreState,
	type ChatStreamEvent,
	createChatStore,
} from "../core";

type AdaptedChatPreferenceStore = ChatPreferenceStore & {
	subscribe: (key: string, cb: () => void) => () => void;
};

type ChatModelCacheState = {
	requestID: number;
	status: "loading" | "success" | "error";
	promise: Promise<readonly ChatModelOption[]> | null;
	models: readonly ChatModelOption[];
	error: unknown;
};

type ChatRuntimeContextValue = {
	runtime: ChatRuntime;
	preferenceStore: AdaptedChatPreferenceStore;
	store: ChatStore;
	activeChatId: string | null;
	setActiveChatId: Dispatch<SetStateAction<string | null>>;
	lastDurableMessageId: MutableRefObject<number | undefined>;
	selectionToken: MutableRefObject<number>;
	modelCache: MutableRefObject<ChatModelCacheState | null>;
};

/** @public Props for the shared React chat runtime provider. */
export type ChatRuntimeProviderProps = {
	children: ReactNode;
	runtime: ChatRuntime;
	preferenceStore: ChatPreferenceStore;
};

const ChatRuntimeContext = createContext<ChatRuntimeContextValue | null>(null);

const createAdaptedPreferenceStore = (
	backingStore: ChatPreferenceStore,
): AdaptedChatPreferenceStore => {
	const localListeners = new Map<string, Set<() => void>>();

	const subscribe = (key: string, cb: () => void): (() => void) => {
		if (backingStore.subscribe) {
			return backingStore.subscribe(key, cb);
		}
		const listenersForKey = localListeners.get(key) ?? new Set<() => void>();
		listenersForKey.add(cb);
		localListeners.set(key, listenersForKey);
		return () => {
			const existingListeners = localListeners.get(key);
			if (!existingListeners) {
				return;
			}
			existingListeners.delete(cb);
			if (existingListeners.size === 0) {
				localListeners.delete(key);
			}
		};
	};

	const notifyKey = (key: string): void => {
		const listenersForKey = localListeners.get(key);
		if (!listenersForKey) {
			return;
		}
		for (const listener of listenersForKey) {
			listener();
		}
	};

	return {
		get<T>(key: string, fallback: T): T {
			return backingStore.get(key, fallback);
		},
		set<T>(key: string, value: T): void {
			backingStore.set(key, value);
			if (!backingStore.subscribe) {
				notifyKey(key);
			}
		},
		subscribe,
	};
};

const isMismatchedChatID = (
	eventChatID: string | undefined,
	activeChatID: string,
): boolean => Boolean(eventChatID && eventChatID !== activeChatID);

const shouldApplyMessageParts = (store: ChatStore): boolean => {
	const currentStatus = store.getSnapshot().chatStatus;
	return currentStatus !== "pending" && currentStatus !== "waiting";
};

/** @public Provides the shared React chat runtime context. */
export const ChatRuntimeProvider = ({
	children,
	runtime,
	preferenceStore,
}: ChatRuntimeProviderProps) => {
	const storeRef = useRef<ChatStore | null>(null);
	if (storeRef.current === null) {
		storeRef.current = createChatStore();
	}

	const adaptedPreferenceStore = useMemo(
		() => createAdaptedPreferenceStore(preferenceStore),
		[preferenceStore],
	);

	const [activeChatId, setActiveChatId] = useState<string | null>(null);
	const lastDurableMessageId = useRef<number | undefined>(undefined);
	const selectionToken = useRef(0);
	const modelCache = useRef<ChatModelCacheState | null>(null);
	const deferredStreamClearTimeoutRef = useRef<ReturnType<
		typeof setTimeout
	> | null>(null);
	const store = storeRef.current;

	if (store === null) {
		throw new Error("ChatRuntimeProvider failed to initialize the chat store.");
	}

	const clearDeferredStreamClear = useCallback((): void => {
		if (deferredStreamClearTimeoutRef.current === null) {
			return;
		}
		clearTimeout(deferredStreamClearTimeoutRef.current);
		deferredStreamClearTimeoutRef.current = null;
	}, []);

	const scheduleDeferredStreamClear = useCallback((): void => {
		if (deferredStreamClearTimeoutRef.current !== null) {
			return;
		}
		deferredStreamClearTimeoutRef.current = setTimeout(() => {
			deferredStreamClearTimeoutRef.current = null;
			store.clearStreamState();
		}, 0);
	}, [store]);

	useEffect(() => {
		clearDeferredStreamClear();
		store.resetTransientState();
		if (!activeChatId) {
			return;
		}

		let disposed = false;
		const chatId = activeChatId;

		const applyEventBatch = (events: readonly ChatStreamEvent[]): void => {
			if (disposed || events.length === 0) {
				return;
			}

			const pendingMessageParts: Record<string, unknown>[] = [];
			let shouldClearStreamAfterBatch = false;

			const flushMessageParts = (): void => {
				if (pendingMessageParts.length === 0) {
					return;
				}
				clearDeferredStreamClear();
				if (!shouldApplyMessageParts(store)) {
					pendingMessageParts.length = 0;
					return;
				}
				store.applyMessageParts(pendingMessageParts.splice(0));
			};

			for (const event of events) {
				switch (event.type) {
					case "message_part": {
						if (isMismatchedChatID(event.chat_id, chatId)) {
							continue;
						}
						if (!shouldApplyMessageParts(store)) {
							continue;
						}
						const nextPart = event.message_part?.part;
						if (nextPart) {
							clearDeferredStreamClear();
							pendingMessageParts.push(nextPart);
						}
						continue;
					}
					case "message": {
						flushMessageParts();
						if (isMismatchedChatID(event.chat_id, chatId)) {
							continue;
						}
						const result = store.upsertDurableMessage(event.message);
						if (result.changed) {
							if (
								lastDurableMessageId.current === undefined ||
								event.message.id > lastDurableMessageId.current
							) {
								lastDurableMessageId.current = event.message.id;
							}
							shouldClearStreamAfterBatch = true;
						}
						continue;
					}
					case "queue_update": {
						flushMessageParts();
						if (isMismatchedChatID(event.chat_id, chatId)) {
							continue;
						}
						store.setQueuedMessages(event.queued_messages);
						continue;
					}
					case "status": {
						flushMessageParts();
						if (event.chat_id && event.chat_id !== chatId) {
							store.setSubagentStatusOverride(
								event.chat_id,
								event.status.status,
							);
							continue;
						}
						store.setChatStatus(event.status.status);
						if (
							event.status.status === "pending" ||
							event.status.status === "waiting"
						) {
							clearDeferredStreamClear();
							store.clearStreamState();
							store.clearRetryState();
						}
						if (event.status.status === "running") {
							store.clearRetryState();
						}
						continue;
					}
					case "error": {
						flushMessageParts();
						if (isMismatchedChatID(event.chat_id, chatId)) {
							continue;
						}
						clearDeferredStreamClear();
						store.setChatStatus("error");
						store.setStreamError(
							event.error.message.trim() || "Chat processing failed.",
						);
						store.clearRetryState();
						continue;
					}
					case "retry": {
						flushMessageParts();
						if (isMismatchedChatID(event.chat_id, chatId)) {
							continue;
						}
						clearDeferredStreamClear();
						store.clearStreamState();
						store.setRetryState({
							attempt: event.retry.attempt,
							error: event.retry.error,
						});
						continue;
					}
					default:
						continue;
				}
			}

			flushMessageParts();
			if (shouldClearStreamAfterBatch) {
				scheduleDeferredStreamClear();
			}
		};

		const subscription = runtime.subscribeToChat(
			{
				chatId,
				afterMessageId: lastDurableMessageId.current,
			},
			(event) => {
				applyEventBatch([event]);
			},
		);

		return () => {
			disposed = true;
			subscription.dispose();
			clearDeferredStreamClear();
			store.resetTransientState();
		};
	}, [
		activeChatId,
		clearDeferredStreamClear,
		runtime,
		scheduleDeferredStreamClear,
		store,
	]);

	const value = useMemo<ChatRuntimeContextValue>(
		() => ({
			runtime,
			preferenceStore: adaptedPreferenceStore,
			store,
			activeChatId,
			setActiveChatId,
			lastDurableMessageId,
			selectionToken,
			modelCache,
		}),
		[activeChatId, adaptedPreferenceStore, runtime, store],
	);

	return (
		<ChatRuntimeContext.Provider value={value}>
			{children}
		</ChatRuntimeContext.Provider>
	);
};

/** @public Reads the shared React chat runtime context. */
export const useChatRuntimeContext = (): ChatRuntimeContextValue => {
	const context = useContext(ChatRuntimeContext);
	if (context === null) {
		throw new Error(
			"useChatRuntimeContext must be used within a ChatRuntimeProvider.",
		);
	}
	return context;
};

/** @public Reads the current shared chat store snapshot. */
export const useChatStoreSnapshot = (): ChatStoreState => {
	const { store } = useChatRuntimeContext();
	return useSyncExternalStore(
		store.subscribe,
		store.getSnapshot,
		store.getSnapshot,
	);
};
