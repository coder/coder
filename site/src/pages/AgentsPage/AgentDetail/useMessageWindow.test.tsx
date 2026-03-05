import { act, render } from "@testing-library/react";
import type * as TypesGen from "api/typesGenerated";
import type { FC, RefObject } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useMessageWindow } from "./useMessageWindow";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

let intersectionCallback: IntersectionObserverCallback;

beforeEach(() => {
	vi.stubGlobal(
		"IntersectionObserver",
		vi.fn(function mockIO(
			this: unknown,
			callback: IntersectionObserverCallback,
		) {
			intersectionCallback = callback;
			return {
				observe: vi.fn(),
				disconnect: vi.fn(),
				unobserve: vi.fn(),
			};
		}),
	);
});

const makeMessage = (id: number): TypesGen.ChatMessage =>
	({
		id,
		chat_id: "chat-1",
		created_at: `2025-01-01T00:${String(id).padStart(2, "0")}:00.000Z`,
		role: id % 2 === 0 ? "user" : "assistant",
		content: [{ type: "text", text: `Message ${id}` }],
	}) as TypesGen.ChatMessage;

/** Fires the mocked IntersectionObserver callback as if the sentinel
 *  scrolled into view. */
const triggerLoadMore = () => {
	intersectionCallback(
		[{ isIntersecting: true } as IntersectionObserverEntry],
		{} as IntersectionObserver,
	);
};

// ---------------------------------------------------------------------------
// Test harness — a real component so refs are assigned during the
// React render cycle and useEffect fires normally.
// ---------------------------------------------------------------------------

type HarnessResult = {
	windowedLength: number;
	hasMoreMessages: boolean;
};

let latestResult: HarnessResult = {
	windowedLength: 0,
	hasMoreMessages: false,
};

const Harness: FC<{
	messages: readonly TypesGen.ChatMessage[];
	pageSize: number;
	resetKey?: string;
	scrollContainerRef?: RefObject<HTMLElement | null>;
}> = ({ messages, pageSize, resetKey, scrollContainerRef }) => {
	const { hasMoreMessages, windowedMessages, loadMoreSentinelRef } =
		useMessageWindow({
			messages,
			resetKey,
			pageSize,
			scrollContainerRef,
		});

	latestResult = {
		windowedLength: windowedMessages.length,
		hasMoreMessages,
	};

	return (
		<div>
			{hasMoreMessages && (
				<div ref={loadMoreSentinelRef} data-testid="sentinel" />
			)}
			<div data-testid="count">{windowedMessages.length}</div>
		</div>
	);
};

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useMessageWindow", () => {
	describe("windowing", () => {
		it("returns the last pageSize messages", () => {
			const messages = Array.from({ length: 100 }, (_, i) =>
				makeMessage(i + 1),
			);

			render(<Harness messages={messages} pageSize={50} />);

			expect(latestResult.windowedLength).toBe(50);
			expect(latestResult.hasMoreMessages).toBe(true);
		});

		it("returns all messages when count is below pageSize", () => {
			const messages = Array.from({ length: 10 }, (_, i) => makeMessage(i + 1));

			render(<Harness messages={messages} pageSize={50} />);

			expect(latestResult.windowedLength).toBe(10);
			expect(latestResult.hasMoreMessages).toBe(false);
		});

		it("loads more messages when the sentinel intersects", () => {
			const messages = Array.from({ length: 120 }, (_, i) =>
				makeMessage(i + 1),
			);

			render(<Harness messages={messages} pageSize={50} />);
			expect(latestResult.windowedLength).toBe(50);

			act(() => {
				triggerLoadMore();
			});

			expect(latestResult.windowedLength).toBe(100);
			expect(latestResult.hasMoreMessages).toBe(true);
		});

		it("loads more without crashing when no scrollContainerRef", () => {
			const messages = Array.from({ length: 100 }, (_, i) =>
				makeMessage(i + 1),
			);

			render(<Harness messages={messages} pageSize={50} />);

			act(() => {
				triggerLoadMore();
			});

			expect(latestResult.windowedLength).toBe(100);
		});

		it("resets rendered count when resetKey changes", () => {
			const messages = Array.from({ length: 100 }, (_, i) =>
				makeMessage(i + 1),
			);

			const { rerender } = render(
				<Harness messages={messages} pageSize={50} resetKey="a" />,
			);

			act(() => {
				triggerLoadMore();
			});
			expect(latestResult.windowedLength).toBe(100);

			rerender(<Harness messages={messages} pageSize={50} resetKey="b" />);
			expect(latestResult.windowedLength).toBe(50);
		});
	});
});
