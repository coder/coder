import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { createChatStore } from "./ChatConversation/chatStore";
import {
	buildStreamRenderState,
	FIXTURE_NOW,
} from "./ChatConversation/storyFixtures";
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

const buildRegressionStore = () => {
	const store = createChatStore();

	store.replaceMessages([
		buildMessage(1, "user", [{ type: "text", text: "Read the source files" }]),
		buildMessage(2, "assistant", [
			{
				type: "reasoning",
				text: "I should read SKILL.md and main.go to understand the codebase.",
			},
			{
				type: "tool-call",
				tool_call_id: "tool-1",
				tool_name: "read_file",
				args: { path: "SKILL.md" },
			},
			{
				type: "tool-call",
				tool_call_id: "tool-2",
				tool_name: "read_file",
				args: { path: "main.go" },
			},
		]),
		buildMessage(3, "tool", [
			{
				type: "tool-result",
				tool_call_id: "tool-1",
				result: { output: "# SKILL.md contents" },
			},
		]),
		buildMessage(4, "tool", [
			{
				type: "tool-result",
				tool_call_id: "tool-2",
				result: { output: "package main" },
			},
		]),
	]);

	return store;
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
	]);

	return store;
};

export const StreamingToolCallGapRegression: Story = {
	render: () => {
		const store = buildRegressionStore();
		const { streamState } = buildStreamRenderState([
			{
				type: "tool-call",
				tool_call_id: "tool-streaming",
				tool_name: "read_file",
				args: { path: "types.go" },
			},
		]);
		store.setStreamState(streamState);
		store.setChatStatus("pending");

		return <ChatPageTimeline store={store} persistedError={undefined} />;
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByTestId("assistant-bottom-spacer")).toBeNull();
	},
};

export const StartingPhaseToolCallGapRegression: Story = {
	render: () => {
		const store = buildRegressionStore();
		store.setChatStatus("running");

		return <ChatPageTimeline store={store} persistedError={undefined} />;
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		canvas.getAllByText("Thinking...");
		expect(canvas.queryByTestId("assistant-bottom-spacer")).toBeNull();
	},
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
