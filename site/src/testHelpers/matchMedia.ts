import { spyOn } from "storybook/test";

/**
 * Replaces `window.matchMedia` with a controllable stub for stories.
 * Queries listed in `initialMatches` report their configured value;
 * every other query delegates to the real `matchMedia` so unrelated
 * responsive components keep behaving truthfully. `setMatches` updates
 * a query and notifies its registered change listeners; `restore` puts
 * the original `window.matchMedia` back.
 *
 * Story-only: stories run in a real browser, so a real `matchMedia` to
 * delegate to always exists. jsdom has no `matchMedia`, so unit tests
 * must install their own stub with `vi.stubGlobal` instead.
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
	const original = window.matchMedia.bind(window);
	const spy = spyOn(window, "matchMedia").mockImplementation(
		(query: string): MediaQueryList => {
			if (!(query in matches)) {
				return original(query);
			}
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
				dispatchEvent: () => true,
				addListener: () => {},
				removeListener: () => {},
			} satisfies MediaQueryList;
		},
	);
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
		restore: () => spy.mockRestore(),
	};
};
