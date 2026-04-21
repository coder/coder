export const FOLLOW_THRESHOLD_PX = 100;
export const PINNED_PREVIEW_MIN_HEIGHT_PX = 72;
export const CHAT_ANCHOR_SELECTOR = "[data-chat-anchor='true']";
export const CHAT_NON_ANCHOR_SELECTOR = "[data-chat-anchor-ignore='true']";
export const LIVE_STREAM_TAIL_ANCHOR_ID = "live-stream-tail";

export type ScrollMode = "following-latest" | "detached";

export interface AnchorSnapshot {
	anchorId: string;
	offsetTop: number;
}

interface ResolvedAnchorTarget {
	anchorId: string;
	element: HTMLElement;
}

export const getBottomGap = ({
	scrollHeight,
	clientHeight,
	scrollTop,
}: {
	scrollHeight: number;
	clientHeight: number;
	scrollTop: number;
}): number => Math.max(0, scrollHeight - clientHeight - scrollTop);

export const getScrollMode = (
	bottomGap: number,
	threshold = FOLLOW_THRESHOLD_PX,
): ScrollMode => (bottomGap <= threshold ? "following-latest" : "detached");

export const restoreAnchorScrollTop = ({
	currentScrollTop,
	currentAnchorTop,
	targetOffsetTop,
}: {
	currentScrollTop: number;
	currentAnchorTop: number;
	targetOffsetTop: number;
}): number =>
	Math.max(0, currentScrollTop + (currentAnchorTop - targetOffsetTop));

export const resolveAnchorTarget = ({
	content,
	anchorId,
}: {
	content: ParentNode;
	anchorId: string;
}): ResolvedAnchorTarget | null => {
	const exactMatch = content.querySelector<HTMLElement>(
		`[data-chat-anchor-id='${CSS.escape(anchorId)}']`,
	);
	if (exactMatch) {
		return { anchorId, element: exactMatch };
	}
	if (anchorId !== LIVE_STREAM_TAIL_ANCHOR_ID) {
		return null;
	}

	// When streamed output commits into a durable assistant message, the
	// transient tail unmounts before detached anchor restoration runs. Falling
	// back to the last remaining durable anchor preserves the reader's place at
	// the tail boundary instead of silently losing the snapshot.
	const durableAnchors = Array.from(
		content.querySelectorAll<HTMLElement>(CHAT_ANCHOR_SELECTOR),
	).filter((anchor) => {
		if (anchor.closest(CHAT_NON_ANCHOR_SELECTOR)) {
			return false;
		}
		const nextAnchorId = anchor.dataset.chatAnchorId;
		return Boolean(nextAnchorId) && nextAnchorId !== LIVE_STREAM_TAIL_ANCHOR_ID;
	});
	const replacement = durableAnchors.at(-1);
	if (!replacement?.dataset.chatAnchorId) {
		return null;
	}
	return {
		anchorId: replacement.dataset.chatAnchorId,
		element: replacement,
	};
};

export const canElementScrollInDirection = (
	element: Pick<HTMLElement, "scrollTop" | "scrollHeight" | "clientHeight">,
	deltaY: number,
): boolean => {
	if (deltaY < 0) {
		return element.scrollTop > 0;
	}
	if (deltaY > 0) {
		return element.scrollTop + element.clientHeight < element.scrollHeight - 1;
	}
	return false;
};

const isVerticallyScrollable = (element: HTMLElement): boolean => {
	if (element.scrollHeight <= element.clientHeight + 1) {
		return false;
	}

	const style = getComputedStyle(element);
	return (
		style.overflowY === "auto" ||
		style.overflowY === "scroll" ||
		style.overflow === "auto" ||
		style.overflow === "scroll"
	);
};

export const findNearestScrollableAncestor = (
	target: EventTarget | null,
	container: HTMLElement,
): HTMLElement | null => {
	let element = target instanceof HTMLElement ? target : null;
	while (element && element !== container) {
		if (isVerticallyScrollable(element)) {
			return element;
		}
		element = element.parentElement;
	}
	return null;
};

interface PinnedPreviewMetrics {
	active: boolean;
	clipHeight: number;
	fadeOpacity: number;
	overlayOpacity: number;
}

export const getPinnedPreviewMetrics = ({
	messageHeight,
	scrolledPast,
	minimumHeight = PINNED_PREVIEW_MIN_HEIGHT_PX,
}: {
	messageHeight: number;
	scrolledPast: number;
	minimumHeight?: number;
}): PinnedPreviewMetrics => {
	if (scrolledPast <= 0 || messageHeight <= 0) {
		return {
			active: false,
			clipHeight: messageHeight,
			fadeOpacity: 0,
			overlayOpacity: 0,
		};
	}

	const clipHeight = Math.max(minimumHeight, messageHeight - scrolledPast);
	const fadeRange = 40;
	const fadeOpacity = Math.max(
		0,
		Math.min((minimumHeight + fadeRange - clipHeight) / fadeRange, 1),
	);
	const overlayOpacity = Math.max(0, Math.min(scrolledPast / 16, 1));

	return {
		active: true,
		clipHeight,
		fadeOpacity,
		overlayOpacity,
	};
};
