import { beforeEach, describe, expect, it } from "vitest";
import {
	type AnchorSnapshot,
	canElementScrollInDirection,
	FOLLOW_THRESHOLD_PX,
	findNearestScrollableAncestor,
	getBottomGap,
	getPinnedPreviewMetrics,
	getScrollMode,
	LIVE_STREAM_TAIL_ANCHOR_ID,
	PINNED_PREVIEW_MIN_HEIGHT_PX,
	resolveAnchorTarget,
	restoreAnchorScrollTop,
} from "./chatViewportUtils";

const setScrollMetrics = (
	element: HTMLElement,
	metrics: {
		scrollHeight: number;
		clientHeight: number;
		scrollTop: number;
	},
): void => {
	Object.defineProperty(element, "scrollHeight", {
		configurable: true,
		value: metrics.scrollHeight,
	});
	Object.defineProperty(element, "clientHeight", {
		configurable: true,
		value: metrics.clientHeight,
	});
	Object.defineProperty(element, "scrollTop", {
		configurable: true,
		writable: true,
		value: metrics.scrollTop,
	});
};

const setRect = (element: Element, top: number, height = 0): void => {
	Object.defineProperty(element, "getBoundingClientRect", {
		configurable: true,
		value: () => new DOMRect(0, top, 0, height),
	});
};

const appendAnchor = (
	container: HTMLElement,
	anchorId: string,
): HTMLDivElement => {
	const anchor = document.createElement("div");
	anchor.dataset.chatAnchor = "true";
	anchor.dataset.chatAnchorId = anchorId;
	container.append(anchor);
	return anchor;
};

describe("chatViewportUtils", () => {
	beforeEach(() => {
		document.body.innerHTML = "";
	});

	it("enters following mode when bottom gap is within threshold", () => {
		const scrollContainer = document.createElement("div");
		setScrollMetrics(scrollContainer, {
			scrollHeight: 600,
			clientHeight: 400,
			scrollTop: 600 - 400 - FOLLOW_THRESHOLD_PX,
		});

		expect(getBottomGap(scrollContainer)).toBe(FOLLOW_THRESHOLD_PX);
		expect(getScrollMode(getBottomGap(scrollContainer))).toBe(
			"following-latest",
		);
	});

	it("exits following mode when bottom gap exceeds threshold", () => {
		const scrollContainer = document.createElement("div");
		setScrollMetrics(scrollContainer, {
			scrollHeight: 600,
			clientHeight: 400,
			scrollTop: 600 - 400 - FOLLOW_THRESHOLD_PX - 1,
		});

		expect(getBottomGap(scrollContainer)).toBe(FOLLOW_THRESHOLD_PX + 1);
		expect(getScrollMode(getBottomGap(scrollContainer))).toBe("detached");
	});

	it("restores anchor position after layout growth", () => {
		const scrollContainer = document.createElement("div");
		setScrollMetrics(scrollContainer, {
			scrollHeight: 1200,
			clientHeight: 400,
			scrollTop: 200,
		});
		setRect(scrollContainer, 50, 400);
		const anchor = document.createElement("div");
		setRect(anchor, 170, 24);
		const snapshot: AnchorSnapshot = {
			anchorId: "message-1",
			offsetTop: 80,
		};

		expect(restoreAnchorScrollTop(scrollContainer, anchor, snapshot)).toBe(240);
	});

	it("restores anchor position after layout shrink", () => {
		const scrollContainer = document.createElement("div");
		setScrollMetrics(scrollContainer, {
			scrollHeight: 1200,
			clientHeight: 400,
			scrollTop: 240,
		});
		setRect(scrollContainer, 50, 400);
		const anchor = document.createElement("div");
		setRect(anchor, 100, 24);
		const snapshot: AnchorSnapshot = {
			anchorId: "message-1",
			offsetTop: 80,
		};

		expect(restoreAnchorScrollTop(scrollContainer, anchor, snapshot)).toBe(210);
	});

	it("resolves exact anchor matches", () => {
		const scrollContainer = document.createElement("div");
		const firstAnchor = appendAnchor(scrollContainer, "message-1");
		const secondAnchor = appendAnchor(scrollContainer, "message-2");

		expect(
			resolveAnchorTarget(scrollContainer, {
				anchorId: "message-2",
				offsetTop: 0,
			}),
		).toBe(secondAnchor);
		expect(firstAnchor.dataset.chatAnchorId).toBe("message-1");
	});

	it("falls back from live stream tail to latest durable anchor", () => {
		const scrollContainer = document.createElement("div");
		appendAnchor(scrollContainer, "message-1");
		const latestDurableAnchor = appendAnchor(scrollContainer, "message-2");

		expect(
			resolveAnchorTarget(scrollContainer, {
				anchorId: LIVE_STREAM_TAIL_ANCHOR_ID,
				offsetTop: 0,
			}),
		).toBe(latestDurableAnchor);
	});

	it("ignores non-anchor elements when resolving live stream fallback", () => {
		const scrollContainer = document.createElement("div");
		appendAnchor(scrollContainer, "message-1");
		const latestDurableAnchor = appendAnchor(scrollContainer, "message-2");
		const ignoredWrapper = document.createElement("div");
		ignoredWrapper.dataset.chatAnchorIgnore = "true";
		scrollContainer.append(ignoredWrapper);
		appendAnchor(ignoredWrapper, "ignored-anchor");

		expect(
			resolveAnchorTarget(scrollContainer, {
				anchorId: LIVE_STREAM_TAIL_ANCHOR_ID,
				offsetTop: 0,
			}),
		).toBe(latestDurableAnchor);
	});

	it("reports nested scroller boundary availability", () => {
		const nestedScroller = document.createElement("div");
		nestedScroller.style.overflowY = "auto";
		setScrollMetrics(nestedScroller, {
			scrollHeight: 400,
			clientHeight: 100,
			scrollTop: 0,
		});

		expect(canElementScrollInDirection(nestedScroller, "down")).toBe(true);
		expect(canElementScrollInDirection(nestedScroller, "up")).toBe(false);

		nestedScroller.scrollTop = 300;

		expect(canElementScrollInDirection(nestedScroller, "down")).toBe(false);
		expect(canElementScrollInDirection(nestedScroller, "up")).toBe(true);
	});

	it("finds nearest scrollable ancestor before the chat viewport", () => {
		const scrollViewport = document.createElement("div");
		scrollViewport.style.overflowY = "auto";
		setScrollMetrics(scrollViewport, {
			scrollHeight: 800,
			clientHeight: 300,
			scrollTop: 120,
		});
		const wrapper = document.createElement("div");
		const nestedScroller = document.createElement("div");
		nestedScroller.style.overflowY = "auto";
		setScrollMetrics(nestedScroller, {
			scrollHeight: 500,
			clientHeight: 150,
			scrollTop: 20,
		});
		const leaf = document.createElement("div");
		document.body.append(scrollViewport);
		scrollViewport.append(wrapper);
		wrapper.append(nestedScroller);
		nestedScroller.append(leaf);

		expect(findNearestScrollableAncestor(leaf, scrollViewport, "down")).toBe(
			nestedScroller,
		);
	});

	it("calculates pinned preview metrics when inactive and clamped", () => {
		const inactiveMetrics = getPinnedPreviewMetrics({
			fullHeight: 310,
			scrollerHeight: 400,
			scrolledPastTop: 80,
		});

		expect(inactiveMetrics.isActive).toBe(false);
		expect(inactiveMetrics.isTooTall).toBe(true);
		expect(inactiveMetrics.isClamped).toBe(false);
		expect(inactiveMetrics.clipHeight).toBe(310);
		expect(inactiveMetrics.fadeOpacity).toBe(0);
		expect(inactiveMetrics.stickyTop).toBe(8);

		const clampedMetrics = getPinnedPreviewMetrics({
			fullHeight: 140,
			scrollerHeight: 400,
			scrolledPastTop: 200,
			nextAnchorTop: 60,
		});

		expect(clampedMetrics.isActive).toBe(true);
		expect(clampedMetrics.isTooTall).toBe(false);
		expect(clampedMetrics.isClamped).toBe(true);
		expect(clampedMetrics.clipHeight).toBe(PINNED_PREVIEW_MIN_HEIGHT_PX);
		expect(clampedMetrics.fadeOpacity).toBe(1);
		expect(clampedMetrics.stickyTop).toBe(-4);
	});
});
