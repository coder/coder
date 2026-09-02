import { type Mock, vi } from "vitest";

interface FakeResizeObserver {
	observe: Mock;
	unobserve: Mock;
	disconnect: Mock;
	/**
	 * Invokes the observed callback with a single entry carrying the given
	 * content-box size, simulating a resize. `height` defaults to 0 for callers
	 * that only care about width.
	 */
	simulateResize: (width: number, height?: number) => void;
}

export interface ResizeObserverMock {
	/** Every observer constructed since setup, in creation order. */
	readonly instances: FakeResizeObserver[];
	/** Returns the most recently constructed observer, throwing if none exist. */
	getLast: () => FakeResizeObserver;
}

/**
 * Installs a fake `ResizeObserver` on the global object via `vi.stubGlobal`.
 * jsdom does not implement `ResizeObserver` with real layout, so unit tests
 * that exercise resize-driven behavior install a controllable stub. The stub is
 * a `vi.fn` wrapping a class (a plain function is not newable), so each
 * constructed observer is tracked and tests can drive its callback with
 * `simulateResize` and assert on `disconnect`.
 *
 * Call this inside `beforeEach` (each call starts a fresh instance list) and
 * `vi.unstubAllGlobals()` in `afterEach` to restore the original global.
 */
export function setupResizeObserverMock(): ResizeObserverMock {
	const instances: FakeResizeObserver[] = [];

	class MockResizeObserver implements ResizeObserver {
		observe = vi.fn();
		unobserve = vi.fn();
		disconnect = vi.fn();
		private callback: ResizeObserverCallback;

		constructor(callback: ResizeObserverCallback) {
			this.callback = callback;
			instances.push(this);
		}

		simulateResize = (width: number, height = 0): void => {
			this.callback(
				[{ contentRect: { width, height } } as ResizeObserverEntry],
				this,
			);
		};
	}

	vi.stubGlobal("ResizeObserver", vi.fn(MockResizeObserver));

	return {
		instances,
		getLast: () => {
			const instance = instances.at(-1);
			if (!instance) {
				throw new Error("No ResizeObserver was constructed");
			}
			return instance;
		},
	};
}
