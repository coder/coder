import {
	type RefObject,
	useCallback,
	useEffect,
	useRef,
	useState,
} from "react";
import { correctedScrollTop, isAtBottom } from "./anchorMath";

// The anchor is keyed by a stable item id so it survives windowing remounts.
// The element ref is a fast path; if it has been unmounted we re-find by id.
// contentTop is the anchor's position in scroll content coordinates
// (scrollTop + viewport offset) at capture; comparing it to the current content
// position isolates content-height changes from any scroll that happened since.
type Anchor = { id: string | null; el: HTMLElement; contentTop: number };

const ITEM_SELECTOR = "[data-chat-item]";

// findTopAnchor returns the first rendered item intersecting the viewport top.
// Spacers and other non-item children are ignored so the anchor is always a
// real element whose offset we can preserve.
function findTopAnchor(
	scroller: HTMLElement,
	content: HTMLElement,
): Anchor | null {
	const scrollerTop = scroller.getBoundingClientRect().top;
	for (const child of content.querySelectorAll(ITEM_SELECTOR)) {
		if (!(child instanceof HTMLElement)) {
			continue;
		}
		const rect = child.getBoundingClientRect();
		if (rect.bottom > scrollerTop) {
			return {
				id: child.getAttribute("data-chat-item-id"),
				el: child,
				contentTop: scroller.scrollTop + (rect.top - scrollerTop),
			};
		}
	}
	return null;
}

function resolveAnchorElement(
	content: HTMLElement,
	anchor: Anchor,
): HTMLElement | null {
	if (anchor.el.isConnected) {
		return anchor.el;
	}
	if (anchor.id == null) {
		return null;
	}
	const found = content.querySelector(
		`[data-chat-item-id="${CSS.escape(anchor.id)}"]`,
	);
	return found instanceof HTMLElement ? found : null;
}

type ScrollAnchor = {
	scrollerRef: RefObject<HTMLDivElement | null>;
	contentRef: RefObject<HTMLDivElement | null>;
	atBottom: boolean;
	scrollToBottom: () => void;
	// captureAnchor records the item currently at the viewport top. Callers run
	// it after every committed render so the anchor always reflects the latest
	// settled layout.
	captureAnchor: () => void;
	// restoreAnchor re-pins to the bottom when sticky, otherwise moves the
	// captured anchor element back to its recorded viewport offset. It is the
	// single owner of scrollTop correction and is idempotent.
	restoreAnchor: () => void;
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

	const captureAnchor = useCallback(() => {
		const scroller = scrollerRef.current;
		const content = contentRef.current;
		if (!scroller || !content) {
			return;
		}
		anchorRef.current = stickRef.current
			? null
			: findTopAnchor(scroller, content);
	}, []);

	const restoreAnchor = useCallback(() => {
		const scroller = scrollerRef.current;
		const content = contentRef.current;
		if (!scroller || !content) {
			return;
		}
		if (stickRef.current) {
			scroller.scrollTop = scroller.scrollHeight;
			return;
		}
		const anchor = anchorRef.current;
		if (!anchor) {
			return;
		}
		const el = resolveAnchorElement(content, anchor);
		if (!el) {
			return;
		}
		const currentContentTop =
			scroller.scrollTop +
			(el.getBoundingClientRect().top - scroller.getBoundingClientRect().top);
		scroller.scrollTop = correctedScrollTop(
			scroller.scrollTop,
			anchor.contentTop,
			currentContentTop,
		);
		// Re-baseline to the anchor's current content position so a second restore
		// for the same change (the content ResizeObserver, a repeated effect, a
		// StrictMode double invoke) computes a zero delta and stays a no-op.
		// Scrolling does not move an element's content position, so this never
		// suppresses a later correction.
		anchor.contentTop = currentContentTop;
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
			// events keep the same scrollTop; ignoring them avoids flipping the
			// stick state when our own correction nudges the position.
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
			if (frame) {
				return;
			}
			frame = requestAnimationFrame(() => {
				frame = 0;
				setAtBottom(bottom);
			});
		};

		// The ResizeObserver is a fallback trigger for async intrinsic growth
		// (streaming markdown, image load, late highlight) that happens without a
		// React render. Windowing, measurement, and prepend are driven explicitly
		// by the container's layout effect instead.
		const resize = new ResizeObserver(() => {
			restoreAnchor();
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
	}, [restoreAnchor]);

	return {
		scrollerRef,
		contentRef,
		atBottom,
		scrollToBottom,
		captureAnchor,
		restoreAnchor,
	};
}
