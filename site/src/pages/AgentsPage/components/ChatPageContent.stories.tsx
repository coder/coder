import type { Meta, StoryObj } from "@storybook/react-vite";
import type { InfiniteData } from "react-query";
import { expect, within } from "storybook/test";
import { chatKeys } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { ChatWorkspaceContext } from "../context/ChatWorkspaceContext";
import { createChatStore } from "./ChatConversation/chatStore";
import { FIXTURE_NOW } from "./ChatConversation/storyFixtures";
import { ChatPageTimeline } from "./ChatPageContent";

const meta = {
	title: "pages/AgentsPage/ChatPageContent",
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

// Durable messages and chat status are both canonical in the query cache, so a
// story seeds those entries rather than the store. The store only carries the
// transient streaming overlay.
const seededChat = (
	messages: readonly TypesGen.ChatMessage[],
	status: TypesGen.ChatStatus = "waiting",
) => [
	{
		key: chatKeys.detail(CHAT_ID),
		data: { ...MockChat, id: CHAT_ID, status } satisfies TypesGen.Chat,
	},
	{
		key: chatKeys.messages(CHAT_ID),
		data: {
			// Pages arrive newest-first from the API.
			pages: [
				{
					messages: [...messages].sort((left, right) => right.id - left.id),
					queued_messages: [],
					has_more: false,
				},
			],
			pageParams: [undefined],
		} satisfies InfiniteData<TypesGen.ChatMessagesResponse>,
	},
];

export const SpacerVisibleWhenNotStreaming: Story = {
	parameters: {
		queries: seededChat([
			buildMessage(1, "user", [
				{ type: "text", text: "Read the source files" },
			]),
			buildMessage(2, "assistant", [
				{ type: "reasoning", text: "I should think before answering." },
			]),
			// A following message is needed so the spacer renders.
			buildMessage(3, "user", [{ type: "text", text: "Any progress?" }]),
		]),
	},
	render: () => (
		<ChatPageTimeline
			store={createChatStore()}
			chatId={CHAT_ID}
			persistedError={undefined}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		canvas.getByRole("button", { name: /thinking/i });
		expect(canvas.getByTestId("assistant-bottom-spacer")).toBeInTheDocument();
	},
};

export const DurableUnresolvedWorkspaceToolRuns: Story = {
	parameters: {
		queries: seededChat(
			[
				buildMessage(1, "user", [{ type: "text", text: "Create a workspace" }]),
				buildMessage(2, "assistant", [
					{
						type: "tool-call",
						tool_call_id: "create-workspace-call",
						tool_name: "create_workspace",
						args: { name: "dev" },
					},
				]),
			],
			"running",
		),
	},
	render: () => (
		<ChatWorkspaceContext value={{ workspaceId: "workspace-1" }}>
			<ChatPageTimeline
				store={createChatStore()}
				chatId={CHAT_ID}
				persistedError={undefined}
			/>
		</ChatWorkspaceContext>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Creating workspace…")).toBeInTheDocument();
		expect(canvas.queryByText("Created workspace")).toBeNull();
		expect(canvas.getByText("Loading build logs…")).toBeInTheDocument();
	},
};

export const HiddenAssistantPlaceholderDoesNotRender: Story = {
	parameters: {
		queries: seededChat([
			buildMessage(1, "user", [{ type: "text", text: "Run the command" }]),
			buildMessage(2, "assistant", [{ type: "text", text: "Done." }]),
			buildMessage(3, "assistant", []),
			buildMessage(4, "user", [{ type: "text", text: "Thanks!" }]),
		]),
	},
	render: () => (
		<ChatPageTimeline
			store={createChatStore()}
			chatId={CHAT_ID}
			persistedError={undefined}
		/>
	),
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

// One created_at for the whole batch, so the ID is the only ordering signal and
// the page order the API returned cannot leak into the transcript.
const batchCreatedAt = new Date(FIXTURE_NOW).toISOString();
const batchedMessage = (
	id: number,
	role: TypesGen.ChatMessageRole,
	text: string,
): TypesGen.ChatMessage => ({
	...buildMessage(id, role, [{ type: "text", text }]),
	created_at: batchCreatedAt,
});

export const MergedMessagesRenderInIDOrder: Story = {
	parameters: {
		queries: [
			{
				key: chatKeys.detail(CHAT_ID),
				data: {
					...MockChat,
					id: CHAT_ID,
					status: "waiting",
				} satisfies TypesGen.Chat,
			},
			{
				key: chatKeys.messages(CHAT_ID),
				data: {
					// Deliberately unsorted, and split across pages.
					pages: [
						{
							messages: [
								batchedMessage(4, "assistant", "delta"),
								batchedMessage(3, "user", "charlie"),
							],
							queued_messages: [],
							has_more: true,
						},
						{
							messages: [
								batchedMessage(1, "user", "alpha"),
								batchedMessage(2, "assistant", "bravo"),
							],
							queued_messages: [],
							has_more: false,
						},
					],
					pageParams: [undefined, 3],
				} satisfies InfiniteData<TypesGen.ChatMessagesResponse>,
			},
		],
	},
	render: () => (
		<ChatPageTimeline
			store={createChatStore()}
			chatId={CHAT_ID}
			persistedError={undefined}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByTestId("conversation-timeline")).toHaveTextContent(
			/alpha[\s\S]*bravo[\s\S]*charlie[\s\S]*delta/,
		);
	},
};
