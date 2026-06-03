import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { _resetForTesting, scrollToUserSentinel } from "./scrollToUserSentinel";

function buildDOM(messageIds: number[]) {
	const scroller = document.createElement("div");
	scroller.classList.add("overflow-y-auto");
	Object.defineProperty(scroller, "clientHeight", { value: 600 });

	// Make scrollTop writable and observable.
	let scrollTopValue = 0;
	Object.defineProperty(scroller, "scrollTop", {
		get: () => scrollTopValue,
		set: (v: number) => {
			scrollTopValue = v;
		},
	});

	for (const id of messageIds) {
		const sentinel = document.createElement("div");
		sentinel.setAttribute("data-user-sentinel", "");
		sentinel.setAttribute("data-user-message-id", String(id));
		// Mock getBoundingClientRect relative to scroller.
		sentinel.getBoundingClientRect = () =>
			({
				top: id * 200,
				left: 0,
				right: 0,
				bottom: 0,
				width: 0,
				height: 0,
			}) as DOMRect;
		scroller.appendChild(sentinel);
	}

	scroller.getBoundingClientRect = () =>
		({
			top: 0,
			left: 0,
			right: 0,
			bottom: 600,
			width: 0,
			height: 600,
		}) as DOMRect;

	document.body.appendChild(scroller);
	return scroller;
}

describe("scrollToUserSentinel", () => {
	let matchMediaSpy: ReturnType<typeof vi.spyOn>;

	beforeEach(() => {
		document.body.innerHTML = "";
		_resetForTesting();
		// Default: no reduced motion
		matchMediaSpy = vi.spyOn(window, "matchMedia").mockReturnValue({
			matches: false,
		} as MediaQueryList);
	});

	afterEach(() => {
		matchMediaSpy.mockRestore();
		vi.restoreAllMocks();
	});

	it("does nothing when sentinel is not found", () => {
		const scroller = buildDOM([1]);
		expect(scrollToUserSentinel(999)).toBe(false);
		expect(scroller.scrollTop).toBe(0);
	});

	it("does nothing when scroller is not found", () => {
		// Create sentinel outside a scroll container
		const sentinel = document.createElement("div");
		sentinel.setAttribute("data-user-sentinel", "");
		sentinel.setAttribute("data-user-message-id", "1");
		document.body.appendChild(sentinel);

		expect(scrollToUserSentinel(1)).toBe(false);
	});

	it("jumps instantly and updates scrollTop when prefers-reduced-motion is set", () => {
		matchMediaSpy.mockReturnValue({
			matches: true,
		} as MediaQueryList);

		const scroller = buildDOM([1]);
		expect(scrollToUserSentinel(1)).toBe(true);

		// sentinel top=200, scroller top=0, clientHeight=600 => offset = 200 - 0 - 300 = -100
		expect(scroller.scrollTop).toBe(-100);
		expect(scroller.hasAttribute("data-scroll-lock")).toBe(false);
		expect(scroller.style.overflowAnchor).toBe("");
	});

	it("sets and clears scroll-lock during animation", () => {
		const scroller = buildDOM([1]);

		// Mock RAF to run callbacks synchronously
		let rafCallback: FrameRequestCallback | null = null;
		vi.spyOn(window, "requestAnimationFrame").mockImplementation(
			(cb: FrameRequestCallback) => {
				rafCallback = cb;
				return 1;
			},
		);

		scrollToUserSentinel(1);

		// Lock should be set during animation
		expect(scroller.hasAttribute("data-scroll-lock")).toBe(true);
		expect(scroller.style.overflowAnchor).toBe("none");

		// Simulate completion by calling with a time far in the future
		if (rafCallback) {
			(rafCallback as FrameRequestCallback)(Number.MAX_SAFE_INTEGER);
		}

		// Lock should be cleared after completion
		expect(scroller.hasAttribute("data-scroll-lock")).toBe(false);
		expect(scroller.style.overflowAnchor).toBe("");
	});

	it("updates scrollTop to final position after animation completes", () => {
		const scroller = buildDOM([1]);

		let rafCallback: FrameRequestCallback | null = null;
		vi.spyOn(window, "requestAnimationFrame").mockImplementation(
			(cb: FrameRequestCallback) => {
				rafCallback = cb;
				return 1;
			},
		);

		scrollToUserSentinel(1);

		// Complete the animation
		if (rafCallback) {
			(rafCallback as FrameRequestCallback)(Number.MAX_SAFE_INTEGER);
		}

		// sentinel top=200, scroller top=0, clientHeight=600 => offset = -100
		expect(scroller.scrollTop).toBe(-100);
	});

	it("cancels in-flight animation and cleans up lock state", () => {
		const scroller = buildDOM([1, 2]);
		const cancelSpy = vi.spyOn(window, "cancelAnimationFrame");
		let rafId = 0;
		vi.spyOn(window, "requestAnimationFrame").mockImplementation(() => {
			rafId++;
			return rafId;
		});

		scrollToUserSentinel(1);
		expect(cancelSpy).not.toHaveBeenCalled();
		// Lock should be set from first animation.
		expect(scroller.hasAttribute("data-scroll-lock")).toBe(true);
		expect(scroller.style.overflowAnchor).toBe("none");

		// Second call should cancel the first and clean up its lock.
		scrollToUserSentinel(2);
		expect(cancelSpy).toHaveBeenCalledWith(1);
		// Lock should be re-applied for the new animation, proving
		// unlock() ran between cancel and the new lock.
		expect(scroller.hasAttribute("data-scroll-lock")).toBe(true);
		expect(scroller.style.overflowAnchor).toBe("none");
	});

	it("dispatches scroll event on completion", () => {
		const scroller = buildDOM([1]);
		const scrollHandler = vi.fn();
		scroller.addEventListener("scroll", scrollHandler);

		let rafCallback: FrameRequestCallback | null = null;
		vi.spyOn(window, "requestAnimationFrame").mockImplementation(
			(cb: FrameRequestCallback) => {
				rafCallback = cb;
				return 1;
			},
		);

		scrollToUserSentinel(1);

		// Complete the animation
		if (rafCallback) {
			(rafCallback as FrameRequestCallback)(Number.MAX_SAFE_INTEGER);
		}

		expect(scrollHandler).toHaveBeenCalled();
	});
});
