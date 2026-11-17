import { createContext, useContext, useEffect, useState } from "react";

/**
 * Read, write, and subscribe operations over serialized values, shared by the
 * `useLocalStorage` calls in one provider.
 */
type LocalStorageStore = {
	readSerialized: (key: string) => string | null;
	commitSerialized: (key: string, serialized: string | null) => void;
	subscribe: (key: string, onChange: () => void) => () => void;
	handleStorageEvent: (event: StorageEvent) => void;
};

/** Creates a store with no values. Each provider creates its own. */
function createLocalStorageStore(): LocalStorageStore {
	const listenersByKey = new Map<string, Set<() => void>>();

	const notify = (key: string) => {
		for (const listener of listenersByKey.get(key) ?? []) {
			listener();
		}
	};

	const readSerialized = (key: string): string | null => {
		try {
			return localStorage.getItem(key);
		} catch {
			return null;
		}
	};

	const commitSerialized = (key: string, serialized: string | null): void => {
		try {
			if (serialized === null) {
				localStorage.removeItem(key);
			} else {
				localStorage.setItem(key, serialized);
			}
		} catch {
			// Storage can be unavailable or full. Subscribers re-read what it holds.
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

	// Storage events come from other tabs. Subscribers re-read the key, so the
	// event payload is not used as the value.
	const handleStorageEvent = (event: StorageEvent): void => {
		const changedKeys =
			event.key === null ? [...listenersByKey.keys()] : [event.key];

		for (const key of changedKeys) {
			notify(key);
		}
	};

	return {
		readSerialized,
		commitSerialized,
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
