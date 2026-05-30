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

// correctedScrollTop returns the scrollTop that keeps an anchor element at its
// previous viewport offset after content above it changed height.
export function correctedScrollTop(
	scrollTop: number,
	previousOffset: number,
	currentOffset: number,
): number {
	return scrollTop + (currentOffset - previousOffset);
}
