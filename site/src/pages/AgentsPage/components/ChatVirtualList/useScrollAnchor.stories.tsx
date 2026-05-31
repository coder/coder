import type { Meta, StoryObj } from "@storybook/react-vite";
import { type FC, useLayoutEffect, useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { useScrollAnchor } from "./useScrollAnchor";

// Exercises the capture/restore primitive in isolation, without the windowing
// renderer, so a failure here points at the anchor hook rather than the layout
// math.

const ROWS = 40;
const ROW_HEIGHT = 100;
const SPACER_HEIGHT = 50;

const AnchorHarness: FC = () => {
	const { scrollerRef, contentRef, captureAnchor, restoreAnchor } =
		useScrollAnchor();
	const [heights, setHeights] = useState<Record<number, number>>({});
	const [version, setVersion] = useState(0);

	const mutate = (changes: Record<number, number>) => {
		// Record the item at the viewport top before the heights change, then
		// restore it once the mutation commits. This mirrors how the container
		// brackets a content mutation around the layout effect.
		captureAnchor();
		setHeights((prev) => ({ ...prev, ...changes }));
		setVersion((value) => value + 1);
	};

	// growNoCapture changes heights and triggers a restore without capturing
	// first, so a test can capture, scroll, then mutate and prove the restore
	// honors the post-scroll position instead of the capture position.
	const growNoCapture = (changes: Record<number, number>) => {
		setHeights((prev) => ({ ...prev, ...changes }));
		setVersion((value) => value + 1);
	};

	// biome-ignore lint/correctness/useExhaustiveDependencies(version): version is the layout-mutation trigger; restore reads the DOM and must run after each mutation commits.
	useLayoutEffect(() => {
		restoreAnchor();
	}, [version, restoreAnchor]);

	return (
		<div className="flex h-[400px] w-[600px] flex-col">
			<div className="flex gap-1">
				<button
					type="button"
					data-testid="grow-above"
					onClick={() => mutate({ 5: 300 })}
				>
					grow-above
				</button>
				<button
					type="button"
					data-testid="zero-net"
					onClick={() => mutate({ 5: 200, 35: 0 })}
				>
					zero-net
				</button>
				<button
					type="button"
					data-testid="capture"
					onClick={() => captureAnchor()}
				>
					capture
				</button>
				<button
					type="button"
					data-testid="grow-no-capture"
					onClick={() => growNoCapture({ 5: 300 })}
				>
					grow-no-capture
				</button>
			</div>
			<div
				ref={scrollerRef}
				data-testid="scroll-container"
				className="flex min-h-0 flex-1 flex-col overflow-y-auto [overflow-anchor:none]"
			>
				<div ref={contentRef} className="flex flex-col">
					<div data-virtual-spacer="" style={{ height: SPACER_HEIGHT }} />
					{Array.from({ length: ROWS }, (_, i) => (
						<div
							key={i}
							data-chat-item=""
							data-chat-item-id={`row-${i}`}
							data-testid={`row-${i}`}
							style={{ height: heights[i] ?? ROW_HEIGHT }}
						>
							row {i}
						</div>
					))}
				</div>
			</div>
		</div>
	);
};

const meta: Meta<typeof AnchorHarness> = {
	title: "pages/AgentsPage/ChatVirtualList/useScrollAnchor",
	component: AnchorHarness,
};
export default meta;

type Story = StoryObj<typeof AnchorHarness>;

const offsetFromTop = (scroller: HTMLElement, el: HTMLElement): number =>
	el.getBoundingClientRect().top - scroller.getBoundingClientRect().top;

// Scroll so row 20 sits at the viewport top: spacer + 20 rows.
const SCROLL_TO_ROW_20 = SPACER_HEIGHT + 20 * ROW_HEIGHT;

export const AnchorSurvivesAboveMutation: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		scroller.scrollTop = SCROLL_TO_ROW_20;
		scroller.dispatchEvent(new Event("scroll"));
		await waitFor(() => expect(scroller.scrollTop).toBe(SCROLL_TO_ROW_20));
		const before = offsetFromTop(scroller, canvas.getByTestId("row-20"));
		await userEvent.click(canvas.getByTestId("grow-above"));
		await waitFor(() =>
			expect(
				Math.abs(
					offsetFromTop(scroller, canvas.getByTestId("row-20")) - before,
				),
			).toBeLessThanOrEqual(2),
		);
	},
};

export const AnchorSurvivesZeroNetMutation: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		scroller.scrollTop = SCROLL_TO_ROW_20;
		scroller.dispatchEvent(new Event("scroll"));
		await waitFor(() => expect(scroller.scrollTop).toBe(SCROLL_TO_ROW_20));
		const before = offsetFromTop(scroller, canvas.getByTestId("row-20"));
		// Grows a row above the anchor and shrinks a row below it by the same
		// amount: total height is unchanged, so a content ResizeObserver never
		// fires, yet the anchor moved. Only capture/restore corrects this.
		await userEvent.click(canvas.getByTestId("zero-net"));
		await waitFor(() =>
			expect(
				Math.abs(
					offsetFromTop(scroller, canvas.getByTestId("row-20")) - before,
				),
			).toBeLessThanOrEqual(2),
		);
	},
};

// Reproduces the Safari instability: the anchor is captured at one scroll
// position, the user scrolls, then content above changes. The restore must
// compensate only the content growth and leave the viewport where the scroll
// put it. The earlier offset-delta formula snapped back to the capture point.
export const AnchorSurvivesScrollBetweenCaptureAndMutation: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		scroller.scrollTop = SCROLL_TO_ROW_20;
		scroller.dispatchEvent(new Event("scroll"));
		await waitFor(() => expect(scroller.scrollTop).toBe(SCROLL_TO_ROW_20));
		// Capture row 20 at the top, then scroll up 200px without re-capturing,
		// mimicking a continuous scroll where capture ran a frame earlier.
		await userEvent.click(canvas.getByTestId("capture"));
		scroller.scrollTop = SCROLL_TO_ROW_20 - 200;
		scroller.dispatchEvent(new Event("scroll"));
		await waitFor(() =>
			expect(scroller.scrollTop).toBe(SCROLL_TO_ROW_20 - 200),
		);
		const before = offsetFromTop(scroller, canvas.getByTestId("row-20"));
		await userEvent.click(canvas.getByTestId("grow-no-capture"));
		await waitFor(() =>
			expect(
				Math.abs(
					offsetFromTop(scroller, canvas.getByTestId("row-20")) - before,
				),
			).toBeLessThanOrEqual(2),
		);
	},
};
