import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { MockResizeObserver } from "#/testHelpers/resizeObserver";
import { MessageScroller } from "#/vendor/message-scroller";
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
	// The scroller only calls scrollTo's options form.
	vi.spyOn(Element.prototype, "scrollTo").mockImplementation(function (
		this: Element,
		options?: ScrollToOptions | number,
	) {
		if (typeof options === "object") {
			this.scrollTop = options.top ?? 0;
		}
	});
});

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
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

// Stacks the transcript rows at the given heights, otherRowHeight for rows not
// listed, in a viewport of viewportHeight, with the tail spacer below them.
const mockLayout = (
	viewportHeight: number,
	heights: Record<string, number>,
	otherRowHeight: number,
) => {
	const heightOf = (row: HTMLElement) =>
		heights[row.dataset.messageId ?? ""] ?? otherRowHeight;
	const rowTop = (row: HTMLElement) => {
		let top = 0;
		for (const other of document.querySelectorAll<HTMLElement>(
			"[data-message-id]",
		)) {
			if (other === row) {
				return top;
			}
			top += heightOf(other);
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
			const rows = this.querySelectorAll<HTMLElement>("[data-message-id]");
			const last = rows[rows.length - 1];
			const spacer = this.querySelector<HTMLElement>(
				"[data-message-scroller-spacer]",
			);
			const spacerHeight =
				spacer && !spacer.hidden ? Number.parseFloat(spacer.style.height) : 0;
			return (last ? rowTop(last) + heightOf(last) : 0) + spacerHeight;
		},
	);
	vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(
		function (this: Element) {
			if (!(this instanceof HTMLElement) || !this.dataset.messageId) {
				return new DOMRect();
			}
			const scrollTop =
				document.querySelector('[aria-label="Messages"]')?.scrollTop ?? 0;
			return new DOMRect(0, rowTop(this) - scrollTop, 0, heightOf(this));
		},
	);
};

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

// A browser reports a resize after each change to the transcript's height.
const resize = async () => {
	act(() => {
		for (const observer of MockResizeObserver.instances) {
			observer.simulateResize(0);
		}
	});
	await nextFrames();
};

// Sends a message from a waiting chat and lets the stream store it and start
// a long reply, as a turn does since the pending row anchors the scroller.
const startTurnWithLongReply = async (
	beforeStreamStoresSend?: (viewport: HTMLElement) => void,
) => {
	// The pending row has an ID chosen at the click, so it takes the 40px for
	// rows not listed.
	mockLayout(
		400,
		{
			"message:1": 300,
			"message:2": 300,
			"live-assistant": 600,
			"message:4": 600,
		},
		40,
	);
	MockResizeObserver.reset();
	vi.stubGlobal("ResizeObserver", MockResizeObserver);
	const store = createChatStore();
	store.upsertDurableMessages([
		mockMessage(1, "user", "First question"),
		mockMessage(2, "assistant", "First answer"),
	]);
	store.applyServerChatStatus("waiting");
	renderWithAuth(
		<MessageScroller.Provider autoScroll defaultScrollPosition="end">
			<ChatPageTimeline
				organizationId="organization-id"
				store={store}
				persistedError={undefined}
				hasMoreMessages={false}
				isFetchingMoreMessages={false}
				hasFetchMoreError={false}
				onFetchMoreMessages={() => {}}
				urlTransform={(url) => url}
				mcpServers={[]}
			/>
		</MessageScroller.Provider>,
	);
	const viewport = await screen.findByLabelText("Messages");
	await nextFrames();

	await act(async () => {
		void store.runPendingAction(
			{
				kind: "send",
				chatID: MockChatMessage.chat_id,
				content: [{ type: "text", text: "Second question" }],
				busyBehavior: "queue",
			},
			() => Promise.resolve(),
		);
	});
	await nextFrames();
	const heldScrollTop = viewport.scrollTop;
	beforeStreamStoresSend?.(viewport);

	// One stream frame stores the sent message, starts the turn and streams
	// the first part of the reply.
	act(() => {
		store.batch(() => {
			store.upsertDurableMessages([mockMessage(3, "user", "Second question")]);
			store.applyServerChatStatus("running");
			store.applyMessagePart({ type: "text", text: "A long answer" });
		});
	});
	await resize();
	act(() => {
		store.batch(() => {
			store.upsertDurableMessages([
				mockMessage(4, "assistant", "A long answer"),
			]);
			store.applyServerChatStatus("waiting");
		});
	});
	await resize();
	return { viewport, heldScrollTop };
};

it("holds a sent message at the top through a long reply after the stream replaces its pending row", async () => {
	const { viewport, heldScrollTop } = await startTurnWithLongReply();

	expect(viewport.scrollTop).toBe(heldScrollTop);
	expect(
		viewport.querySelector<HTMLElement>("[data-message-scroller-spacer]")
			?.hidden,
	).toBe(true);
});

it("leaves the view where the reader scrolled before the stream replaces the pending row", async () => {
	let scrolledTop = 0;
	const { viewport } = await startTurnWithLongReply((viewport) => {
		fireEvent.wheel(viewport, { deltaY: -400 });
		viewport.scrollTop -= 400;
		fireEvent.scroll(viewport);
		scrolledTop = viewport.scrollTop;
	});

	expect(viewport.scrollTop).toBe(scrolledTop);
});
