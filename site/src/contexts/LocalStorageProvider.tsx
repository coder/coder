import { createContext, useContext, useState } from "react";

type PendingWrite = {
	serialized: string | null;
	timer: ReturnType<typeof setTimeout>;
};

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
};

/** Creates a store with no values. Each provider creates its own. */
function createLocalStorageStore(): LocalStorageStore {
	const listenersByKey = new Map<string, Set<() => void>>();
	const pendingWrites = new Map<string, PendingWrite>();
	// Values whose storage write failed. They stay in memory until storage changes.
	const unsavedWrites = new Map<string, string | null>();

	const readSerialized = (key: string): string | null => {
		const pending = pendingWrites.get(key);
		if (pending) {
			return pending.serialized;
		}

		const unsaved = unsavedWrites.get(key);
		if (unsaved !== undefined) {
			return unsaved;
		}

		try {
			return localStorage.getItem(key);
		} catch {
			return null;
		}
	};

	const writeSerialized = (key: string, serialized: string | null): void => {
		try {
			if (serialized === null) {
				localStorage.removeItem(key);
			} else {
				localStorage.setItem(key, serialized);
			}

			unsavedWrites.delete(key);
		} catch {
			// Storage can be unavailable or full. The in-memory value still applies.
			unsavedWrites.set(key, serialized);
		}
	};

	const flushWrite = (key: string): void => {
		const pending = pendingWrites.get(key);
		if (!pending) {
			return;
		}

		clearTimeout(pending.timer);
		pendingWrites.delete(key);
		writeSerialized(key, pending.serialized);
	};

	const commitSerialized = (
		key: string,
		serialized: string | null,
		debounceMs: number,
	) => {
		const pending = pendingWrites.get(key);
		if (pending) {
			clearTimeout(pending.timer);
			pendingWrites.delete(key);
		}

		if (debounceMs > 0) {
			const timer = setTimeout(() => flushWrite(key), debounceMs);
			pendingWrites.set(key, { serialized, timer });
		} else {
			writeSerialized(key, serialized);
		}

		for (const listener of listenersByKey.get(key) ?? []) {
			listener();
		}
	};

	const subscribe = (key: string, onChange: () => void): (() => void) => {
		let listeners = listenersByKey.get(key);
		if (!listeners) {
			listeners = new Set();
			listenersByKey.set(key, listeners);
		}
		listeners.add(onChange);

		// Storage events come from other tabs. The listener only signals a change.
		const onStorage = (event: StorageEvent) => {
			if (event.key === null || event.key === key) {
				unsavedWrites.delete(key);
				onChange();
			}
		};
		window.addEventListener("storage", onStorage);

		return () => {
			listeners.delete(onChange);
			if (listeners.size === 0) {
				listenersByKey.delete(key);
			}

			window.removeEventListener("storage", onStorage);
		};
	};

	return { readSerialized, commitSerialized, flushWrite, subscribe };
}

const LocalStorageStoreContext = createContext<LocalStorageStore | null>(null);

type LocalStorageProviderProps = { children: React.ReactNode };

/** Gives every `useLocalStorage` call in its subtree one shared store. */
export const LocalStorageProvider: React.FC<LocalStorageProviderProps> = ({
	children,
}) => {
	const [store] = useState(createLocalStorageStore);

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
