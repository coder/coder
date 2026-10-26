import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { MockResizeObserver } from "#/testHelpers/resizeObserver";
import {
	MessageScroller,
	useMessageScroller,
	useMessageScrollerUserLayoutIntent,
} from "#/vendor/message-scroller";
import { createChatStore } from "./ChatConversation/chatStore";
import { ChatMessageScroller } from "./ChatMessageScroller";
import { ChatPageTimeline } from "./ChatPageContent";

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
					isTurnActive={false}
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
// the tail spacer below them. The layout never resizes on its own, and the
// unit setup's ResizeObserver polyfill calls back after DOM changes, so a
// controllable one replaces it.
const mockLayout = (
	viewportHeight: number,
	heights: Record<string, number>,
) => {
	MockResizeObserver.reset();
	vi.stubGlobal("ResizeObserver", MockResizeObserver);
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

type Row = { id: string; scrollAnchor?: boolean; children?: React.ReactNode };

const transcript = (
	rows: Row[],
	{
		isTurnActive,
		autoScroll = true,
	}: { isTurnActive: boolean; autoScroll?: boolean },
) => (
	<MessageScroller.Provider autoScroll={autoScroll} defaultScrollPosition="end">
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
				>
					{row.children}
				</MessageScroller.Item>
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
	const { rerender, getByLabelText } = render(
		transcript(history, { isTurnActive: false }),
	);
	await nextFrames();

	rerender(transcript([...history, sent], { isTurnActive: true }));
	await nextFrames();
	rerender(
		transcript([...history, sent, { id: "reply" }], { isTurnActive: true }),
	);
	await nextFrames();
	const viewport = getByLabelText("Messages");
	const spacer = viewport.querySelector<HTMLElement>(
		"[data-message-scroller-spacer]",
	);
	// The scroller holds the sent message at the top above blank space.
	expect(spacer?.hidden).toBe(false);

	rerender(
		transcript([...history, answered, { id: "reply" }], {
			isTurnActive: false,
		}),
	);
	await nextFrames();

	expect(spacer?.hidden).toBe(true);
	expect(viewport.scrollTop).toBe(
		viewport.scrollHeight - viewport.clientHeight,
	);
});

it("keeps the sent message in view when a long reply lands just before its turn ends", async () => {
	mockLayout(400, { 1: 200, 2: 200, 3: 200, sent: 40, reply: 600 });
	const { rerender, getByLabelText } = render(
		transcript(history, { isTurnActive: false }),
	);
	await nextFrames();

	rerender(transcript([...history, sent], { isTurnActive: true }));
	await nextFrames();
	const viewport = getByLabelText("Messages");
	const heldScrollTop = viewport.scrollTop;

	// The reply lands and the turn ends before a resize sizes the spacer for
	// the reply.
	rerender(
		transcript([...history, sent, { id: "reply" }], { isTurnActive: true }),
	);
	rerender(
		transcript([...history, answered, { id: "reply" }], {
			isTurnActive: false,
		}),
	);
	await nextFrames();

	expect(viewport.scrollTop).toBe(heldScrollTop);
	expect(
		viewport.querySelector<HTMLElement>("[data-message-scroller-spacer]")
			?.hidden,
	).toBe(true);
});

it("hands a turn off to following the end when its reply lands just before the turn ends", async () => {
	mockLayout(400, { 1: 200, 2: 200, 3: 200, sent: 40, reply: 60, more: 270 });
	const { rerender, getByLabelText } = render(
		transcript(history, { isTurnActive: false }),
	);
	await nextFrames();

	rerender(
		transcript([...history, sent, { id: "reply" }], { isTurnActive: true }),
	);
	await nextFrames();

	// The reply grows 34 px past the view and the turn ends before a resize.
	rerender(
		transcript([...history, sent, { id: "reply" }, { id: "more" }], {
			isTurnActive: true,
		}),
	);
	rerender(
		transcript([...history, answered, { id: "reply" }, { id: "more" }], {
			isTurnActive: false,
		}),
	);
	await nextFrames();

	const viewport = getByLabelText("Messages");
	expect(viewport.scrollTop).toBe(
		viewport.scrollHeight - viewport.clientHeight,
	);
});

it("drops the blank space without moving the view when the reader scrolled during the turn", async () => {
	mockLayout(400, { 1: 200, 2: 200, 3: 200, sent: 40, reply: 60 });
	const { rerender, getByLabelText } = render(
		transcript(history, { isTurnActive: false }),
	);
	await nextFrames();

	rerender(transcript([...history, sent], { isTurnActive: true }));
	await nextFrames();
	const viewport = getByLabelText("Messages");
	fireEvent.wheel(viewport, { deltaY: -400 });
	viewport.scrollTop -= 400;
	fireEvent.scroll(viewport);
	const scrolledTop = viewport.scrollTop;
	rerender(
		transcript([...history, sent, { id: "reply" }], { isTurnActive: true }),
	);
	await nextFrames();

	rerender(
		transcript([...history, answered, { id: "reply" }], {
			isTurnActive: false,
		}),
	);
	await nextFrames();

	expect(viewport.scrollTop).toBe(scrolledTop);
	expect(
		viewport.querySelector<HTMLElement>("[data-message-scroller-spacer]")
			?.hidden,
	).toBe(true);
});

it.each([
	{
		gesture: "a wheel down",
		scroll: (viewport: HTMLElement) => {
			// The view is already at the end, so the wheel scrolls nothing.
			fireEvent.wheel(viewport, { deltaY: 100 });
		},
	},
	{
		gesture: "a small wheel up",
		scroll: (viewport: HTMLElement) => {
			fireEvent.wheel(viewport, { deltaY: -50 });
			viewport.scrollTop -= 50;
			fireEvent.scroll(viewport);
		},
	},
])(
	"moves a short reply to the end of the view after $gesture at the held position",
	async ({ scroll }) => {
		mockLayout(400, { 1: 200, 2: 200, 3: 200, sent: 40, reply: 60 });
		const { rerender, getByLabelText } = render(
			transcript(history, { isTurnActive: false }),
		);
		await nextFrames();

		rerender(
			transcript([...history, sent, { id: "reply" }], { isTurnActive: true }),
		);
		await nextFrames();
		const viewport = getByLabelText("Messages");
		scroll(viewport);
		await nextFrames();

		rerender(
			transcript([...history, answered, { id: "reply" }], {
				isTurnActive: false,
			}),
		);
		await nextFrames();

		expect(
			viewport.querySelector<HTMLElement>("[data-message-scroller-spacer]")
				?.hidden,
		).toBe(true);
		expect(viewport.scrollTop).toBe(
			viewport.scrollHeight - viewport.clientHeight,
		);
	},
);

it("lets the reader scroll up from the held position during the turn", async () => {
	mockLayout(400, { 1: 200, 2: 200, 3: 200, sent: 40, reply: 60 });
	const { rerender, getByLabelText } = render(
		transcript(history, { isTurnActive: false }),
	);
	await nextFrames();

	rerender(
		transcript([...history, sent, { id: "reply" }], { isTurnActive: true }),
	);
	await nextFrames();
	const viewport = getByLabelText("Messages");
	const spacer = viewport.querySelector<HTMLElement>(
		"[data-message-scroller-spacer]",
	);
	const spacerHeight = spacer?.style.height;
	fireEvent.wheel(viewport, { deltaY: -50 });
	viewport.scrollTop -= 50;
	fireEvent.scroll(viewport);
	const scrolledTop = viewport.scrollTop;
	// A browser reports a resize when the spacer height changes.
	if (spacer?.style.height !== spacerHeight) {
		for (const observer of MockResizeObserver.instances) {
			observer.simulateResize(0);
		}
	}
	await nextFrames();

	expect(viewport.scrollTop).toBe(scrolledTop);
});

it("keeps the sent message in view when a long reply lands after a wheel down at the held position", async () => {
	mockLayout(400, { 1: 200, 2: 200, 3: 200, sent: 40, reply: 600 });
	const { rerender, getByLabelText } = render(
		transcript(history, { isTurnActive: false }),
	);
	await nextFrames();

	rerender(transcript([...history, sent], { isTurnActive: true }));
	await nextFrames();
	const viewport = getByLabelText("Messages");
	const heldScrollTop = viewport.scrollTop;
	fireEvent.wheel(viewport, { deltaY: 100 });
	rerender(
		transcript([...history, sent, { id: "reply" }], { isTurnActive: true }),
	);
	await nextFrames();

	rerender(
		transcript([...history, answered, { id: "reply" }], {
			isTurnActive: false,
		}),
	);
	await nextFrames();

	expect(viewport.scrollTop).toBe(heldScrollTop);
});

it("keeps the sent message in place when the turn ends without autoScroll", async () => {
	mockLayout(400, { 1: 200, 2: 200, 3: 200, sent: 40, reply: 60 });
	const { rerender, getByLabelText } = render(
		transcript(history, { isTurnActive: false, autoScroll: false }),
	);
	await nextFrames();

	rerender(
		transcript([...history, sent, { id: "reply" }], {
			isTurnActive: true,
			autoScroll: false,
		}),
	);
	await nextFrames();
	const viewport = getByLabelText("Messages");
	const heldScrollTop = viewport.scrollTop;

	rerender(
		transcript([...history, answered, { id: "reply" }], {
			isTurnActive: false,
			autoScroll: false,
		}),
	);
	await nextFrames();

	expect(viewport.scrollTop).toBe(heldScrollTop);
});

const LayoutToggle: React.FC = () => {
	const userLayoutIntent = useMessageScrollerUserLayoutIntent();
	return (
		<button
			type="button"
			onClick={(event) => userLayoutIntent(event.currentTarget)}
		>
			Toggle
		</button>
	);
};

it("keeps the sent message in view when the reader toggled the reply during the turn", async () => {
	mockLayout(400, { 1: 200, 2: 200, 3: 200, sent: 40, reply: 60 });
	const { rerender, getByLabelText, getByRole } = render(
		transcript(history, { isTurnActive: false }),
	);
	await nextFrames();

	const reply: Row = { id: "reply", children: <LayoutToggle /> };
	rerender(transcript([...history, sent, reply], { isTurnActive: true }));
	await nextFrames();
	const viewport = getByLabelText("Messages");
	const heldScrollTop = viewport.scrollTop;
	fireEvent.click(getByRole("button", { name: "Toggle" }));
	await nextFrames();

	rerender(transcript([...history, answered, reply], { isTurnActive: false }));
	await nextFrames();

	expect(viewport.scrollTop).toBe(heldScrollTop);
	const spacer = viewport.querySelector<HTMLElement>(
		"[data-message-scroller-spacer]",
	);
	// The blank space stays until the reader scrolls it out of view.
	expect(spacer?.hidden).toBe(false);

	fireEvent.wheel(viewport, { deltaY: -300 });
	viewport.scrollTop -= 300;
	fireEvent.scroll(viewport);

	expect(spacer?.hidden).toBe(true);
	expect(viewport.scrollTop).toBe(heldScrollTop - 300);
});

it("keeps the view in place when the reader toggled a row above the sent message during the turn", async () => {
	mockLayout(400, { 1: 200, 2: 200, 3: 200, sent: 40, reply: 60 });
	const rows: Row[] = [
		{ id: "1" },
		{ id: "2" },
		{ id: "3", children: <LayoutToggle /> },
	];
	const { rerender, getByLabelText, getByRole } = render(
		transcript(rows, { isTurnActive: false }),
	);
	await nextFrames();

	rerender(
		transcript([...rows, sent, { id: "reply" }], { isTurnActive: true }),
	);
	await nextFrames();
	const viewport = getByLabelText("Messages");
	const heldScrollTop = viewport.scrollTop;
	fireEvent.click(getByRole("button", { name: "Toggle" }));
	await nextFrames();

	rerender(
		transcript([...rows, answered, { id: "reply" }], { isTurnActive: false }),
	);
	await nextFrames();

	expect(viewport.scrollTop).toBe(heldScrollTop);
	expect(viewport.scrollTop).toBeLessThanOrEqual(
		viewport.scrollHeight - viewport.clientHeight,
	);
});

const JumpButton: React.FC = () => {
	const { scrollToMessage } = useMessageScroller();
	return (
		<button
			type="button"
			onClick={() => scrollToMessage("3", { behavior: "smooth" })}
		>
			Jump
		</button>
	);
};

it("keeps the spacer a jump in flight needs while it scrolls and when the turn ends", async () => {
	mockLayout(400, { 1: 200, 2: 200, 3: 200 });
	const row3Top = 200 + 200;
	// The smooth jump stays in flight: the view does not move.
	Element.prototype.scrollTo = function (
		this: Element,
		options?: ScrollToOptions | number,
	) {
		if (typeof options === "object" && options.behavior !== "smooth") {
			this.scrollTop = options.top ?? 0;
		}
	};
	const rows: Row[] = [
		{ id: "1", children: <JumpButton /> },
		{ id: "2" },
		{ id: "3" },
	];
	const { rerender, getByLabelText, getByRole } = render(
		transcript(rows, { isTurnActive: true }),
	);
	await nextFrames();
	const viewport = getByLabelText("Messages");
	fireEvent.wheel(viewport, { deltaY: -200 });
	viewport.scrollTop = 0;
	fireEvent.scroll(viewport);

	// Message 3 can reach the top of the view only above a spacer.
	fireEvent.click(getByRole("button", { name: "Jump" }));
	viewport.scrollTop = 100;
	fireEvent.scroll(viewport);
	expect(viewport.scrollHeight - viewport.clientHeight).toBe(row3Top);

	rerender(transcript(rows, { isTurnActive: false }));
	await nextFrames();

	expect(viewport.scrollHeight - viewport.clientHeight).toBe(row3Top);
});

const mockMessage = (
	id: number,
	role: TypesGen.ChatMessageRole,
	text: string,
): TypesGen.ChatMessage => ({
	...MockChatMessage,
	id,
	role,
	content: [{ type: "text", text }],
});

it("moves a short reply to the end of the view when the chat stops running", async () => {
	mockLayout(400, {
		"message:1": 300,
		"message:2": 300,
		"message:3": 40,
		"message:4": 60,
	});
	const store = createChatStore();
	store.replaceMessages([
		mockMessage(1, "user", "First question"),
		mockMessage(2, "assistant", "First answer"),
	]);
	store.setChatStatus("waiting");
	renderWithAuth(
		<MessageScroller.Provider autoScroll defaultScrollPosition="end">
			<ChatPageTimeline
				organizationId="organization-id"
				store={store}
				persistedError={undefined}
				hasMoreMessages={false}
				isFetchingMoreMessages={false}
				isHydratingMessages={false}
				hasFetchMoreError={false}
				onFetchMoreMessages={async () => {}}
				urlTransform={(url) => url}
				mcpServers={[]}
			/>
		</MessageScroller.Provider>,
	);
	const viewport = await screen.findByLabelText("Messages");
	await nextFrames();

	await act(async () => {
		store.upsertDurableMessages([mockMessage(3, "user", "Second question")]);
		store.setChatStatus("running");
	});
	await nextFrames();
	await act(async () => {
		store.upsertDurableMessages([mockMessage(4, "assistant", "Short answer")]);
	});
	await nextFrames();
	const spacer = viewport.querySelector<HTMLElement>(
		"[data-message-scroller-spacer]",
	);
	// The scroller holds the second question at the top above blank space.
	expect(spacer?.hidden).toBe(false);

	await act(async () => {
		store.setChatStatus("waiting");
	});
	await nextFrames();

	expect(spacer?.hidden).toBe(true);
	expect(viewport.scrollTop).toBe(
		viewport.scrollHeight - viewport.clientHeight,
	);
});
