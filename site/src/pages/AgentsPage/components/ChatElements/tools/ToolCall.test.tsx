import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
	currentVisibleRows,
	history,
	isViewport,
	nextRow,
	prompt,
	renderTranscript,
	row,
	SCROLLBAR_WIDTH,
	setupMessageScrollerLayout,
	VIEWPORT_WIDTH,
	viewport,
	visibleRowsAfterLayout,
} from "#/testHelpers/messageScrollerLayout";
import { MockResizeObserver } from "#/testHelpers/resizeObserver";
import { MessageScroller } from "#/vendor/message-scroller";
import { ToolCall } from "./ToolCall";

const ToolRow: React.FC<{
	defaultExpanded?: boolean;
	streamedHeight?: number;
}> = ({ defaultExpanded = false, streamedHeight = 0 }) => {
	const [expanded, setExpanded] = useState(defaultExpanded);
	return (
		<MessageScroller.Item
			messageId="reply"
			data-height={streamedHeight + (expanded ? 600 : 40)}
		>
			<ToolCall.Root
				status="completed"
				hasContent
				defaultExpanded={defaultExpanded}
				onExpandedChange={setExpanded}
			>
				<ToolCall.Header iconName="read_file" label="Read config.yaml" />
				<ToolCall.Content>
					<div>File contents</div>
				</ToolCall.Content>
			</ToolCall.Root>
		</MessageScroller.Item>
	);
};

const toggle = () => screen.getByRole("button", { name: "Read config.yaml" });

describe("ToolCall in a MessageScroller", () => {
	setupMessageScrollerLayout();

	it("keeps the reader in place when a tool collapses while following the bottom", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([
			history,
			<ToolRow key="reply" defaultExpanded />,
		]);

		// No scroll event or resize report between the collapse and the output.
		await user.click(toggle());
		await update([history, <ToolRow key="reply" defaultExpanded />, nextRow]);

		expect(await visibleRowsAfterLayout()).not.toContain("next");
	});

	it("holds the anchored prompt when Space collapses a tool", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([history]);
		await update([history, prompt, <ToolRow key="reply" />]);
		await user.click(toggle());
		const before = await visibleRowsAfterLayout();

		await user.keyboard(" ");

		expect(await visibleRowsAfterLayout()).toEqual(before);
	});

	it("holds the anchored prompt when the turn overflows the view from the start", async () => {
		const update = renderTranscript([history]);
		// The turn ends 24px past the view, inside the handoff band.
		const tallPrompt = row("prompt", 320, true);
		await update([history, tallPrompt, <ToolRow key="reply" />]);
		const before = await visibleRowsAfterLayout();

		await update([history, tallPrompt, <ToolRow key="reply" />, nextRow]);

		expect(await visibleRowsAfterLayout()).toEqual(before);
	});

	it("holds the anchored prompt when the reply streams past it after a toggle", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([history]);
		await update([history, prompt, <ToolRow key="reply" />]);
		await user.click(toggle());
		await user.click(toggle());
		const before = await visibleRowsAfterLayout();

		// One line past the room below the prompt.
		const reply = <ToolRow key="reply" streamedHeight={280} />;
		await update([history, prompt, reply]);
		await visibleRowsAfterLayout();
		await update([history, prompt, reply, nextRow]);

		expect(await visibleRowsAfterLayout()).toEqual(before);
	});

	it("holds the anchored prompt when the content remounts after a toggle", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([history]);
		const rows = [history, prompt, <ToolRow key="reply" />];
		await update(rows);
		await user.click(toggle());
		await user.click(toggle());
		const before = await visibleRowsAfterLayout();

		await update(rows, "remounted");

		expect(await visibleRowsAfterLayout()).toEqual(before);
	});

	it("holds the anchored prompt when output lands past the handoff band", async () => {
		const update = renderTranscript([history]);
		await update([history, prompt, <ToolRow key="reply" />]);
		const before = await visibleRowsAfterLayout();

		await update([
			history,
			prompt,
			<ToolRow key="reply" streamedHeight={600} />,
			nextRow,
		]);

		expect(await visibleRowsAfterLayout()).toEqual(before);
	});

	it("keeps a tool above the anchored prompt in view when it expands", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([history, <ToolRow key="reply" />]);
		// The previous turn's tool peeks in above the new prompt.
		await update([history, <ToolRow key="reply" />, prompt, nextRow]);

		await user.click(toggle());

		expect(await visibleRowsAfterLayout()).toContain("reply");
	});

	it("resumes following after a scrollbar drag back to the bottom", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([history, <ToolRow key="reply" />]);
		await user.click(toggle());

		await user.pointer({
			keys: "[MouseLeft>]",
			target: viewport(),
			coords: { offsetX: VIEWPORT_WIDTH - SCROLLBAR_WIDTH / 2 },
		});
		viewport().scrollTop = viewport().scrollHeight;
		fireEvent.scroll(viewport());
		await user.pointer({ keys: "[/MouseLeft]", target: viewport() });
		await update([history, <ToolRow key="reply" />, nextRow]);

		expect(await visibleRowsAfterLayout()).toContain("next");
	});

	it("resumes following when output lands during a scrollbar press at the bottom", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([history, <ToolRow key="reply" />]);

		// At the bottom, a press and release that move nothing fire no scroll.
		await user.pointer({
			keys: "[MouseLeft>]",
			target: viewport(),
			coords: { offsetX: VIEWPORT_WIDTH - SCROLLBAR_WIDTH / 2 },
		});
		await update([history, <ToolRow key="reply" />, nextRow]);
		await visibleRowsAfterLayout();
		await user.pointer({ keys: "[/MouseLeft]", target: viewport() });

		expect(await visibleRowsAfterLayout()).toContain("next");
	});

	it.each([
		["a wheel down", () => fireEvent.wheel(viewport(), { deltaY: 100 })],
		[
			"a swipe up",
			() => {
				fireEvent.touchStart(viewport(), { touches: [{ clientY: 300 }] });
				fireEvent.touchMove(viewport(), { touches: [{ clientY: 100 }] });
			},
		],
	])(
		"resumes following after %s at the bottom after a collapse",
		async (_, gesture) => {
			const user = userEvent.setup();
			const update = renderTranscript([
				history,
				<ToolRow key="reply" defaultExpanded />,
			]);
			await user.click(toggle());
			// The browser reports the collapse's clamp as a scroll.
			fireEvent.scroll(viewport());
			await visibleRowsAfterLayout();

			// At the bottom, the gesture moves nothing and fires no scroll.
			gesture();
			await update([history, <ToolRow key="reply" defaultExpanded />, nextRow]);

			expect(await visibleRowsAfterLayout()).toContain("next");
		},
	);

	it("stays put when a collapse clamps the reader to the bottom", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([
			history,
			<ToolRow key="reply" defaultExpanded />,
		]);
		fireEvent.wheel(viewport());
		viewport().scrollTop -= 100;
		fireEvent.scroll(viewport());

		await user.click(toggle());
		// The browser reports the collapse's clamp as a scroll.
		fireEvent.scroll(viewport());
		const before = await visibleRowsAfterLayout();
		await update([history, <ToolRow key="reply" defaultExpanded />, nextRow]);

		expect(await visibleRowsAfterLayout()).toEqual(before);
	});

	it("stays put when a removed row clamps the reader after a collapse", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([
			history,
			<ToolRow key="reply" defaultExpanded />,
			row("marker", 40),
		]);
		await user.click(toggle());
		await visibleRowsAfterLayout();

		await update([history, <ToolRow key="reply" defaultExpanded />]);
		// A removed row resizes the content but not the viewport.
		const content = screen.getByRole("log");
		MockResizeObserver.instances
			.find(({ observe }) => observe.mock.calls[0]?.[0] === content)
			?.simulateResize(0);
		await update([history, <ToolRow key="reply" defaultExpanded />, nextRow]);

		expect(await visibleRowsAfterLayout()).not.toContain("next");
	});

	it("resumes following in the next turn after a toggle", async () => {
		const user = userEvent.setup();
		const turn = [history, <ToolRow key="reply" />];
		const update = renderTranscript(turn);
		await user.click(toggle());

		await update([...turn, prompt, row("next-reply", 100)]);
		// The reply grows past the tail spacer but stays in the handoff band.
		await update([...turn, prompt, row("next-reply", 340)]);
		await visibleRowsAfterLayout();
		await update([...turn, prompt, row("next-reply", 340), nextRow]);

		expect(await visibleRowsAfterLayout()).toContain("next");
	});

	it("holds the anchored prompt in the first frame after a collapse", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([history]);
		await update([history, prompt, <ToolRow key="reply" defaultExpanded />]);
		const before = await visibleRowsAfterLayout();

		await user.click(toggle());
		// The browser reports the collapse's clamp as a scroll.
		fireEvent.scroll(viewport());
		// No resize report: the scroller's ResizeObservers defer their correction
		// to the next frame, after the browser has painted this one.
		await act(() => new Promise(requestAnimationFrame));
		await act(() => new Promise(requestAnimationFrame));

		expect(currentVisibleRows()).toEqual(before);
	});
});

describe("ToolCall.Scrollable", () => {
	// A 100px box over content that starts 300px tall.
	let scrollHeight = 300;
	const CLIENT_HEIGHT = 100;

	const renderLog = () =>
		render(
			<ToolCall.Scrollable ariaLabel="Build log" followEnd>
				<div>Log lines</div>
			</ToolCall.Scrollable>,
		);
	const log = () => screen.getByRole("region", { name: "Build log" });
	const growContent = (height: number) => {
		scrollHeight = height;
		for (const observer of MockResizeObserver.instances) {
			observer.simulateResize(0, height);
		}
	};

	beforeEach(() => {
		scrollHeight = 300;
		const scrollTops = new WeakMap<Element, number>();
		MockResizeObserver.reset();
		vi.stubGlobal("ResizeObserver", MockResizeObserver);
		vi.spyOn(Element.prototype, "clientHeight", "get").mockImplementation(
			function (this: Element) {
				return isViewport(this) ? CLIENT_HEIGHT : 0;
			},
		);
		vi.spyOn(Element.prototype, "scrollHeight", "get").mockImplementation(
			function (this: Element) {
				return isViewport(this) ? scrollHeight : 0;
			},
		);
		vi.spyOn(Element.prototype, "scrollTop", "get").mockImplementation(
			function (this: Element) {
				return scrollTops.get(this) ?? 0;
			},
		);
		vi.spyOn(Element.prototype, "scrollTop", "set").mockImplementation(
			function (this: Element, top: number) {
				const max = Math.max(0, scrollHeight - CLIENT_HEIGHT);
				scrollTops.set(this, Math.max(0, Math.min(top, max)));
			},
		);
	});

	afterEach(() => {
		vi.restoreAllMocks();
		vi.unstubAllGlobals();
	});

	it("follows new content while the reader is at the end", () => {
		renderLog();
		expect(log().scrollTop).toBe(200);

		growContent(400);

		expect(log().scrollTop).toBe(300);
	});

	it("keeps the reader's place after they scroll up", () => {
		renderLog();
		log().scrollTop = 40;

		growContent(400);

		expect(log().scrollTop).toBe(40);
	});
});
