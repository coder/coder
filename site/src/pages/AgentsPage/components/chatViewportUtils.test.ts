import { describe, expect, it } from "vitest";
import {
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

describe("chatViewportUtils", () => {
	it("enters following mode when bottom gap is within threshold", () => {
		expect(getScrollMode(FOLLOW_THRESHOLD_PX, FOLLOW_THRESHOLD_PX)).toBe(
			"following-latest",
		);
	});

	it("exits following mode when bottom gap exceeds threshold", () => {
		expect(getScrollMode(FOLLOW_THRESHOLD_PX + 1)).toBe("detached");
	});

	it("restores anchor position after prepend-like layout growth", () => {
		expect(
			restoreAnchorScrollTop({
				currentScrollTop: 700,
				currentAnchorTop: 210,
				targetOffsetTop: 110,
			}),
		).toBe(800);
	});

	it("preserves anchor position when content shrinks", () => {
		expect(
			restoreAnchorScrollTop({
				currentScrollTop: 800,
				currentAnchorTop: 110,
				targetOffsetTop: 210,
			}),
		).toBe(700);
	});

	it("translates a live stream tail snapshot to the latest durable anchor", () => {
		const content = document.createElement("div");
		const durableOne = document.createElement("div");
		durableOne.dataset.chatAnchor = "true";
		durableOne.dataset.chatAnchorId = "message-1";
		const durableTwo = document.createElement("div");
		durableTwo.dataset.chatAnchor = "true";
		durableTwo.dataset.chatAnchorId = "message-2";
		content.append(durableOne, durableTwo);

		expect(
			resolveAnchorTarget({
				content,
				anchorId: LIVE_STREAM_TAIL_ANCHOR_ID,
			}),
		).toEqual({ anchorId: "message-2", element: durableTwo });
	});

	it("prefers the exact anchor match when the live stream tail is still mounted", () => {
		const content = document.createElement("div");
		const durable = document.createElement("div");
		durable.dataset.chatAnchor = "true";
		durable.dataset.chatAnchorId = "message-2";
		const liveTail = document.createElement("div");
		liveTail.dataset.chatAnchor = "true";
		liveTail.dataset.chatAnchorId = LIVE_STREAM_TAIL_ANCHOR_ID;
		content.append(durable, liveTail);

		expect(
			resolveAnchorTarget({
				content,
				anchorId: LIVE_STREAM_TAIL_ANCHOR_ID,
			}),
		).toEqual({
			anchorId: LIVE_STREAM_TAIL_ANCHOR_ID,
			element: liveTail,
		});
	});

	it("reports boundary scrolling for nested scrollers", () => {
		expect(
			canElementScrollInDirection(
				{ scrollTop: 0, clientHeight: 100, scrollHeight: 400 },
				-30,
			),
		).toBe(false);
		expect(
			canElementScrollInDirection(
				{ scrollTop: 50, clientHeight: 100, scrollHeight: 400 },
				-30,
			),
		).toBe(true);
		expect(
			canElementScrollInDirection(
				{ scrollTop: 300, clientHeight: 100, scrollHeight: 400 },
				40,
			),
		).toBe(false);
	});

	it("finds the nearest scrollable ancestor before the chat viewport", () => {
		const container = document.createElement("div");
		const scrollable = document.createElement("div");
		const child = document.createElement("div");
		container.appendChild(scrollable);
		scrollable.appendChild(child);
		document.body.appendChild(container);

		Object.defineProperty(scrollable, "clientHeight", {
			configurable: true,
			value: 100,
		});
		Object.defineProperty(scrollable, "scrollHeight", {
			configurable: true,
			value: 300,
		});
		scrollable.style.overflowY = "auto";

		expect(findNearestScrollableAncestor(child, container)).toBe(scrollable);

		container.remove();
	});

	it("calculates the visible bottom gap", () => {
		expect(
			getBottomGap({ scrollHeight: 1500, clientHeight: 500, scrollTop: 950 }),
		).toBe(50);
	});

	it("keeps pinned preview inactive until the user message leaves the viewport", () => {
		expect(
			getPinnedPreviewMetrics({
				messageHeight: 160,
				scrolledPast: 0,
			}),
		).toEqual({
			active: false,
			clipHeight: 160,
			fadeOpacity: 0,
			overlayOpacity: 0,
		});
	});

	it("clamps pinned preview height and increases fade as it compresses", () => {
		const metrics = getPinnedPreviewMetrics({
			messageHeight: 220,
			scrolledPast: 180,
		});
		expect(metrics.active).toBe(true);
		expect(metrics.clipHeight).toBe(PINNED_PREVIEW_MIN_HEIGHT_PX);
		expect(metrics.fadeOpacity).toBeGreaterThan(0);
		expect(metrics.overlayOpacity).toBe(1);
	});
});
