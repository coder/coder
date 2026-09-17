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

// jsdom has no layout. Rows stack by their data-height in a 400px viewport
// that clamps scrollTop the way a browser does, with a 10px classic
// scrollbar past its 790px clientWidth.
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

const history = Array.from({ length: 10 }, (_, index) => (
	<MessageScroller.Item
		key={index}
		messageId={`history-${index}`}
		data-height={100}
	/>
));
const prompt = (
	<MessageScroller.Item
		key="prompt"
		messageId="prompt"
		scrollAnchor
		data-height={40}
	/>
);
const nextRow = (
	<MessageScroller.Item key="next" messageId="next" data-height={100} />
);

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

// Mirrors the agent chat: autoScroll, opening at the end.
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
const readVisibleRows = async () => {
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
		vi.spyOn(Element.prototype, "scrollTop", "get").mockImplementation(
			function (this: Element) {
				const stored = scrollTops.get(this) ?? 0;
				if (!isViewport(this)) {
					return stored;
				}
				const top = Math.min(stored, maxScrollTop(this));
				scrollTops.set(this, top);
				return top;
			},
		);
		vi.spyOn(Element.prototype, "scrollTop", "set").mockImplementation(
			function (this: Element, top: number) {
				scrollTops.set(this, Math.max(0, top));
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
		const before = await readVisibleRows();

		await user.click(toggle());
		await update([history, <ToolRow key="reply" />, nextRow]);

		expect(await readVisibleRows()).toEqual(before);
	});

	it("keeps the reader in place when a tool collapses while following the bottom", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([
			history,
			<ToolRow key="reply" defaultExpanded />,
		]);

		await user.click(toggle());
		// The browser reports the collapse's clamp as a scroll.
		fireEvent.scroll(viewport());
		const before = await readVisibleRows();
		await update([history, <ToolRow key="reply" defaultExpanded />, nextRow]);

		expect(await readVisibleRows()).toEqual(before);
	});

	it("holds the anchored prompt when Space collapses a tool", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([history]);
		await update([history, prompt, <ToolRow key="reply" />]);
		await user.click(toggle());
		const before = await readVisibleRows();

		await user.keyboard(" ");

		expect(await readVisibleRows()).toEqual(before);
	});

	it("holds the anchored prompt when a collapse ends near the live edge", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([history]);
		await update([history, prompt, <ToolRow key="reply" />]);
		await user.click(toggle());
		// The reply streams on, so the collapse leaves it ending 21px below the
		// view, inside the band where streaming hands off to following.
		const reply = <ToolRow key="reply" streamedHeight={277} />;
		await update([history, prompt, reply]);
		const before = await readVisibleRows();

		await user.click(toggle());
		await update([history, prompt, reply, nextRow]);

		expect(await readVisibleRows()).toEqual(before);
	});

	it("holds the anchored prompt when the content remounts after a toggle", async () => {
		const user = userEvent.setup();
		const update = renderTranscript([history]);
		const rows = [history, prompt, <ToolRow key="reply" />];
		await update(rows);
		await user.click(toggle());
		await user.click(toggle());
		const before = await readVisibleRows();

		await update(rows, "remounted");

		expect(await readVisibleRows()).toEqual(before);
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

		expect(await readVisibleRows()).toContain("next");
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
		const before = await readVisibleRows();
		await update([history, <ToolRow key="reply" defaultExpanded />, nextRow]);

		expect(await readVisibleRows()).toEqual(before);
	});
});
