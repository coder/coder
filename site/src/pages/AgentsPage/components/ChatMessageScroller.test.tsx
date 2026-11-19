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
import { MessageScroller, useMessageScroller } from "#/vendor/message-scroller";
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
	"message:4": 600,
};
// A pending row's ID is chosen at the click, so rows not listed are the sent
// message, as a pending row and once stored.
const sentRowHeight = 40;

describe("ChatMessageScroller after a send", () => {
	let liveReplyHeight = 0;
	setupMessageScrollerLayout({
		rowHeight: (messageId) =>
			messageId === "live-assistant"
				? liveReplyHeight
				: (rowHeights[messageId] ?? sentRowHeight),
	});

	let scrollToEnd: ReturnType<typeof useMessageScroller>["scrollToEnd"];
	// AgentChatPage takes scrollToEnd for submitChatTurn from the same hook.
	const ScrollToEnd: React.FC = () => {
		({ scrollToEnd } = useMessageScroller());
		return null;
	};

	const renderWaitingChat = async () => {
		liveReplyHeight = 0;
		const store = createChatStore();
		store.upsertDurableMessages([mockFirstQuestion, mockFirstAnswer]);
		store.applyServerChatStatus("waiting");
		renderWithAuth(
			<MessageScroller.Provider autoScroll defaultScrollPosition="end">
				<ScrollToEnd />
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

		const streamFrame = async (apply: () => void) => {
			act(() => store.batch(apply));
			await flushLayout();
		};
		return {
			// As submitChatTurn does for a send that adds a pending row.
			send: async () => {
				act(() => {
					void store.runPendingAction(
						{
							kind: "send",
							chatID: MockChatMessage.chat_id,
							content: sentContent,
							busyBehavior: "queue",
						},
						() => Promise.resolve(),
					);
					scrollToEnd({ behavior: "auto" });
				});
				await flushLayout();
			},
			startReply: () =>
				streamFrame(() => {
					liveReplyHeight = 200;
					store.upsertDurableMessages([mockSentMessage]);
					store.applyServerChatStatus("running");
					store.applyMessagePart({ type: "text", text: "A long" });
				}),
			continueReply: () =>
				streamFrame(() => {
					liveReplyHeight = 600;
					store.applyMessagePart({ type: "text", text: " answer" });
				}),
			endReply: () =>
				streamFrame(() => {
					store.upsertDurableMessages([mockLongReply]);
					store.clearStreamState();
					store.applyServerChatStatus("waiting");
				}),
		};
	};

	const scrollUp = (distance: number) => {
		fireEvent.wheel(viewport(), { deltaY: -distance });
		viewport().scrollTop -= distance;
		fireEvent.scroll(viewport());
	};

	// The view rests at the end of rows that add up to this height, so no
	// blank space follows the last row.
	const atEndOf = (rowsHeight: number) => ({
		scrollTop: rowsHeight - viewport().clientHeight,
		scrollHeight: rowsHeight,
	});
	const position = () => ({
		scrollTop: viewport().scrollTop,
		scrollHeight: viewport().scrollHeight,
	});

	const transcriptHeight =
		rowHeights["message:1"] + rowHeights["message:2"] + sentRowHeight;

	it.each([
		{ start: "the end", scrolledUp: false },
		{ start: "earlier messages", scrolledUp: true },
	])(
		"moves to the end at the click and follows a long reply after a send from $start",
		async ({ scrolledUp }) => {
			const chat = await renderWaitingChat();
			if (scrolledUp) {
				scrollUp(200);
			}

			await chat.send();
			expect(position()).toEqual(atEndOf(transcriptHeight));
			await chat.startReply();
			expect(position()).toEqual(atEndOf(transcriptHeight + 200));
			await chat.continueReply();
			expect(position()).toEqual(atEndOf(transcriptHeight + 600));
			await chat.endReply();
			expect(position()).toEqual(
				atEndOf(transcriptHeight + rowHeights["message:4"]),
			);
		},
	);

	it("leaves the view where the reader scrolled during the reply", async () => {
		const chat = await renderWaitingChat();
		await chat.send();
		await chat.startReply();
		scrollUp(300);
		const scrolledTop = viewport().scrollTop;

		await chat.continueReply();
		await chat.endReply();

		expect(viewport().scrollTop).toBe(scrolledTop);
	});
});
