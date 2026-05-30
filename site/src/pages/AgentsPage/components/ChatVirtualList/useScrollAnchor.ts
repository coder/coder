import {
	type RefObject,
	useCallback,
	useEffect,
	useRef,
	useState,
} from "react";
import { correctedScrollTop, isAtBottom } from "./anchorMath";

type Anchor = { el: HTMLElement; offset: number };

// findTopAnchor returns the first child intersecting the viewport top, together
// with its current offset from the scroller top. That child is what we keep
// fixed when content above it changes height.
function findTopAnchor(
	scroller: HTMLElement,
	content: HTMLElement,
): Anchor | null {
	const scrollerTop = scroller.getBoundingClientRect().top;
	for (const child of content.children) {
		if (!(child instanceof HTMLElement)) {
			continue;
		}
		const rect = child.getBoundingClientRect();
		if (rect.bottom > scrollerTop) {
			return { el: child, offset: rect.top - scrollerTop };
		}
	}
	return null;
}

type ScrollAnchor = {
	scrollerRef: RefObject<HTMLDivElement | null>;
	contentRef: RefObject<HTMLDivElement | null>;
	atBottom: boolean;
	scrollToBottom: () => void;
	maintainPin: () => void;
};

export function useScrollAnchor(): ScrollAnchor {
	const scrollerRef = useRef<HTMLDivElement | null>(null);
	const contentRef = useRef<HTMLDivElement | null>(null);
	const anchorRef = useRef<Anchor | null>(null);
	const stickRef = useRef(true);
	const lastScrollTopRef = useRef(-1);
	const [atBottom, setAtBottom] = useState(true);

	const scrollToBottom = useCallback(() => {
		const scroller = scrollerRef.current;
		if (!scroller) {
			return;
		}
		scroller.scrollTop = scroller.scrollHeight;
		stickRef.current = true;
		setAtBottom(true);
	}, []);

	// maintainPin keeps the viewport pinned to the bottom while the user has not
	// scrolled away. Callers run this in a layout effect on content changes so
	// new messages pin synchronously, independent of ResizeObserver timing.
	const maintainPin = useCallback(() => {
		const scroller = scrollerRef.current;
		if (scroller && stickRef.current) {
			scroller.scrollTop = scroller.scrollHeight;
		}
	}, []);

	useEffect(() => {
		const scroller = scrollerRef.current;
		const content = contentRef.current;
		if (!scroller || !content) {
			return;
		}

		let frame = 0;
		const onScroll = () => {
			const scrollTop = scroller.scrollTop;
			// A real user scroll always moves scrollTop. Layout-induced scroll
			// events keep the same scrollTop, so processing them would recapture
			// the anchor against an already-mutated layout and make the resize
			// correction a no-op. Our own correction does move scrollTop, so it
			// still re-establishes a fresh anchor afterward.
			if (scrollTop === lastScrollTopRef.current) {
				return;
			}
			lastScrollTopRef.current = scrollTop;
			const bottom = isAtBottom(
				scrollTop,
				scroller.scrollHeight,
				scroller.clientHeight,
			);
			stickRef.current = bottom;
			anchorRef.current = bottom ? null : findTopAnchor(scroller, content);
			if (frame) {
				return;
			}
			frame = requestAnimationFrame(() => {
				frame = 0;
				setAtBottom(bottom);
			});
		};

		// ResizeObserver notifications run between layout and paint, so correcting
		// scrollTop here keeps the viewport stable without a visible jump. Safari
		// has no native scroll anchoring, which is why we must do this ourselves.
		const resize = new ResizeObserver(() => {
			if (stickRef.current) {
				scroller.scrollTop = scroller.scrollHeight;
				return;
			}
			const anchor = anchorRef.current;
			if (!anchor) {
				return;
			}
			const offset =
				anchor.el.getBoundingClientRect().top -
				scroller.getBoundingClientRect().top;
			scroller.scrollTop = correctedScrollTop(
				scroller.scrollTop,
				anchor.offset,
				offset,
			);
		});

		scroller.addEventListener("scroll", onScroll, { passive: true });
		resize.observe(content);
		return () => {
			scroller.removeEventListener("scroll", onScroll);
			resize.disconnect();
			if (frame) {
				cancelAnimationFrame(frame);
			}
		};
	}, []);

	return { scrollerRef, contentRef, atBottom, scrollToBottom, maintainPin };
}
