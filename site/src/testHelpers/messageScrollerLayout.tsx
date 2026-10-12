import { act, render, screen } from "@testing-library/react";
import { afterAll, afterEach, beforeAll, beforeEach, vi } from "vitest";
import { MockResizeObserver } from "#/testHelpers/resizeObserver";
import {
	MessageScroller,
	useMessageScrollerVisibility,
} from "#/vendor/message-scroller";

// jsdom has no layout. Rows stack by their data-height in a fixed viewport
// that clamps scrollTop the way a browser does, with a classic scrollbar past
// its clientWidth.
const VIEWPORT_HEIGHT = 400;
export const VIEWPORT_WIDTH = 800;
export const SCROLLBAR_WIDTH = 10;

export const isViewport = (element: Element | null) =>
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

export const row = (id: string, height: number, scrollAnchor = false) => (
	<MessageScroller.Item
		key={id}
		messageId={id}
		scrollAnchor={scrollAnchor}
		data-height={height}
	/>
);
export const history = Array.from({ length: 10 }, (_, index) =>
	row(`history-${index}`, 100),
);
export const prompt = row("prompt", 40, true);
export const nextRow = row("next", 100);

let visibleRows: readonly string[] = [];

const VisibleRows: React.FC = () => {
	visibleRows = useMessageScrollerVisibility().visibleMessageIds;
	return null;
};

export const currentVisibleRows = () => visibleRows;

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

export const viewport = () => screen.getByRole("region", { name: "Messages" });

// The scroller sees row changes through a MutationObserver, which runs as a
// microtask after React commits.
export const renderTranscript = (children: React.ReactNode) => {
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
export const flushLayout = async () => {
	for (const observer of MockResizeObserver.instances) {
		observer.simulateResize(0);
	}
	await act(() => new Promise(requestAnimationFrame));
	await act(() => new Promise(requestAnimationFrame));
};

export const visibleRowsAfterLayout = async () => {
	await flushLayout();
	return visibleRows;
};

/**
 * Installs the layout mocks for the enclosing describe block. Call it at the
 * top of a describe that renders a transcript with renderTranscript.
 */
export const setupMessageScrollerLayout = () => {
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
		// jsdom runs requestAnimationFrame on the global setInterval. A frame
		// requested under fake timers, as motion does, leaves that loop on
		// the fake clock and stalls every later frame in the same file.
		vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) =>
			setTimeout(() => callback(performance.now()), 16),
		);
		vi.stubGlobal("cancelAnimationFrame", clearTimeout);
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
};
