export const FOLLOW_THRESHOLD_PX = 100;
export const PINNED_PREVIEW_MIN_HEIGHT_PX = 72;
export const CHAT_ANCHOR_SELECTOR = "[data-chat-anchor='true']";
export const CHAT_NON_ANCHOR_SELECTOR = "[data-chat-anchor-ignore='true']";
export const LIVE_STREAM_TAIL_ANCHOR_ID = "live-stream-tail";

const STICKY_TOP_PX = 8;
const PINNED_PREVIEW_FADE_RANGE_PX = 40;
const PINNED_PREVIEW_MAX_HEIGHT_RATIO = 0.75;
const EPSILON_PX = 1;

export type ScrollDirection = "up" | "down";

export type ScrollMode = "following-latest" | "detached";

export interface AnchorSnapshot {
	anchorId: string;
	offsetTop: number;
}

interface PinnedPreviewMetricsOptions {
	fullHeight: number;
	scrollerHeight: number;
	scrolledPastTop: number;
	nextAnchorTop?: number;
}

interface PinnedPreviewMetrics {
	clipHeight: number;
	fadeOpacity: number;
	stickyTop: number;
	isActive: boolean;
	isClamped: boolean;
	isTooTall: boolean;
}

const clamp = (value: number, min: number, max: number): number =>
	Math.min(Math.max(value, min), max);

const getMaxScrollTop = (element: HTMLElement): number =>
	Math.max(element.scrollHeight - element.clientHeight, 0);

const getAnchorElements = (scrollContainer: HTMLElement): HTMLElement[] =>
	Array.from(
		scrollContainer.querySelectorAll<HTMLElement>(CHAT_ANCHOR_SELECTOR),
	).filter((element) => !element.closest(CHAT_NON_ANCHOR_SELECTOR));

const isOverflowScrollable = (value: string): boolean =>
	value === "auto" || value === "scroll" || value === "overlay";

export const getBottomGap = (scrollContainer: HTMLElement): number =>
	Math.max(
		scrollContainer.scrollHeight -
			scrollContainer.clientHeight -
			scrollContainer.scrollTop,
		0,
	);

export const getScrollMode = (
	bottomGap: number,
	thresholdPx = FOLLOW_THRESHOLD_PX,
): ScrollMode => (bottomGap <= thresholdPx ? "following-latest" : "detached");

export const restoreAnchorScrollTop = (
	scrollContainer: HTMLElement,
	anchorElement: HTMLElement,
	snapshot: AnchorSnapshot,
): number => {
	const scrollContainerTop = scrollContainer.getBoundingClientRect().top;
	const anchorTop = anchorElement.getBoundingClientRect().top;
	const currentOffsetTop = anchorTop - scrollContainerTop;
	const nextScrollTop =
		scrollContainer.scrollTop + currentOffsetTop - snapshot.offsetTop;

	return clamp(nextScrollTop, 0, getMaxScrollTop(scrollContainer));
};

export const restorePrependScrollTop = (
	scrollContainer: HTMLElement,
	previousScrollHeight: number,
	previousScrollTop: number,
): number => {
	const scrollHeightDelta = Math.max(
		scrollContainer.scrollHeight - previousScrollHeight,
		0,
	);
	return clamp(
		previousScrollTop + scrollHeightDelta,
		0,
		getMaxScrollTop(scrollContainer),
	);
};

export const resolveAnchorTarget = (
	scrollContainer: HTMLElement,
	snapshot: AnchorSnapshot,
): HTMLElement | null => {
	const anchors = getAnchorElements(scrollContainer);
	const exactMatch = anchors.find(
		(anchor) => anchor.dataset.chatAnchorId === snapshot.anchorId,
	);
	if (exactMatch) {
		return exactMatch;
	}
	if (snapshot.anchorId !== LIVE_STREAM_TAIL_ANCHOR_ID) {
		return null;
	}

	for (let index = anchors.length - 1; index >= 0; index -= 1) {
		const anchor = anchors[index];
		if (anchor.dataset.chatAnchorId !== LIVE_STREAM_TAIL_ANCHOR_ID) {
			return anchor;
		}
	}

	return null;
};

export const canElementScrollInDirection = (
	element: HTMLElement,
	direction: ScrollDirection,
): boolean => {
	const style = getComputedStyle(element);
	const overflowY = style.overflowY || style.overflow;
	if (!isOverflowScrollable(overflowY)) {
		return false;
	}

	const maxScrollTop = getMaxScrollTop(element);
	if (maxScrollTop <= EPSILON_PX) {
		return false;
	}

	if (direction === "up") {
		return element.scrollTop > EPSILON_PX;
	}

	return element.scrollTop < maxScrollTop - EPSILON_PX;
};

export const findNearestScrollableAncestor = (
	element: HTMLElement,
	boundary: HTMLElement | null,
	direction: ScrollDirection,
): HTMLElement | null => {
	let current = element.parentElement;
	while (current && current !== boundary) {
		if (canElementScrollInDirection(current, direction)) {
			return current;
		}
		current = current.parentElement;
	}

	return null;
};

export const getPinnedPreviewMetrics = ({
	fullHeight,
	scrollerHeight,
	scrolledPastTop,
	nextAnchorTop,
}: PinnedPreviewMetricsOptions): PinnedPreviewMetrics => {
	const isTooTall =
		fullHeight > scrollerHeight * PINNED_PREVIEW_MAX_HEIGHT_RATIO;
	if (isTooTall) {
		return {
			clipHeight: fullHeight,
			fadeOpacity: 0,
			stickyTop: STICKY_TOP_PX,
			isActive: false,
			isClamped: false,
			isTooTall,
		};
	}

	if (scrolledPastTop <= 0) {
		return {
			clipHeight: fullHeight,
			fadeOpacity: 0,
			stickyTop: STICKY_TOP_PX,
			isActive: false,
			isClamped: false,
			isTooTall,
		};
	}

	const clipHeight = Math.max(
		fullHeight - scrolledPastTop,
		PINNED_PREVIEW_MIN_HEIGHT_PX,
	);
	const isClamped = clipHeight === PINNED_PREVIEW_MIN_HEIGHT_PX;
	const fadeOpacity = clamp(
		(PINNED_PREVIEW_MIN_HEIGHT_PX + PINNED_PREVIEW_FADE_RANGE_PX - clipHeight) /
			PINNED_PREVIEW_FADE_RANGE_PX,
		0,
		1,
	);
	const stickyTop =
		nextAnchorTop === undefined
			? STICKY_TOP_PX
			: Math.min(STICKY_TOP_PX, nextAnchorTop - clipHeight + STICKY_TOP_PX);

	return {
		clipHeight,
		fadeOpacity,
		stickyTop,
		isActive: true,
		isClamped,
		isTooTall,
	};
};
