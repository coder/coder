const BOTTOM_THRESHOLD_PX = 16;

// isAtBottom reports whether the scroller is within `threshold` pixels of the
// bottom. Used to decide between pinning to the bottom and holding an anchor.
export function isAtBottom(
	scrollTop: number,
	scrollHeight: number,
	clientHeight: number,
	threshold = BOTTOM_THRESHOLD_PX,
): boolean {
	return scrollHeight - scrollTop - clientHeight <= threshold;
}

// correctedScrollTop returns the scrollTop that keeps an anchor element visually
// fixed after content above it changed height. It works from the anchor's
// content position (scrollTop + viewport offset) at capture versus now, so a
// scroll that happened between capture and restore is not mistaken for a content
// shift and never snaps the viewport back to the capture position.
export function correctedScrollTop(
	currentScrollTop: number,
	savedContentTop: number,
	currentContentTop: number,
): number {
	return currentScrollTop + (currentContentTop - savedContentTop);
}
