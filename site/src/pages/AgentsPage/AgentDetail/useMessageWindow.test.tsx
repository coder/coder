import { renderHook, act } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ChatMessage } from "api/typesGenerated";
import { useMessageWindow } from "./useMessageWindow";

// ---------------------------------------------------------------------------
// IntersectionObserver mock
// ---------------------------------------------------------------------------

type IOCallback = (entries: IntersectionObserverEntry[]) => void;

interface MockIOInstance {
	callback: IOCallback;
	options?: IntersectionObserverInit;
	observe: ReturnType<typeof vi.fn>;
	disconnect: ReturnType<typeof vi.fn>;
}

let ioInstances: MockIOInstance[];

beforeEach(() => {
	ioInstances = [];

	// Must be a regular function so it can be called with `new`.
	vi.stubGlobal(
		"IntersectionObserver",
		function MockIntersectionObserver(
			this: MockIOInstance,
			callback: IOCallback,
			options?: IntersectionObserverInit,
		) {
			this.callback = callback;
			this.options = options;
			this.observe = vi.fn();
			this.disconnect = vi.fn();
			ioInstances.push(this);
		},
	);
});

afterEach(() => {
	vi.restoreAllMocks();
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeMessages(count: number): ChatMessage[] {
	return Array.from({ length: count }, (_, i) => ({
		id: i,
		chat_id: "test-chat",
		created_at: new Date(i * 1000).toISOString(),
		role: "user",
		content: [{ type: "text" as const, text: `message-${i}` }],
	}));
}

/** Simulate the sentinel element becoming visible. */
function triggerIntersection() {
	const instance = ioInstances.at(-1);
	if (!instance) {
		throw new Error("No IntersectionObserver instance found");
	}
	instance.callback([{ isIntersecting: true } as IntersectionObserverEntry]);
}

/**
 * Flush the requestAnimationFrame-based loading gate so the
 * observer can fire again.
 */
function flushLoadingGate() {
	vi.advanceTimersToNextTimer();
}

/**
 * The hook uses a callback ref for the sentinel. In tests we
 * invoke it manually after mount to simulate React's commit phase.
 */
function renderMessageWindow(
	messages: ChatMessage[],
	pageSize: number,
	resetKey?: string,
) {
	const sentinel = document.createElement("div");

	const hookResult = renderHook(
		(props: { messages: ChatMessage[]; pageSize: number; resetKey?: string }) =>
			useMessageWindow(props),
		{ initialProps: { messages, pageSize, resetKey } },
	);

	// Simulate React calling the callback ref with the sentinel.
	act(() => {
		hookResult.result.current.loadMoreSentinelRef(sentinel);
	});

	return hookResult;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useMessageWindow", () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("shows only pageSize messages from the end on initial render", () => {
		const pageSize = 5;
		const messages = makeMessages(20);

		const { result } = renderMessageWindow(messages, pageSize);

		expect(result.current.windowedMessages).toHaveLength(pageSize);
		expect(result.current.windowedMessages[0]).toBe(
			messages[messages.length - pageSize],
		);
		expect(result.current.windowedMessages[pageSize - 1]).toBe(
			messages[messages.length - 1],
		);
		expect(result.current.hasMoreMessages).toBe(true);
	});

	it("loads exactly one more page when the sentinel intersects (no cascade)", () => {
		const pageSize = 5;
		const messages = makeMessages(20);

		const { result } = renderMessageWindow(messages, pageSize);
		expect(result.current.windowedMessages).toHaveLength(pageSize);

		// Trigger the IO callback once.
		act(() => {
			triggerIntersection();
		});

		// Exactly one additional page.
		expect(result.current.windowedMessages).toHaveLength(pageSize * 2);
		expect(result.current.hasMoreMessages).toBe(true);
	});

	it("blocks rapid-fire IO callbacks until the loading gate clears", () => {
		const pageSize = 5;
		const messages = makeMessages(20);

		const { result } = renderMessageWindow(messages, pageSize);

		// Fire the IO callback three times in a row (simulating the
		// browser re-evaluating intersection after each React
		// commit). Only the first should take effect.
		act(() => {
			triggerIntersection();
			triggerIntersection();
			triggerIntersection();
		});

		expect(result.current.windowedMessages).toHaveLength(pageSize * 2);

		// Flush the rAF gate — now a second intersection should work.
		act(() => {
			flushLoadingGate();
		});
		act(() => {
			triggerIntersection();
		});

		expect(result.current.windowedMessages).toHaveLength(pageSize * 3);
	});

	it("sets hasMoreMessages to false when all messages are loaded", () => {
		const pageSize = 5;
		const messages = makeMessages(12);

		const { result } = renderMessageWindow(messages, pageSize);

		expect(result.current.windowedMessages).toHaveLength(5);
		expect(result.current.hasMoreMessages).toBe(true);

		// Page 2.
		act(() => triggerIntersection());
		act(() => flushLoadingGate());
		expect(result.current.windowedMessages).toHaveLength(10);
		expect(result.current.hasMoreMessages).toBe(true);

		// Page 3 — all 12 messages.
		act(() => triggerIntersection());
		expect(result.current.windowedMessages).toHaveLength(12);
		expect(result.current.hasMoreMessages).toBe(false);
	});

	it("returns all messages when total is less than pageSize", () => {
		const pageSize = 10;
		const messages = makeMessages(3);

		const { result } = renderMessageWindow(messages, pageSize);

		expect(result.current.windowedMessages).toHaveLength(3);
		expect(result.current.windowedMessages).toBe(messages);
		expect(result.current.hasMoreMessages).toBe(false);
	});

	it("resets to initial page size when resetKey changes", () => {
		const pageSize = 5;
		const messages = makeMessages(20);

		const { result, rerender } = renderMessageWindow(messages, pageSize, "a");

		act(() => triggerIntersection());
		expect(result.current.windowedMessages).toHaveLength(pageSize * 2);

		rerender({ messages, pageSize, resetKey: "b" });
		expect(result.current.windowedMessages).toHaveLength(pageSize);
		expect(result.current.hasMoreMessages).toBe(true);
	});

	it("does not recreate the IntersectionObserver on every render", () => {
		const pageSize = 5;
		const messages = makeMessages(20);

		const { rerender } = renderMessageWindow(messages, pageSize);

		const countAfterMount = ioInstances.length;
		expect(countAfterMount).toBeGreaterThan(0);

		rerender({ messages, pageSize, resetKey: undefined });
		rerender({ messages, pageSize, resetKey: undefined });
		rerender({ messages, pageSize, resetKey: undefined });

		expect(ioInstances.length).toBe(countAfterMount);
	});
});
