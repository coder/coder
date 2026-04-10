import { act, render } from "@testing-library/react";
import { createRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ChatScrollContainer } from "./ChatScrollContainer";

let intersectionCallback: IntersectionObserverCallback | null = null;

class MockIntersectionObserver implements IntersectionObserver {
	readonly root: Element | Document | null = null;
	readonly rootMargin = "";
	readonly scrollMargin = "";
	readonly thresholds = [];
	observe = vi.fn();
	unobserve = vi.fn();
	disconnect = vi.fn();
	takeRecords = vi.fn(() => []);

	constructor(callback: IntersectionObserverCallback) {
		intersectionCallback = callback;
	}
}

describe("ChatScrollContainer pagination sentinel", () => {
	beforeEach(() => {
		intersectionCallback = null;
		vi.stubGlobal(
			"IntersectionObserver",
			MockIntersectionObserver as unknown as typeof IntersectionObserver,
		);
		vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
			callback(0);
			return 1;
		});
		vi.spyOn(window, "cancelAnimationFrame").mockImplementation(() => {});
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("calls onFetchMoreMessages when the sentinel intersects and no fetch is active", () => {
		const onFetchMoreMessages = vi.fn();
		const scrollContainerRef = createRef<HTMLDivElement | null>();
		const scrollToBottomRef = createRef<(() => void) | null>();

		render(
			<ChatScrollContainer
				resetKey="chat-a"
				scrollContainerRef={scrollContainerRef}
				scrollToBottomRef={scrollToBottomRef}
				isFetchingMoreMessages={false}
				hasMoreMessages
				onFetchMoreMessages={onFetchMoreMessages}
			>
				<div style={{ height: 400 }}>messages</div>
			</ChatScrollContainer>,
		);

		expect(intersectionCallback).not.toBeNull();

		act(() => {
			intersectionCallback?.(
				[{ isIntersecting: true } as IntersectionObserverEntry],
				{} as IntersectionObserver,
			);
		});

		expect(onFetchMoreMessages).toHaveBeenCalledTimes(1);
	});

	it("does not trigger a duplicate fetch after chat reset when the new chat is already fetching", () => {
		const onFetchMoreMessages = vi.fn();
		const scrollContainerRef = createRef<HTMLDivElement | null>();
		const scrollToBottomRef = createRef<(() => void) | null>();

		const { rerender } = render(
			<ChatScrollContainer
				resetKey="chat-a"
				scrollContainerRef={scrollContainerRef}
				scrollToBottomRef={scrollToBottomRef}
				isFetchingMoreMessages={false}
				hasMoreMessages
				onFetchMoreMessages={onFetchMoreMessages}
			>
				<div style={{ height: 400 }}>messages</div>
			</ChatScrollContainer>,
		);

		rerender(
			<ChatScrollContainer
				resetKey="chat-b"
				scrollContainerRef={scrollContainerRef}
				scrollToBottomRef={scrollToBottomRef}
				isFetchingMoreMessages
				hasMoreMessages
				onFetchMoreMessages={onFetchMoreMessages}
			>
				<div style={{ height: 400 }}>messages</div>
			</ChatScrollContainer>,
		);

		expect(intersectionCallback).not.toBeNull();

		act(() => {
			intersectionCallback?.(
				[{ isIntersecting: true } as IntersectionObserverEntry],
				{} as IntersectionObserver,
			);
		});

		expect(onFetchMoreMessages).not.toHaveBeenCalled();
	});
});
