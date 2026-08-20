import { vi } from "vitest";

/**
 * Replaces `window.matchMedia` with a controllable mock for hook tests.
 * Queries listed in `initialMatches` report their configured value and
 * every other query reports `false`. `setMatches` updates a query and
 * notifies its registered change listeners.
 */
export const setupMatchMedia = (initialMatches: Record<string, boolean>) => {
	const matches = { ...initialMatches };
	const listeners = new Map<string, Set<EventListenerOrEventListenerObject>>();
	const listenersFor = (query: string) => {
		let set = listeners.get(query);
		if (!set) {
			set = new Set();
			listeners.set(query, set);
		}
		return set;
	};
	Object.defineProperty(window, "matchMedia", {
		writable: true,
		value: vi.fn((query: string): MediaQueryList => {
			return {
				get matches() {
					return matches[query] ?? false;
				},
				media: query,
				onchange: null,
				addEventListener: (
					_type: string,
					listener: EventListenerOrEventListenerObject,
				) => {
					listenersFor(query).add(listener);
				},
				removeEventListener: (
					_type: string,
					listener: EventListenerOrEventListenerObject,
				) => {
					listenersFor(query).delete(listener);
				},
				dispatchEvent: vi.fn(() => true),
				addListener: vi.fn(),
				removeListener: vi.fn(),
			} satisfies MediaQueryList;
		}),
	});
	return {
		setMatches: (query: string, value: boolean) => {
			matches[query] = value;
			const event = new Event("change");
			for (const listener of listenersFor(query)) {
				if (typeof listener === "function") {
					listener(event);
				} else {
					listener.handleEvent(event);
				}
			}
		},
	};
};
