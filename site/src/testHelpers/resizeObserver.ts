import { vi } from "vitest";

/**
 * Controllable fake `ResizeObserver` for unit tests. jsdom does not implement
 * `ResizeObserver` with real layout, so tests that exercise resize-driven
 * behavior install this stub and drive it directly.
 *
 * Each test wires it up itself: install it with
 * `vi.stubGlobal("ResizeObserver", MockResizeObserver)` in `beforeEach` (after
 * calling `MockResizeObserver.reset()`), and restore the real global with
 * `vi.unstubAllGlobals()` in `afterEach`. Tests reach constructed observers via
 * `MockResizeObserver.getLast()` / `MockResizeObserver.instances`, drive the
 * observed callback with `simulateResize`, and assert on `disconnect`.
 */
export class MockResizeObserver implements ResizeObserver {
	/** Every observer constructed since the last reset, in creation order. */
	static instances: MockResizeObserver[] = [];

	/** Clears tracked instances; call in `beforeEach`. */
	static reset(): void {
		MockResizeObserver.instances = [];
	}

	/** Returns the most recently constructed observer, throwing if none exist. */
	static getLast(): MockResizeObserver {
		const last = MockResizeObserver.instances.at(-1);
		if (!last) {
			throw new Error("No ResizeObserver was constructed");
		}
		return last;
	}

	observe = vi.fn();
	unobserve = vi.fn();
	disconnect = vi.fn();
	private callback: ResizeObserverCallback;

	constructor(callback: ResizeObserverCallback) {
		this.callback = callback;
		MockResizeObserver.instances.push(this);
	}

	/**
	 * Invokes the observed callback with a single entry carrying the given
	 * content-box size. `height` defaults to 0 for callers that only care about
	 * width.
	 */
	simulateResize = (width: number, height = 0): void => {
		this.callback(
			[{ contentRect: { width, height } } as ResizeObserverEntry],
			this,
		);
	};
}
