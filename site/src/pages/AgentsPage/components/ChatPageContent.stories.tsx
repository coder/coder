import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { ChatWorkspaceContext } from "../context/ChatWorkspaceContext";
import { createChatStreamStore } from "./ChatConversation/chatStreamStore";
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

const thinkingSpacerMessages = [
	buildMessage(1, "user", [{ type: "text", text: "Read the source files" }]),
	buildMessage(2, "assistant", [
		{
			type: "reasoning",
			text: "I should think before answering.",
		},
	]),
	// A following message is needed so the spacer renders.
	buildMessage(3, "user", [{ type: "text", text: "Any progress?" }]),
];

export const SpacerVisibleWhenNotStreaming: Story = {
	render: () => (
		<ChatPageTimeline
			store={createChatStreamStore()}
			chatStatus="waiting"
			messages={thinkingSpacerMessages}
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
	render: () => {
		const store = createChatStreamStore();
		const messages = [
			buildMessage(1, "user", [{ type: "text", text: "Create a workspace" }]),
			buildMessage(2, "assistant", [
				{
					type: "tool-call",
					tool_call_id: "create-workspace-call",
					tool_name: "create_workspace",
					args: { name: "dev" },
				},
			]),
		];

		return (
			<ChatWorkspaceContext value={{ workspaceId: "workspace-1" }}>
				<ChatPageTimeline
					store={store}
					chatStatus="running"
					messages={messages}
					persistedError={undefined}
				/>
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

export const HiddenAssistantPlaceholderDoesNotRender: Story = {
	render: () => (
		<ChatPageTimeline
			store={createChatStreamStore()}
			chatStatus="waiting"
			messages={[
				buildMessage(1, "user", [{ type: "text", text: "Run the command" }]),
				buildMessage(2, "assistant", [{ type: "text", text: "Done." }]),
				buildMessage(3, "assistant", []),
				buildMessage(4, "user", [{ type: "text", text: "Thanks!" }]),
			]}
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
