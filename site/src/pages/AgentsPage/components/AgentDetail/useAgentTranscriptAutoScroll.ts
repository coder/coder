import type { RefObject } from "react";
import { useEffect, useRef, useState } from "react";

const SCROLL_THRESHOLD = 100;

const isNearBottom = (container: HTMLElement): boolean => {
	const distanceFromBottom =
		container.scrollHeight - container.scrollTop - container.clientHeight;
	return distanceFromBottom <= SCROLL_THRESHOLD;
};

type RafIdRef = { current: number | null };
type BooleanRef = { current: boolean };

const cancelRaf = (rafIdRef: RafIdRef) => {
	if (rafIdRef.current !== null) {
		cancelAnimationFrame(rafIdRef.current);
		rafIdRef.current = null;
	}
};

const cancelPendingPins = (
	pinOuterRafIdRef: RafIdRef,
	pinInnerRafIdRef: RafIdRef,
) => {
	cancelRaf(pinOuterRafIdRef);
	cancelRaf(pinInnerRafIdRef);
};

const cancelScrollStateUpdate = (scrollStateRafIdRef: RafIdRef) => {
	cancelRaf(scrollStateRafIdRef);
};

const scheduleBottomPin = (
	scrollContainerRef: RefObject<HTMLDivElement | null>,
	autoScrollRef: BooleanRef,
	pinOuterRafIdRef: RafIdRef,
	pinInnerRafIdRef: RafIdRef,
) => {
	const container = scrollContainerRef.current;
	if (!container) return;

	cancelPendingPins(pinOuterRafIdRef, pinInnerRafIdRef);
	pinOuterRafIdRef.current = requestAnimationFrame(() => {
		pinOuterRafIdRef.current = null;
		pinInnerRafIdRef.current = requestAnimationFrame(() => {
			pinInnerRafIdRef.current = null;
			const nextContainer = scrollContainerRef.current;
			if (!nextContainer || !autoScrollRef.current) return;
			nextContainer.scrollTop = Math.max(
				nextContainer.scrollHeight - nextContainer.clientHeight,
				0,
			);
		});
	});
};

interface UseAgentTranscriptAutoScrollResult {
	contentRef: RefObject<HTMLDivElement | null>;
	showScrollToBottom: boolean;
	jumpToBottom: () => void;
}

export function useAgentTranscriptAutoScroll(
	scrollContainerRef: RefObject<HTMLDivElement | null>,
): UseAgentTranscriptAutoScrollResult {
	const contentRef = useRef<HTMLDivElement>(null);
	const autoScrollRef = useRef(true);
	const isProgrammaticScrollRef = useRef(false);
	const scrollStateRafIdRef = useRef<number | null>(null);
	const pinOuterRafIdRef = useRef<number | null>(null);
	const pinInnerRafIdRef = useRef<number | null>(null);
	const [showScrollToBottom, setShowScrollToBottom] = useState(false);

	useEffect(() => {
		const container = scrollContainerRef.current;
		if (!container) return;

		const scheduleButtonStateUpdate = () => {
			if (scrollStateRafIdRef.current !== null) return;
			scrollStateRafIdRef.current = requestAnimationFrame(() => {
				scrollStateRafIdRef.current = null;
				const nextContainer = scrollContainerRef.current;
				if (!nextContainer) return;
				const shouldShow = !isNearBottom(nextContainer);
				setShowScrollToBottom((prev) =>
					prev === shouldShow ? prev : shouldShow,
				);
			});
		};

		const handleScroll = () => {
			const nearBottom = isNearBottom(container);

			if (isProgrammaticScrollRef.current) {
				if (nearBottom) {
					isProgrammaticScrollRef.current = false;
					autoScrollRef.current = true;
					cancelScrollStateUpdate(scrollStateRafIdRef);
					setShowScrollToBottom(false);
				}
				return;
			}

			autoScrollRef.current = nearBottom;
			scheduleButtonStateUpdate();
		};

		const handleUserInterrupt = () => {
			isProgrammaticScrollRef.current = false;
		};

		container.addEventListener("scroll", handleScroll, { passive: true });
		container.addEventListener("wheel", handleUserInterrupt, { passive: true });
		container.addEventListener("touchstart", handleUserInterrupt, {
			passive: true,
		});

		scheduleBottomPin(
			scrollContainerRef,
			autoScrollRef,
			pinOuterRafIdRef,
			pinInnerRafIdRef,
		);

		return () => {
			container.removeEventListener("scroll", handleScroll);
			container.removeEventListener("wheel", handleUserInterrupt);
			container.removeEventListener("touchstart", handleUserInterrupt);
		};
	}, [scrollContainerRef]);

	useEffect(() => {
		const container = scrollContainerRef.current;
		const content = contentRef.current;
		if (!container || !content) return;

		const initialRect = content.getBoundingClientRect();
		let prevContentHeight = initialRect.height;

		const observer = new ResizeObserver((entries) => {
			const entry = entries[0];
			const nextHeight =
				entry?.contentRect.height ?? content.getBoundingClientRect().height;
			const heightDelta = nextHeight - prevContentHeight;

			prevContentHeight = nextHeight;

			if (heightDelta < 1 || !autoScrollRef.current) {
				return;
			}

			scheduleBottomPin(
				scrollContainerRef,
				autoScrollRef,
				pinOuterRafIdRef,
				pinInnerRafIdRef,
			);
		});

		observer.observe(content);

		return () => {
			observer.disconnect();
		};
	}, [scrollContainerRef]);

	useEffect(() => {
		const container = scrollContainerRef.current;
		if (!container) return;

		let prevContainerHeight = container.clientHeight;
		const observer = new ResizeObserver((entries) => {
			const nextHeight =
				entries[0]?.contentRect.height ?? container.clientHeight;
			const heightDelta = nextHeight - prevContainerHeight;
			prevContainerHeight = nextHeight;

			if (Math.abs(heightDelta) < 1 || !autoScrollRef.current) {
				return;
			}

			scheduleBottomPin(
				scrollContainerRef,
				autoScrollRef,
				pinOuterRafIdRef,
				pinInnerRafIdRef,
			);
		});

		observer.observe(container);

		return () => {
			observer.disconnect();
		};
	}, [scrollContainerRef]);

	useEffect(() => {
		return () => {
			cancelPendingPins(pinOuterRafIdRef, pinInnerRafIdRef);
			cancelScrollStateUpdate(scrollStateRafIdRef);
		};
	}, []);

	const jumpToBottom = () => {
		const container = scrollContainerRef.current;
		if (!container) return;

		autoScrollRef.current = true;
		isProgrammaticScrollRef.current = true;
		cancelScrollStateUpdate(scrollStateRafIdRef);
		setShowScrollToBottom(false);
		container.scrollTo({
			top: container.scrollHeight - container.clientHeight,
			behavior: "smooth",
		});
	};

	return {
		contentRef,
		showScrollToBottom,
		jumpToBottom,
	};
}
