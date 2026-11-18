import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import {
	flushLayout,
	setupMessageScrollerLayout,
	viewport,
} from "#/testHelpers/messageScrollerLayout";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
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

const mockFirstQuestion: TypesGen.ChatMessage = {
	...MockChatMessage,
	id: 1,
	content: [{ type: "text", text: "First question" }],
};
const mockFirstAnswer: TypesGen.ChatMessage = {
	...MockChatMessage,
	id: 2,
	role: "assistant",
	content: [{ type: "text", text: "First answer" }],
};
const sentContent: TypesGen.ChatMessagePart[] = [
	{ type: "text", text: "Second question" },
];
const mockSentMessage: TypesGen.ChatMessage = {
	...MockChatMessage,
	id: 3,
	content: sentContent,
};
const mockLongReply: TypesGen.ChatMessage = {
	...MockChatMessage,
	id: 4,
	role: "assistant",
	content: [{ type: "text", text: "A long answer" }],
};

const rowHeights: Record<string, number> = {
	"message:1": 300,
	"message:2": 300,
	"live-assistant": 600,
	"message:4": 600,
};
// A pending row's ID is chosen at the click, so rows not listed are the sent
// message, as a pending row and once stored.
const sentRowHeight = 40;

describe("ChatMessageScroller hold on a sent message", () => {
	setupMessageScrollerLayout({
		rowHeight: (messageId) => rowHeights[messageId] ?? sentRowHeight,
	});

	const renderWaitingChat = async () => {
		const store = createChatStore();
		store.upsertDurableMessages([mockFirstQuestion, mockFirstAnswer]);
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
		await screen.findByRole("region", { name: "Messages" });
		await flushLayout();
		return store;
	};

	const send = (
		store: ReturnType<typeof createChatStore>,
		request: () => Promise<unknown>,
	) =>
		store.runPendingAction(
			{
				kind: "send",
				chatID: MockChatMessage.chat_id,
				content: sentContent,
				busyBehavior: "queue",
			},
			request,
		);

	// Sends from a waiting chat, then lets the stream store the message,
	// stream a long reply and end the turn.
	const runTurnWithLongReply = async (beforeStreamStoresSend?: () => void) => {
		const store = await renderWaitingChat();
		await act(async () => {
			void send(store, () => Promise.resolve());
		});
		await flushLayout();
		const heldScrollTop = viewport().scrollTop;
		beforeStreamStoresSend?.();

		// The stored message mounts in the same commit as the reply's first row.
		act(() => {
			store.batch(() => {
				store.upsertDurableMessages([mockSentMessage]);
				store.applyServerChatStatus("running");
				store.applyMessagePart({ type: "text", text: "A long answer" });
			});
		});
		await flushLayout();
		act(() => {
			store.batch(() => {
				store.upsertDurableMessages([mockLongReply]);
				store.clearStreamState();
				store.applyServerChatStatus("waiting");
			});
		});
		await flushLayout();
		return heldScrollTop;
	};

	it("leaves no blank space below a long reply after the stream replaces the sent message's pending row", async () => {
		const heldScrollTop = await runTurnWithLongReply();

		expect(viewport().scrollTop).toBe(heldScrollTop);
		// The transcript ends at the reply's last row.
		expect(viewport().scrollHeight).toBe(300 + 300 + sentRowHeight + 600);
	});

	it("leaves the view where the reader scrolled before the stream replaces the pending row", async () => {
		let scrolledTop = 0;
		await runTurnWithLongReply(() => {
			fireEvent.wheel(viewport(), { deltaY: -400 });
			viewport().scrollTop -= 400;
			fireEvent.scroll(viewport());
			scrolledTop = viewport().scrollTop;
		});

		expect(viewport().scrollTop).toBe(scrolledTop);
	});

	it("returns to the end with no blank space when a send fails", async () => {
		const store = await renderWaitingChat();
		await act(async () => {
			await expect(
				send(store, () => Promise.reject(new Error("send failed"))),
			).rejects.toThrow("send failed");
		});
		await flushLayout();

		const historyHeight = 300 + 300;
		expect(viewport().scrollHeight).toBe(historyHeight);
		expect(viewport().scrollTop).toBe(historyHeight - viewport().clientHeight);
	});
});
