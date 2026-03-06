import type * as TypesGen from "api/typesGenerated";
import {
	useCallback,
	useEffect,
	useLayoutEffect,
	useMemo,
	useRef,
	useState,
} from "react";

const DEFAULT_PAGE_SIZE = 50;

type UseMessageWindowOptions = {
	messages: readonly TypesGen.ChatMessage[];
	resetKey?: string;
	pageSize?: number;
	scrollContainerRef?: React.RefObject<HTMLElement | null>;
};

const SECTION_ANCHOR_ATTR = "data-section-anchor";

/**
 * Find the section element closest to the viewport top and return
 * its data-section-anchor value + pixel offset from the viewport
 * top. This is the element the user is currently reading.
 */
function captureAnchor(scrollContainer: HTMLElement) {
	const containerRect = scrollContainer.getBoundingClientRect();
	const sections = scrollContainer.querySelectorAll<HTMLElement>(
		`[${SECTION_ANCHOR_ATTR}]`,
	);

	let best: { id: string; offset: number } | null = null;

	for (let i = sections.length - 1; i >= 0; i--) {
		const el = sections[i];
		const rect = el.getBoundingClientRect();
		// Walk from the bottom (newest) toward the top (oldest).
		// The first section whose top edge is at or above the
		// viewport top is the one that straddles the viewport
		// boundary — the section the user is currently reading.
		if (rect.top <= containerRect.top) {
			best = {
				id: el.getAttribute(SECTION_ANCHOR_ATTR)!,
				offset: rect.top - containerRect.top,
			};
			break;
		}
	}

	// Fallback: first section whose bottom is in the viewport.
	if (!best && sections.length > 0) {
		for (const el of sections) {
			const rect = el.getBoundingClientRect();
			if (rect.bottom > containerRect.top) {
				best = {
					id: el.getAttribute(SECTION_ANCHOR_ATTR)!,
					offset: rect.top - containerRect.top,
				};
				break;
			}
		}
	}

	return best;
}

/**
 * After the DOM has been updated, find the same section by its
 * data-section-anchor value and adjust scrollTop so it sits at
 * the same viewport offset as before.
 */
function restoreAnchor(
	scrollContainer: HTMLElement,
	anchor: { id: string; offset: number },
) {
	const el = scrollContainer.querySelector<HTMLElement>(
		`[${SECTION_ANCHOR_ATTR}="${CSS.escape(anchor.id)}"]`,
	);
	if (!el) return;

	const containerRect = scrollContainer.getBoundingClientRect();
	const elRect = el.getBoundingClientRect();
	const currentOffset = elRect.top - containerRect.top;
	const drift = currentOffset - anchor.offset;

	if (Math.abs(drift) > 1) {
		scrollContainer.scrollTop += drift;
	}
}

export const useMessageWindow = ({
	messages,
	resetKey,
	pageSize = DEFAULT_PAGE_SIZE,
	scrollContainerRef,
}: UseMessageWindowOptions) => {
	const [renderedMessageCount, setRenderedMessageCount] = useState(pageSize);
	const observerRef = useRef<IntersectionObserver | null>(null);
	const sentinelNodeRef = useRef<HTMLDivElement | null>(null);
	// Gate that prevents the IO callback from firing more than
	// once per paint frame.
	const isLoadingRef = useRef(false);
	// Anchor captured just before a page load.
	const anchorRef = useRef<{ id: string; offset: number } | null>(null);

	useEffect(() => {
		void resetKey;
		setRenderedMessageCount(pageSize);
	}, [resetKey, pageSize]);

	const hasMoreMessages = renderedMessageCount < messages.length;
	const windowedMessages = useMemo(() => {
		if (renderedMessageCount >= messages.length) {
			return messages;
		}
		return messages.slice(messages.length - renderedMessageCount);
	}, [messages, renderedMessageCount]);

	const hasMoreRef = useRef(hasMoreMessages);
	hasMoreRef.current = hasMoreMessages;

	// After React commits new DOM from a page load, scroll the
	// anchor element back to its original viewport offset. Uses
	// data-section-anchor attributes to find the element by
	// identity, avoiding brittle index arithmetic.
	useLayoutEffect(() => {
		void renderedMessageCount;
		const container = scrollContainerRef?.current;
		const anchor = anchorRef.current;
		if (!container || !anchor) return;
		anchorRef.current = null;

		restoreAnchor(container, anchor);
	}, [renderedMessageCount, scrollContainerRef]);

	// Re-arm the loading gate after the browser paints.
	useEffect(() => {
		void renderedMessageCount;
		if (!isLoadingRef.current) return;
		const id = requestAnimationFrame(() => {
			isLoadingRef.current = false;
		});
		return () => cancelAnimationFrame(id);
	}, [renderedMessageCount]);

	// Create the observer once per pageSize/resetKey cycle.
	// biome-ignore lint/correctness/useExhaustiveDependencies: resetKey is intentionally included so the observer is recreated when the conversation changes.
	useEffect(() => {
		const observer = new IntersectionObserver(
			(entries) => {
				if (
					entries[0]?.isIntersecting &&
					hasMoreRef.current &&
					!isLoadingRef.current
				) {
					const container = scrollContainerRef?.current;
					if (container) {
						anchorRef.current = captureAnchor(container);
					}
					isLoadingRef.current = true;
					setRenderedMessageCount((prev) => prev + pageSize);
				}
			},
			{ rootMargin: "200px" },
		);
		observerRef.current = observer;

		if (sentinelNodeRef.current) {
			observer.observe(sentinelNodeRef.current);
		}

		return () => {
			observer.disconnect();
			observerRef.current = null;
		};
	}, [pageSize, resetKey, scrollContainerRef]);

	const loadMoreSentinelRef = useCallback(
		(node: HTMLDivElement | null) => {
			sentinelNodeRef.current = node;
			const observer = observerRef.current;
			if (!observer) return;
			observer.disconnect();
			if (node) {
				observer.observe(node);
			}
		},
		[],
	);

	return {
		hasMoreMessages,
		windowedMessages,
		loadMoreSentinelRef,
	};
};
