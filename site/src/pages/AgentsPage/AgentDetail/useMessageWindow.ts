import type * as TypesGen from "api/typesGenerated";
import {
	type RefObject,
	useCallback,
	useEffect,
	useLayoutEffect,
	useMemo,
	useRef,
	useState,
} from "react";

const DEFAULT_PAGE_SIZE = 50;

// Maximum number of requestAnimationFrame correction frames to run
// after loading older messages. Each frame checks whether the anchor
// element drifted due to content-visibility layout recalculations
// and adjusts scrollTop to compensate.
const MAX_CORRECTION_FRAMES = 20;
// Number of consecutive frames where the anchor must be within 1px
// of its target before we consider layout stable.
const STABLE_FRAME_THRESHOLD = 3;

type UseMessageWindowOptions = {
	messages: readonly TypesGen.ChatMessage[];
	resetKey?: string;
	pageSize?: number;
	scrollContainerRef?: RefObject<HTMLElement | null>;
};

export const useMessageWindow = ({
	messages,
	resetKey,
	pageSize = DEFAULT_PAGE_SIZE,
	scrollContainerRef,
}: UseMessageWindowOptions) => {
	const [renderedMessageCount, setRenderedMessageCount] = useState(pageSize);
	const loadMoreSentinelRef = useRef<HTMLDivElement | null>(null);

	// Scroll anchor state — captured in the IntersectionObserver
	// callback (before React re-renders) and consumed by a
	// useLayoutEffect that fires after React commits the new DOM.
	const anchorRef = useRef<{
		element: Element;
		offsetFromContainer: number;
	} | null>(null);
	const correctionFrameRef = useRef(0);
	const prevWindowedLengthRef = useRef(0);

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

	// Capture the topmost visible section element within the scroll
	// container and its offset from the container's top edge. Called
	// in the IntersectionObserver callback, before the state update
	// that triggers React's re-render.
	//
	// DOM nesting from scrollContainer:
	//   scrollContainer > div.px-4 > div.mx-auto (ConversationTimeline)
	//     > div.flex-col > [sentinel, ...section divs]
	//
	// We want the section divs (which carry content-visibility: auto),
	// so we query four levels deep from the scroll container's first
	// child element.
	const captureScrollAnchor = useCallback(() => {
		const container = scrollContainerRef?.current;
		if (!container) return;

		const containerRect = container.getBoundingClientRect();
		const content = container.firstElementChild;
		if (!content) return;

		for (const child of content.querySelectorAll(":scope > * > * > *")) {
			const rect = child.getBoundingClientRect();
			if (rect.bottom > containerRect.top && rect.height > 0) {
				anchorRef.current = {
					element: child,
					offsetFromContainer: rect.top - containerRect.top,
				};
				return;
			}
		}
	}, [scrollContainerRef]);

	// Start a requestAnimationFrame loop that measures the anchor
	// element's current position and nudges scrollTop until the
	// layout has stabilized. This handles the asynchronous height
	// changes caused by content-visibility: auto.
	const startScrollCorrection = useCallback(() => {
		const container = scrollContainerRef?.current;
		const anchor = anchorRef.current;
		if (!container || !anchor) return;

		let frames = 0;
		let stableFrames = 0;

		const correct = () => {
			const containerRect = container.getBoundingClientRect();
			const anchorRect = anchor.element.getBoundingClientRect();
			const currentOffset = anchorRect.top - containerRect.top;
			const drift = currentOffset - anchor.offsetFromContainer;

			if (Math.abs(drift) > 1) {
				container.scrollTop -= drift;
				stableFrames = 0;
			} else {
				stableFrames++;
			}

			frames++;
			if (
				stableFrames < STABLE_FRAME_THRESHOLD &&
				frames < MAX_CORRECTION_FRAMES
			) {
				correctionFrameRef.current = requestAnimationFrame(correct);
			} else {
				anchorRef.current = null;
				correctionFrameRef.current = 0;
			}
		};

		correctionFrameRef.current = requestAnimationFrame(correct);
	}, [scrollContainerRef]);

	// After React commits new DOM nodes from a load-more, kick off
	// the scroll correction. useLayoutEffect fires synchronously
	// after React's DOM mutations but before the browser paints,
	// guaranteeing the new message elements are in the DOM when the
	// first rAF callback runs.
	useLayoutEffect(() => {
		const currentLength = windowedMessages.length;
		const prevLength = prevWindowedLengthRef.current;
		prevWindowedLengthRef.current = currentLength;

		// Only correct when messages were prepended (load-more),
		// not on initial render or when messages shrink (reset).
		if (currentLength > prevLength && prevLength > 0 && anchorRef.current) {
			startScrollCorrection();
		}
	}, [windowedMessages, startScrollCorrection]);

	// Clean up any pending correction frame on unmount.
	useEffect(() => {
		return () => {
			if (correctionFrameRef.current) {
				cancelAnimationFrame(correctionFrameRef.current);
			}
		};
	}, []);

	useEffect(() => {
		const node = loadMoreSentinelRef.current;
		if (!node || !hasMoreMessages) {
			return;
		}
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) {
					// Capture the anchor while the DOM is still in
					// its current state. The useLayoutEffect above
					// will start the correction after React commits.
					captureScrollAnchor();
					setRenderedMessageCount((prev) => prev + pageSize);
				}
			},
			{ rootMargin: "200px" },
		);
		observer.observe(node);
		return () => observer.disconnect();
	}, [hasMoreMessages, pageSize, captureScrollAnchor]);

	return {
		hasMoreMessages,
		windowedMessages,
		loadMoreSentinelRef,
	};
};
