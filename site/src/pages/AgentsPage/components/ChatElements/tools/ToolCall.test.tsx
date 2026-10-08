import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import {
	afterAll,
	afterEach,
	beforeAll,
	beforeEach,
	describe,
	expect,
	it,
	vi,
} from "vitest";
import { MockResizeObserver } from "#/testHelpers/resizeObserver";
import {
	MessageScroller,
	useMessageScrollerVisibility,
} from "#/vendor/message-scroller";
import { ToolCall } from "./ToolCall";

// jsdom has no layout. Rows stack by their data-height in a fixed viewport
// that clamps scrollTop the way a browser does, with a classic scrollbar past
// its clientWidth.
const VIEWPORT_HEIGHT = 400;
const VIEWPORT_WIDTH = 800;
const SCROLLBAR_WIDTH = 10;

const isViewport = (element: Element | null) =>
	element?.getAttribute("role") === "region";

const heightOf = (element: Element) =>
	element instanceof HTMLElement && !element.hidden
		? Number.parseFloat(element.dataset.height ?? element.style.height) || 0
		: 0;

const contentHeight = (viewport: Element) =>
	Array.from(viewport.firstElementChild?.children ?? []).reduce(
		(sum, row) => sum + heightOf(row),
		0,
	);

const maxScrollTop = (viewport: Element) =>
	Math.max(0, contentHeight(viewport) - VIEWPORT_HEIGHT);

const row = (id: string, height: number, scrollAnchor = false) => (
	<MessageScroller.Item
		key={id}
		messageId={id}
		scrollAnchor={scrollAnchor}
		data-height={height}
	/>
);
const history = Array.from({ length: 10 }, (_, index) =>
	row(`history-${index}`, 100),
);
const prompt = row("prompt", 40, true);
const nextRow = row("next", 100);

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

let visibleRows: readonly string[] = [];

const VisibleRows: React.FC = () => {
	visibleRows = useMessageScrollerVisibility().visibleMessageIds;
	return null;
};

// Same Provider props as AgentChatPage.
const Transcript: React.FC<{
	children: React.ReactNode;
	contentKey: string;
}> = ({ children, contentKey }) => (
	<MessageScroller.Provider autoScroll defaultScrollPosition="end">
		<VisibleRows />
		<MessageScroller.Root>
			<MessageScroller.Viewport>
				<MessageScroller.Content key={contentKey}>
					{children}
				</MessageScroller.Content>
			</MessageScroller.Viewport>
		</MessageScroller.Root>
	</MessageScroller.Provider>
);

const viewport = () => screen.getByRole("region", { name: "Messages" });
const toggle = () => screen.getByRole("button", { name: "Read config.yaml" });

// The scroller sees row changes through a MutationObserver, which runs as a
// microtask after React commits.
const renderTranscript = (children: React.ReactNode) => {
	const { rerender } = render(
		<Transcript contentKey="content">{children}</Transcript>,
	);
	return (next: React.ReactNode, contentKey = "content") =>
		act(async () =>
			rerender(<Transcript contentKey={contentKey}>{next}</Transcript>),
		);
};

// A browser reports layout changes to the scroller's ResizeObservers, and the
// scroller publishes the rows the reader sees on a later animation frame.
const visibleRowsAfterLayout = async () => {
	for (const observer of MockResizeObserver.instances) {
		observer.simulateResize(0);
	}
	await act(() => new Promise(requestAnimationFrame));
	await act(() => new Promise(requestAnimationFrame));
	return visibleRows;
};

describe("ToolCall in a MessageScroller", () => {
	beforeAll(() => {
		// jsdom does not implement Element.scrollTo.
		Element.prototype.scrollTo = function (
			this: Element,
			options?: ScrollToOptions | number,
			y?: number,
		) {
			const top = typeof options === "number" ? y : options?.top;
			if (top !== undefined) {
				this.scrollTop = top;
			}
		};
	});

	afterAll(() => {
		Reflect.deleteProperty(Element.prototype, "scrollTo");
	});

	beforeEach(() => {
		const scrollTops = new WeakMap<Element, number>();
		MockResizeObserver.reset();
		vi.stubGlobal("ResizeObserver", MockResizeObserver);
		vi.spyOn(Element.prototype, "clientHeight", "get").mockImplementation(
			function (this: Element) {
				return isViewport(this) ? VIEWPORT_HEIGHT : 0;
			},
		);
		vi.spyOn(Element.prototype, "clientWidth", "get").mockImplementation(
			function (this: Element) {
				return isViewport(this) ? VIEWPORT_WIDTH - SCROLLBAR_WIDTH : 0;
			},
		);
		vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockImplementation(
			function (this: HTMLElement) {
				return isViewport(this) ? VIEWPORT_WIDTH : 0;
			},
		);
		vi.spyOn(Element.prototype, "scrollHeight", "get").mockImplementation(
			function (this: Element) {
				return isViewport(this)
					? Math.max(contentHeight(this), VIEWPORT_HEIGHT)
					: 0;
			},
		);
		// A shrink clamps only what reads return. Until the next write, growth
		// scrolls the view back down, as Chromium's scroll anchoring does.
		vi.spyOn(Element.prototype, "scrollTop", "get").mockImplementation(
			function (this: Element) {
				const stored = scrollTops.get(this) ?? 0;
				if (!isViewport(this)) {
					return stored;
				}
				return Math.min(stored, maxScrollTop(this));
			},
		);
		vi.spyOn(Element.prototype, "scrollTop", "set").mockImplementation(
			function (this: Element, top: number) {
				const max = isViewport(this) ? maxScrollTop(this) : top;
				scrollTops.set(this, Math.max(0, Math.min(top, max)));
			},
		);
		vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(
			function (this: Element) {
				if (isViewport(this)) {
					return new DOMRect(0, 0, VIEWPORT_WIDTH, VIEWPORT_HEIGHT);
				}
				const content = this.parentElement;
				const region = content?.parentElement ?? null;
				if (!content || !region || !isViewport(region)) {
					return new DOMRect();
				}
				const rows = Array.from(content.children);
				const top = rows
					.slice(0, rows.indexOf(this))
					.reduce((sum, row) => sum + heightOf(row), 0);
				return new DOMRect(0, top - region.scrollTop, 0, heightOf(this));
			},
		);
	});

	afterEach(() => {
		vi.restoreAllMocks();
		vi.unstubAllGlobals();
	});

	it("keeps the reader in place when a tool expands while following the bottom", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([history, <ToolRow key="reply" />]);
		const before = await visibleRowsAfterLayout();

		await user.click(toggle());
		await update([history, <ToolRow key="reply" />, nextRow]);

		expect(await visibleRowsAfterLayout()).toEqual(before);
	});

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

	it("holds the anchored prompt when output lands far past the live edge", async () => {
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

		expect(visibleRows).toEqual(before);
	});
});
