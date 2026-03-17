import { renderHook, act } from "@testing-library/react";
import { useStickToBottom } from "./useStickToBottom";

// Minimal mock for a flex-col-reverse scrollable div.
const createMockScrollContainer = () => {
	let scrollTop = 0;
	const listeners = new Map<string, Set<EventListener>>();

	const el = {
		get scrollTop() {
			return scrollTop;
		},
		set scrollTop(value: number) {
			scrollTop = value;
		},
		scrollTo({ top }: { top: number; behavior?: string }) {
			scrollTop = top;
			// Fire scroll event synchronously for testing.
			for (const handler of listeners.get("scroll") ?? []) {
				handler(new Event("scroll"));
			}
		},
		addEventListener(type: string, handler: EventListener) {
			if (!listeners.has(type)) {
				listeners.set(type, new Set());
			}
			listeners.get(type)!.add(handler);
		},
		removeEventListener(type: string, handler: EventListener) {
			listeners.get(type)?.delete(handler);
		},
		// Helper to simulate a scroll event at a given scrollTop.
		simulateScroll(newScrollTop: number) {
			scrollTop = newScrollTop;
			for (const handler of listeners.get("scroll") ?? []) {
				handler(new Event("scroll"));
			}
		},
	} as unknown as HTMLDivElement & { simulateScroll: (v: number) => void };

	return el;
};

describe("useStickToBottom", () => {
	it("starts in the stuck state", () => {
		const { result } = renderHook(() => useStickToBottom());
		expect(result.current.isStuck).toBe(true);
	});

	it("unsticks when user scrolls up past threshold", () => {
		const { result } = renderHook(() => useStickToBottom());
		const el = createMockScrollContainer();

		// Attach the scroll container via the callback ref.
		act(() => {
			result.current.scrollRef(el);
		});

		// Simulate scrolling up (scrollTop increases in flex-col-reverse).
		act(() => {
			el.simulateScroll(10);
		});
		// Still within the 48px threshold.
		expect(result.current.isStuck).toBe(true);

		act(() => {
			el.simulateScroll(100);
		});
		// Past threshold — should unstick.
		expect(result.current.isStuck).toBe(false);
	});

	it("re-sticks when user scrolls back to bottom", () => {
		const { result } = renderHook(() => useStickToBottom());
		const el = createMockScrollContainer();

		act(() => {
			result.current.scrollRef(el);
		});

		// Scroll up past threshold.
		act(() => {
			el.simulateScroll(100);
		});
		expect(result.current.isStuck).toBe(false);

		// Scroll back to near-bottom.
		act(() => {
			el.simulateScroll(10);
		});
		expect(result.current.isStuck).toBe(true);
	});

	it("scrollToBottom scrolls to top=0 in flex-col-reverse", () => {
		const { result } = renderHook(() => useStickToBottom());
		const el = createMockScrollContainer();

		act(() => {
			result.current.scrollRef(el);
		});

		// Scroll up.
		act(() => {
			el.simulateScroll(500);
		});
		expect(result.current.isStuck).toBe(false);

		// Use the programmatic scroll-to-bottom.
		act(() => {
			result.current.scrollToBottom();
		});
		expect(el.scrollTop).toBe(0);
		// The scroll event from scrollTo should re-stick.
		expect(result.current.isStuck).toBe(true);
	});

	it("cleans up scroll listener on unmount", () => {
		const { result, unmount } = renderHook(() => useStickToBottom());
		const el = createMockScrollContainer();
		const removeSpy = vi.spyOn(el, "removeEventListener");

		act(() => {
			result.current.scrollRef(el);
		});

		unmount();
		expect(removeSpy).toHaveBeenCalledWith("scroll", expect.any(Function));
	});
});
