import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
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
