import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { MessageScroller } from "#/vendor/message-scroller";
import { ChatMessageScroller } from "./ChatMessageScroller";

// jsdom does no layout, so give the messages viewport a height and content
// that overflows it.
const isViewport = (element: Element) =>
	element.getAttribute("aria-label") === "Messages";

beforeEach(() => {
	vi.spyOn(Element.prototype, "clientHeight", "get").mockImplementation(
		function (this: Element) {
			return isViewport(this) ? 100 : 0;
		},
	);
	vi.spyOn(Element.prototype, "scrollHeight", "get").mockImplementation(
		function (this: Element) {
			return isViewport(this) ? 1000 : 0;
		},
	);
	// jsdom runs a frame's callbacks in one task, so EarlierMessages' older-page
	// request fires before the re-render that cancels it in a browser.
	vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) =>
		window.setTimeout(() => callback(performance.now())),
	);
	vi.stubGlobal("cancelAnimationFrame", (handle: number) =>
		window.clearTimeout(handle),
	);
	// jsdom has no Element.scrollTo. The scroller only calls its options form.
	Element.prototype.scrollTo = function (
		this: Element,
		options?: ScrollToOptions | number,
	) {
		if (typeof options === "object") {
			this.scrollTop = options.top ?? 0;
		}
	};
});

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
	Reflect.deleteProperty(Element.prototype, "scrollTo");
});

const nextFrames = () =>
	act(
		() =>
			new Promise<void>((resolve) => {
				requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
			}),
	);

// Without the viewportRef guard only autoScroll: false fails, because with
// autoScroll the scroller follows the end once the viewport attaches.
it.each([true, false])(
	"opens at the end without loading older messages when a transcript mounts after its scroller provider (autoScroll: %s)",
	async (autoScroll) => {
		const onFetchMoreMessages = vi.fn();
		const page = (transcript?: React.ReactNode) => (
			<MessageScroller.Provider
				autoScroll={autoScroll}
				defaultScrollPosition="end"
			>
				{transcript}
			</MessageScroller.Provider>
		);

		// The provider mounts while the page shows its loading view.
		const { rerender, getByLabelText } = render(page());
		rerender(
			page(
				<ChatMessageScroller
					hasMoreMessages
					isFetchingMoreMessages={false}
					isHydratingMessages={false}
					hasFetchMoreError={false}
					hasTranscriptRows
					onFetchMoreMessages={onFetchMoreMessages}
				>
					{["1", "2", "3"].map((id) => (
						<MessageScroller.Item key={id} messageId={id}>
							message {id}
						</MessageScroller.Item>
					))}
				</ChatMessageScroller>,
			),
		);
		await nextFrames();

		const viewport = getByLabelText("Messages");
		expect(viewport.scrollTop).toBe(
			viewport.scrollHeight - viewport.clientHeight,
		);
		expect(onFetchMoreMessages).not.toHaveBeenCalled();
	},
);

// Stacks the rows at the given heights in a viewport of viewportHeight, with
// the tail spacer below them.
const mockLayout = (
	viewportHeight: number,
	heights: Record<string, number>,
) => {
	const rowTop = (row: HTMLElement) => {
		let top = 0;
		for (const other of document.querySelectorAll<HTMLElement>(
			"[data-message-id]",
		)) {
			if (other === row) {
				return top;
			}
			top += heights[other.dataset.messageId ?? ""] ?? 0;
		}
		return top;
	};
	vi.spyOn(Element.prototype, "clientHeight", "get").mockImplementation(
		function (this: Element) {
			return isViewport(this) ? viewportHeight : 0;
		},
	);
	vi.spyOn(Element.prototype, "scrollHeight", "get").mockImplementation(
		function (this: Element) {
			if (!isViewport(this)) {
				return 0;
			}
			const spacer = this.querySelector<HTMLElement>(
				"[data-message-scroller-spacer]",
			);
			const rows = this.querySelectorAll<HTMLElement>("[data-message-id]");
			const last = rows[rows.length - 1];
			const rowsBottom = last
				? rowTop(last) + (heights[last.dataset.messageId ?? ""] ?? 0)
				: 0;
			const spacerHeight =
				spacer && !spacer.hidden ? Number.parseFloat(spacer.style.height) : 0;
			return rowsBottom + spacerHeight;
		},
	);
	vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(
		function (this: Element) {
			if (!(this instanceof HTMLElement) || !this.dataset.messageId) {
				return new DOMRect();
			}
			const scrollTop =
				document.querySelector('[aria-label="Messages"]')?.scrollTop ?? 0;
			return new DOMRect(
				0,
				rowTop(this) - scrollTop,
				0,
				heights[this.dataset.messageId] ?? 0,
			);
		},
	);
};

type Row = { id: string; scrollAnchor?: boolean };

const transcript = (rows: Row[], isTurnActive: boolean) => (
	<MessageScroller.Provider autoScroll defaultScrollPosition="end">
		<ChatMessageScroller
			hasMoreMessages={false}
			isFetchingMoreMessages={false}
			isHydratingMessages={false}
			hasFetchMoreError={false}
			hasTranscriptRows
			onFetchMoreMessages={vi.fn()}
			isTurnActive={isTurnActive}
		>
			{rows.map((row) => (
				<MessageScroller.Item
					key={row.id}
					messageId={row.id}
					scrollAnchor={row.scrollAnchor}
				/>
			))}
		</ChatMessageScroller>
	</MessageScroller.Provider>
);

const history: Row[] = [{ id: "1" }, { id: "2" }, { id: "3" }];
const sent: Row = { id: "sent", scrollAnchor: true };
// The timeline stops marking the sent message as an anchor once the turn has
// no live reply.
const answered: Row = { id: "sent" };

it("moves a short reply to the end of the view when its turn ends", async () => {
	mockLayout(400, { 1: 200, 2: 200, 3: 200, sent: 40, reply: 60 });
	const { rerender, getByLabelText } = render(transcript(history, false));
	await nextFrames();

	rerender(transcript([...history, sent], true));
	await nextFrames();
	rerender(transcript([...history, sent, { id: "reply" }], true));
	await nextFrames();
	const viewport = getByLabelText("Messages");
	const spacer = viewport.querySelector<HTMLElement>(
		"[data-message-scroller-spacer]",
	);
	// The scroller holds the sent message at the top above blank space.
	expect(spacer?.hidden).toBe(false);

	rerender(transcript([...history, answered, { id: "reply" }], false));
	await nextFrames();

	expect(spacer?.hidden).toBe(true);
	expect(viewport.scrollTop).toBe(
		viewport.scrollHeight - viewport.clientHeight,
	);
});

it("keeps the sent message in view when a long reply lands just before its turn ends", async () => {
	mockLayout(400, { 1: 200, 2: 200, 3: 200, sent: 40, reply: 600 });
	const { rerender, getByLabelText } = render(transcript(history, false));
	await nextFrames();

	rerender(transcript([...history, sent], true));
	await nextFrames();
	const viewport = getByLabelText("Messages");
	const heldScrollTop = viewport.scrollTop;

	// The reply lands and the turn ends before a resize sizes the spacer for
	// the reply.
	rerender(transcript([...history, sent, { id: "reply" }], true));
	rerender(transcript([...history, answered, { id: "reply" }], false));
	await nextFrames();

	expect(viewport.scrollTop).toBe(heldScrollTop);
	expect(
		viewport.querySelector<HTMLElement>("[data-message-scroller-spacer]")
			?.hidden,
	).toBe(true);
});
