import { render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	getAnchoredOverlayPosition,
	useAnchoredOverlayPosition,
} from "./useAnchoredOverlayPosition";

// Layout viewport 800px tall (the hook reads it from a fixed probe); the
// composer sits at the bottom unless a case says otherwise.
const layoutViewportBottom = 800;
const composer = { left: 16, top: 600, width: 358 };

describe("getAnchoredOverlayPosition", () => {
	it.each([
		{
			name: "keyboard closed: 8px above the composer, capped 16px below the top",
			anchor: composer,
			viewport: { offsetTop: 0, height: 800 },
			expected: { left: 16, width: 358, bottom: 208, maxHeight: 576 },
		},
		{
			name: "keyboard covering the composer: lifted above the keyboard",
			anchor: { ...composer, top: 650 },
			viewport: { offsetTop: 0, height: 500 },
			expected: { left: 16, width: 358, bottom: 308, maxHeight: 476 },
		},
		{
			name: "visual viewport panned down: cap shrinks by the pan offset",
			anchor: composer,
			viewport: { offsetTop: 100, height: 700 },
			expected: { left: 16, width: 358, bottom: 208, maxHeight: 476 },
		},
		{
			name: "offset larger than the space above: ignored as a settling WebKit value",
			anchor: composer,
			viewport: { offsetTop: 650, height: 800 },
			expected: { left: 16, width: 358, bottom: 208, maxHeight: 576 },
		},
		{
			name: "composer near the top: height floors at 96px",
			anchor: { ...composer, top: 60 },
			viewport: { offsetTop: 0, height: 800 },
			expected: { left: 16, width: 358, bottom: 748, maxHeight: 96 },
		},
		{
			name: "no visual viewport: layout viewport only",
			anchor: composer,
			viewport: null,
			expected: { left: 16, width: 358, bottom: 208, maxHeight: 576 },
		},
	])("$name", ({ anchor, viewport, expected }) => {
		expect(
			getAnchoredOverlayPosition(anchor, layoutViewportBottom, viewport),
		).toEqual(expected);
	});
});

afterEach(() => {
	vi.restoreAllMocks();
});

describe("useAnchoredOverlayPosition", () => {
	it("owns geometry on the supplied overlay only while enabled", () => {
		const anchor = document.createElement("button");
		const firstOverlay = document.createElement("div");
		const secondOverlay = document.createElement("div");
		const firstWrite = vi.spyOn(firstOverlay.style, "setProperty");
		const firstCleanup = vi.spyOn(firstOverlay.style, "removeProperty");
		const secondWrite = vi.spyOn(secondOverlay.style, "setProperty");
		const secondCleanup = vi.spyOn(secondOverlay.style, "removeProperty");
		const rootWrite = vi.spyOn(document.documentElement.style, "setProperty");

		const Overlay = ({
			overlay,
			enabled,
		}: {
			overlay: HTMLElement;
			enabled: boolean;
		}) => {
			useAnchoredOverlayPosition(anchor, overlay, enabled);

			return null;
		};

		const view = render(<Overlay overlay={firstOverlay} enabled={false} />);
		expect(firstWrite).not.toHaveBeenCalled();

		view.rerender(<Overlay overlay={firstOverlay} enabled />);
		expect(firstWrite).toHaveBeenCalledTimes(4);

		view.rerender(<Overlay overlay={secondOverlay} enabled />);
		expect(firstCleanup).toHaveBeenCalledTimes(4);
		expect(secondWrite).toHaveBeenCalledTimes(4);

		view.rerender(<Overlay overlay={secondOverlay} enabled={false} />);
		expect(secondCleanup).toHaveBeenCalledTimes(4);

		secondWrite.mockClear();
		view.unmount();
		window.dispatchEvent(new Event("resize"));

		expect(secondWrite).not.toHaveBeenCalled();
		expect(rootWrite).not.toHaveBeenCalled();
	});
});
