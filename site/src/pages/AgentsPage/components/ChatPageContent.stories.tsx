import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { ChatWorkspaceContext } from "../context/ChatWorkspaceContext";
import { createChatStore } from "./ChatConversation/chatStore";
import { withMessageScroller } from "./ChatConversation/messageScrollerStoryHarness";
import {
	buildStreamRenderState,
	FIXTURE_NOW,
	textResponseStreamParts,
} from "./ChatConversation/storyFixtures";
import { ChatPageTimeline } from "./ChatPageContent";

const meta = {
	title: "pages/AgentsPage/ChatPageContent",
	decorators: [withMessageScroller],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const CHAT_ID = "chat-page-content-stories";

const buildMessage = (
	id: number,
	role: TypesGen.ChatMessageRole,
	content: TypesGen.ChatMessagePart[],
): TypesGen.ChatMessage => ({
	id,
	chat_id: CHAT_ID,
	created_at: new Date(FIXTURE_NOW - (10 - id) * 60_000).toISOString(),
	role,
	content,
});

const buildLongConversation = (count: number): TypesGen.ChatMessage[] => {
	const messages: TypesGen.ChatMessage[] = [];
	for (let id = 1; id <= count; id++) {
		const role: TypesGen.ChatMessageRole = id % 2 === 1 ? "user" : "assistant";
		messages.push(
			buildMessage(id, role, [
				{
					type: "text",
					text:
						role === "user"
							? `Question ${id}: walk me through how the scroller keeps its place.`
							: `Answer ${id}: the viewport owns the scroll, the content overflows it, ` +
								"and autoScroll follows the latest anchored turn as new rows mount.",
				},
			]),
		);
	}
	return messages;
};

const buildThinkingSpacerStore = () => {
	const store = createChatStore();

	store.replaceMessages([
		buildMessage(1, "user", [{ type: "text", text: "Read the source files" }]),
		buildMessage(2, "assistant", [
			{
				type: "reasoning",
				text: "I should think before answering.",
			},
		]),
		// A following message is needed so the spacer renders.
		buildMessage(3, "user", [{ type: "text", text: "Any progress?" }]),
	]);

	return store;
};

export const SpacerVisibleWhenNotStreaming: Story = {
	render: () => {
		const store = buildThinkingSpacerStore();

		return <ChatPageTimeline store={store} persistedError={undefined} />;
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		canvas.getByRole("button", { name: /thinking/i });
		expect(canvas.getByTestId("assistant-bottom-spacer")).toBeInTheDocument();
	},
};

export const DurableUnresolvedWorkspaceToolRuns: Story = {
	render: () => {
		const store = createChatStore();
		store.replaceMessages([
			buildMessage(1, "user", [{ type: "text", text: "Create a workspace" }]),
			buildMessage(2, "assistant", [
				{
					type: "tool-call",
					tool_call_id: "create-workspace-call",
					tool_name: "create_workspace",
					args: { name: "dev" },
				},
			]),
		]);
		store.setChatStatus("running");

		return (
			<ChatWorkspaceContext value={{ workspaceId: "workspace-1" }}>
				<ChatPageTimeline store={store} persistedError={undefined} />
			</ChatWorkspaceContext>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Creating workspace…")).toBeInTheDocument();
		expect(canvas.queryByText("Created workspace")).toBeNull();
		expect(canvas.getByText("Loading build logs…")).toBeInTheDocument();
	},
};

// Regression guard for the autoScroll wiring. A flex-child Content shrinks to
// the viewport height instead of overflowing, which silently disables
// scrolling and leaves autoScroll nothing to follow. A long transcript must
// overflow the fixed-height viewport, and last-anchor autoScroll must move the
// scroll position off the top edge. An idle transcript must also leave no
// trailing live-stream placeholder row.
export const LongTranscriptOverflowsAndAutoScrolls: Story = {
	render: () => {
		const store = createChatStore();
		store.replaceMessages(buildLongConversation(40));

		return <ChatPageTimeline store={store} persistedError={undefined} />;
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const content = canvas.getByTestId("conversation-timeline");
		const viewport = content.parentElement;
		expect(viewport).not.toBeNull();

		await waitFor(() => {
			const el = viewport as HTMLElement;
			// Content overflows the fixed-height viewport, so it is scrollable.
			expect(el.scrollHeight).toBeGreaterThan(el.clientHeight);
			// last-anchor autoScroll positioned the viewport below the top edge.
			expect(el.scrollTop).toBeGreaterThan(0);
		});

		// The idle live tail must not mount a trailing placeholder item, which
		// would shift appended turns off the tail where the scroller anchors them.
		expect(
			canvasElement.querySelector('[data-message-id="__live_stream__"]'),
		).toBeNull();
	},
};

// A freshly sent user turn must scroll to the top, the way a new prompt jumps up
// while its reply streams in below. This broke when a permanently mounted
// live-stream row sat at the tail: each appended turn landed before it, so the
// scroller never detected the new anchor. The store is module scoped so the
// play function can append to the same instance the render mounted.
const anchorOnAppendStore = createChatStore();
const anchorOnAppendBase = buildLongConversation(30);
anchorOnAppendStore.replaceMessages(anchorOnAppendBase);
const appendedUserMessageId = 31;

export const NewUserTurnAnchorsToTop: Story = {
	render: () => (
		<ChatPageTimeline store={anchorOnAppendStore} persistedError={undefined} />
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const content = canvas.getByTestId("conversation-timeline");
		const viewport = content.parentElement as HTMLElement;

		// Reset to the base transcript so the test always exercises the append
		// path, even if a prior run left the appended turn in this shared store.
		anchorOnAppendStore.replaceMessages(anchorOnAppendBase);

		// Let the initial last-anchor scroll settle before appending.
		await waitFor(() => {
			expect(viewport.scrollHeight).toBeGreaterThan(viewport.clientHeight);
		});

		anchorOnAppendStore.replaceMessages([
			...anchorOnAppendBase,
			buildMessage(appendedUserMessageId, "user", [
				{ type: "text", text: "One more: does the new turn jump to the top?" },
			]),
		]);

		await waitFor(() => {
			const row = canvasElement.querySelector(
				`[data-message-id="${appendedUserMessageId}"]`,
			);
			expect(row).not.toBeNull();
			const viewportRect = viewport.getBoundingClientRect();
			const offset =
				(row as HTMLElement).getBoundingClientRect().top - viewportRect.top;
			// Anchored near the top, not pinned to the bottom edge.
			expect(offset).toBeGreaterThanOrEqual(0);
			expect(offset).toBeLessThan(viewportRect.height / 2);
		});
	},
};

// The streaming reply (live-tail row) must sit the same distance below the
// previous turn as a committed reply, so it does not jump when it hands off to
// the transcript. A stray top margin on the live-tail row reintroduces the
// shift. The row's content is flush to its scroller Item (no extra top margin).
export const StreamingReplyHasNoExtraTopMargin: Story = {
	render: () => {
		const store = createChatStore();
		store.replaceMessages([
			buildMessage(1, "user", [{ type: "text", text: "Stream me a reply" }]),
		]);
		store.setChatStatus("running");
		store.setStreamState(
			buildStreamRenderState(textResponseStreamParts).streamState,
		);

		return <ChatPageTimeline store={store} persistedError={undefined} />;
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText(/storybook streamed answer/i)).toBeVisible();
		});

		const row = canvasElement.querySelector<HTMLElement>(
			'[data-message-id="__live_stream__"]',
		);
		expect(row).not.toBeNull();
		const inner = row?.firstElementChild as HTMLElement | null;
		expect(inner).not.toBeNull();

		// The content is flush to the top of its Item; the row relies on the
		// scroller's gap-2 for spacing rather than its own margin.
		const offset =
			(inner as HTMLElement).getBoundingClientRect().top -
			(row as HTMLElement).getBoundingClientRect().top;
		expect(offset).toBeLessThan(2);
	},
};

// Native browser scroll anchoring (overflow-anchor: auto) adjusts scrollTop on
// its own when rows mount above the fold, which double-corrects against the
// scroller's manual preserveScrollOnPrepend restoration and surfaces as a
// phantom jump. The viewport must opt out so the library is the sole authority
// over scroll position. Asserting the computed value catches a class removal,
// which a prepend-position test cannot: the anchoring heuristic is async and
// will not reproduce the over-correction within a single play frame.
export const ViewportDisablesNativeScrollAnchoring: Story = {
	render: () => {
		const store = createChatStore();
		store.replaceMessages(buildLongConversation(8));

		return <ChatPageTimeline store={store} persistedError={undefined} />;
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const content = canvas.getByTestId("conversation-timeline");
		const viewport = content.parentElement as HTMLElement;

		expect(getComputedStyle(viewport).getPropertyValue("overflow-anchor")).toBe(
			"none",
		);
	},
};

export const HiddenAssistantPlaceholderDoesNotRender: Story = {
	render: () => {
		const store = createChatStore();

		store.replaceMessages([
			buildMessage(1, "user", [{ type: "text", text: "Run the command" }]),
			buildMessage(2, "assistant", [{ type: "text", text: "Done." }]),
			buildMessage(3, "assistant", []),
			buildMessage(4, "user", [{ type: "text", text: "Thanks!" }]),
		]);

		return <ChatPageTimeline store={store} persistedError={undefined} />;
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByText("Message has no renderable content.")).toBeNull();

		const rows = canvasElement.querySelectorAll(
			'[data-role="user"], [data-role="assistant"]',
		);
		expect(rows).toHaveLength(3);
		expect(rows[1]).toHaveAttribute("data-role", "assistant");
		expect(rows[1]).toHaveTextContent("Done.");
	},
};
