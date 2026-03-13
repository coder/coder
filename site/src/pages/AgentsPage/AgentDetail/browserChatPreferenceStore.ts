import type { ChatPreferenceStore } from "modules/chat-shared";

const selectedModelStorageKey = "agents.last-model-config-id";
const chatPreferenceStorageKeyPrefix = "agents.chat.";

type BrowserChatPreferenceStoreOptions = {
	storage?: Storage | null;
};

function assert(condition: unknown, message: string): asserts condition {
	if (!condition) {
		throw new Error(message);
	}
}

const mapPreferenceKeyToStorageKey = (key: string): string => {
	assert(
		typeof key === "string" && key.length > 0,
		"Chat preference keys must be non-empty strings.",
	);

	// Keep selectedModel on the legacy Agents storage key so both the existing
	// page code and the shared chat runtime read the same persisted preference.
	if (key === "selectedModel") {
		return selectedModelStorageKey;
	}

	return `${chatPreferenceStorageKeyPrefix}${key}`;
};

const getBrowserStorage = (): Storage | null => {
	if (typeof window === "undefined") {
		return null;
	}

	try {
		return window.localStorage;
	} catch {
		return null;
	}
};

const hasSubscribers = (
	listenersByKey: Map<string, Set<() => void>>,
): boolean => {
	for (const listeners of listenersByKey.values()) {
		if (listeners.size > 0) {
			return true;
		}
	}

	return false;
};

export const createBrowserChatPreferenceStore = (
	options: BrowserChatPreferenceStoreOptions = {},
): ChatPreferenceStore => {
	const listenersByKey = new Map<string, Set<() => void>>();
	let isListeningToStorageEvents = false;

	const resolveStorage = (): Storage | null => {
		if (typeof window === "undefined") {
			return null;
		}

		return options.storage ?? getBrowserStorage();
	};

	const notifySubscribers = (key: string) => {
		const listeners = listenersByKey.get(key);
		if (!listeners || listeners.size === 0) {
			return;
		}

		for (const listener of [...listeners]) {
			listener();
		}
	};

	const handleStorageEvent = (event: StorageEvent) => {
		if (event.key === null) {
			return;
		}

		for (const key of listenersByKey.keys()) {
			if (mapPreferenceKeyToStorageKey(key) === event.key) {
				notifySubscribers(key);
			}
		}
	};

	const ensureStorageListener = () => {
		if (typeof window === "undefined" || isListeningToStorageEvents) {
			return;
		}

		window.addEventListener("storage", handleStorageEvent);
		isListeningToStorageEvents = true;
	};

	const cleanupStorageListener = () => {
		if (
			typeof window === "undefined" ||
			!isListeningToStorageEvents ||
			hasSubscribers(listenersByKey)
		) {
			return;
		}

		window.removeEventListener("storage", handleStorageEvent);
		isListeningToStorageEvents = false;
	};

	return {
		get<T>(key: string, fallback: T): T {
			const storage = resolveStorage();
			if (!storage) {
				return fallback;
			}

			const storageKey = mapPreferenceKeyToStorageKey(key);
			let rawValue: string | null = null;
			try {
				rawValue = storage.getItem(storageKey);
			} catch {
				return fallback;
			}

			if (rawValue === null) {
				return fallback;
			}

			try {
				return JSON.parse(rawValue) as T;
			} catch {
				if (key === "selectedModel") {
					// Existing Agents pages persisted this value as a raw string rather
					// than JSON, so continue accepting that legacy format.
					return rawValue as T;
				}

				try {
					storage.removeItem(storageKey);
				} catch {
					// Ignore cleanup failures and fall back to the caller's default.
				}

				return fallback;
			}
		},

		set<T>(key: string, value: T): void {
			const storage = resolveStorage();
			if (!storage) {
				return;
			}

			const storageKey = mapPreferenceKeyToStorageKey(key);
			try {
				if (value === undefined) {
					storage.removeItem(storageKey);
				} else if (key === "selectedModel" && typeof value === "string") {
					// Preserve the legacy raw-string format while sharing the same key.
					storage.setItem(storageKey, value);
				} else {
					const serialized = JSON.stringify(value);
					if (serialized === undefined) {
						storage.removeItem(storageKey);
					} else {
						storage.setItem(storageKey, serialized);
					}
				}

				notifySubscribers(key);
			} catch {
				// Storage can fail for quota, privacy mode, or serialization errors.
			}
		},

		subscribe(key: string, cb: () => void): () => void {
			if (resolveStorage() === null || typeof window === "undefined") {
				return () => undefined;
			}

			const listeners = listenersByKey.get(key) ?? new Set<() => void>();
			listeners.add(cb);
			listenersByKey.set(key, listeners);
			ensureStorageListener();

			return () => {
				const currentListeners = listenersByKey.get(key);
				if (!currentListeners) {
					cleanupStorageListener();
					return;
				}

				currentListeners.delete(cb);
				if (currentListeners.size === 0) {
					listenersByKey.delete(key);
				}

				cleanupStorageListener();
			};
		},
	};
};
