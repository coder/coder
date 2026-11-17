import { createContext, useContext, useEffect, useState } from "react";

/**
 * A key's value while it differs from localStorage. A key with no entry reads
 * from localStorage directly.
 */
type Entry =
	| {
			state: "pending";
			serialized: string | null;
			timer: ReturnType<typeof setTimeout>;
	  }
	| { state: "unsaved"; serialized: string | null };

/**
 * Read, write, and subscribe operations over serialized values, shared by the
 * `useLocalStorage` calls in one provider.
 */
export type LocalStorageStore = {
	readSerialized: (key: string) => string | null;
	commitSerialized: (
		key: string,
		serialized: string | null,
		debounceMs: number,
	) => void;
	flushWrite: (key: string) => void;
	subscribe: (key: string, onChange: () => void) => () => void;
	handleStorageEvent: (event: StorageEvent) => void;
};

/** Creates a store with no values. Each provider creates its own. */
function createLocalStorageStore(): LocalStorageStore {
	const listenersByKey = new Map<string, Set<() => void>>();
	const entries = new Map<string, Entry>();

	const notify = (key: string) => {
		for (const listener of listenersByKey.get(key) ?? []) {
			listener();
		}
	};

	const readSerialized = (key: string): string | null => {
		const entry = entries.get(key);
		if (entry) {
			return entry.serialized;
		}

		try {
			return localStorage.getItem(key);
		} catch {
			return null;
		}
	};

	// Failed writes stay in memory as unsaved until storage changes for the key.
	const writeToStorage = (key: string, serialized: string | null): void => {
		try {
			if (serialized === null) {
				localStorage.removeItem(key);
			} else {
				localStorage.setItem(key, serialized);
			}
		} catch {
			entries.set(key, { state: "unsaved", serialized });
		}
	};

	const flushWrite = (key: string): void => {
		const entry = entries.get(key);
		if (entry?.state !== "pending") {
			return;
		}

		clearTimeout(entry.timer);
		entries.delete(key);
		writeToStorage(key, entry.serialized);
	};

	const commitSerialized = (
		key: string,
		serialized: string | null,
		debounceMs: number,
	): void => {
		const entry = entries.get(key);
		if (entry?.state === "pending") {
			clearTimeout(entry.timer);
		}
		entries.delete(key);

		if (debounceMs > 0) {
			const timer = setTimeout(() => flushWrite(key), debounceMs);
			entries.set(key, { state: "pending", serialized, timer });
		} else {
			writeToStorage(key, serialized);
		}

		notify(key);
	};

	const subscribe = (key: string, onChange: () => void): (() => void) => {
		let listeners = listenersByKey.get(key);
		if (!listeners) {
			listeners = new Set();
			listenersByKey.set(key, listeners);
		}
		listeners.add(onChange);

		return () => {
			listeners.delete(onChange);
			if (listeners.size === 0) {
				listenersByKey.delete(key);
			}
		};
	};

	// Storage events come from other tabs. Re-reading localStorage, not the
	// event payload, supplies the value. A pending local write still wins.
	const handleStorageEvent = (event: StorageEvent): void => {
		const changedKeys =
			event.key === null ? [...listenersByKey.keys()] : [event.key];

		for (const key of changedKeys) {
			if (entries.get(key)?.state === "unsaved") {
				entries.delete(key);
			}
			notify(key);
		}
	};

	return {
		readSerialized,
		commitSerialized,
		flushWrite,
		subscribe,
		handleStorageEvent,
	};
}

const LocalStorageStoreContext = createContext<LocalStorageStore | null>(null);

type LocalStorageProviderProps = { children: React.ReactNode };

/** Gives every `useLocalStorage` call in its subtree one shared store. */
export const LocalStorageProvider: React.FC<LocalStorageProviderProps> = ({
	children,
}) => {
	const [store] = useState(createLocalStorageStore);

	useEffect(() => {
		window.addEventListener("storage", store.handleStorageEvent);
		return () => {
			window.removeEventListener("storage", store.handleStorageEvent);
		};
	}, [store]);

	return (
		<LocalStorageStoreContext.Provider value={store}>
			{children}
		</LocalStorageStoreContext.Provider>
	);
};

/** Returns the store from the nearest `LocalStorageProvider`. Throws outside one. */
export function useLocalStorageStore(): LocalStorageStore {
	const store = useContext(LocalStorageStoreContext);
	if (!store) {
		throw new Error(
			"useLocalStorageStore must be used inside LocalStorageProvider",
		);
	}

	return store;
}
