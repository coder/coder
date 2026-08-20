/**
 * Replaces `window.matchMedia` with a controllable stub for tests and
 * stories. Queries listed in `initialMatches` report their configured
 * value; every other query delegates to the real `matchMedia` (or
 * reports `false` where none exists, e.g. jsdom) so unrelated
 * responsive components keep behaving truthfully. `setMatches` updates
 * a query and notifies its registered change listeners; `restore` puts
 * the original `window.matchMedia` back.
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
	const original = window.matchMedia;
	const originalFn =
		typeof original === "function" ? original.bind(window) : undefined;
	Object.defineProperty(window, "matchMedia", {
		configurable: true,
		writable: true,
		value: (query: string): MediaQueryList => {
			if (!(query in matches) && originalFn) {
				return originalFn(query);
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
		restore: () => {
			Object.defineProperty(window, "matchMedia", {
				configurable: true,
				writable: true,
				value: original,
			});
		},
	};
};
