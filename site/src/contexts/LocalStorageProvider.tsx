import { createContext, useContext, useEffect, useState } from "react";

/**
 * Serialized key access shared by the `useLocalStorage` calls under one
 * provider. Writes notify same-tab subscribers, since the native storage event
 * only fires in other tabs.
 */
type LocalStorageStore = {
	read: (key: string) => string | null;
	write: (key: string, serialized: string | null) => void;
	subscribe: (key: string, onChange: () => void) => () => void;
	handleStorageEvent: (event: StorageEvent) => void;
};

function createLocalStorageStore(): LocalStorageStore {
	const listenersByKey = new Map<string, Set<() => void>>();

	const notify = (key: string) => {
		for (const listener of listenersByKey.get(key) ?? []) {
			listener();
		}
	};

	const read = (key: string): string | null => {
		try {
			return localStorage.getItem(key);
		} catch {
			return null;
		}
	};

	const write = (key: string, serialized: string | null): void => {
		try {
			if (serialized === null) {
				localStorage.removeItem(key);
			} else {
				localStorage.setItem(key, serialized);
			}
		} catch {
			// localStorage can be unavailable or full. Notify anyway so subscribers
			// re-read whatever it actually holds.
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

	// Fired for writes from other tabs. A null key means storage was cleared.
	// Subscribers re-read the key, so the event payload is ignored.
	const handleStorageEvent = (event: StorageEvent): void => {
		const changedKeys =
			event.key === null ? [...listenersByKey.keys()] : [event.key];

		for (const key of changedKeys) {
			notify(key);
		}
	};

	return {
		read,
		write,
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
