import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { scrollToUserSentinel } from "./scrollToUserSentinel";

function buildDOM(messageIds: number[]) {
	const scroller = document.createElement("div");
	scroller.setAttribute("data-testid", "scroll-container");
	Object.defineProperty(scroller, "clientHeight", { value: 600 });
	scroller.scrollTop = 0;

	for (const id of messageIds) {
		const sentinel = document.createElement("div");
		sentinel.setAttribute("data-user-sentinel", "");
		sentinel.setAttribute("data-user-message-id", String(id));
		scroller.appendChild(sentinel);
	}

	document.body.appendChild(scroller);
	return scroller;
}

describe("scrollToUserSentinel", () => {
	let matchMediaSpy: ReturnType<typeof vi.spyOn>;

	beforeEach(() => {
		document.body.innerHTML = "";
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
		scrollToUserSentinel(999);
		expect(scroller.scrollTop).toBe(0);
	});

	it("does nothing when scroller is not found", () => {
		// Create sentinel outside a scroll container
		const sentinel = document.createElement("div");
		sentinel.setAttribute("data-user-sentinel", "");
		sentinel.setAttribute("data-user-message-id", "1");
		document.body.appendChild(sentinel);

		scrollToUserSentinel(1);
		// No error thrown is the success condition
	});

	it("jumps instantly when prefers-reduced-motion is set", () => {
		matchMediaSpy.mockReturnValue({
			matches: true,
		} as MediaQueryList);

		const scroller = buildDOM([1, 2]);
		scrollToUserSentinel(1);

		// Reduced motion path sets scrollTop synchronously, no RAF needed.
		// The exact value depends on getBoundingClientRect mocking, but
		// it should have set the lock attribute and removed it.
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

	it("cancels in-flight animation when called again", () => {
		buildDOM([1, 2]);
		const cancelSpy = vi.spyOn(window, "cancelAnimationFrame");
		let rafId = 0;
		vi.spyOn(window, "requestAnimationFrame").mockImplementation(() => {
			rafId++;
			return rafId;
		});

		scrollToUserSentinel(1);
		expect(cancelSpy).not.toHaveBeenCalled();

		// Second call should cancel the first
		scrollToUserSentinel(2);
		expect(cancelSpy).toHaveBeenCalledWith(1);
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
