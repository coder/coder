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

		return (
			<ChatPageTimeline
				chatID={CHAT_ID}
				store={store}
				persistedError={undefined}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByTestId("assistant-bottom-spacer")).toBeNull();
	},
};

export const SpacerVisibleWhenNotStreaming: Story = {
	render: () => {
		const store = buildRegressionStore();

		return (
			<ChatPageTimeline
				chatID={CHAT_ID}
				store={store}
				persistedError={undefined}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByTestId("assistant-bottom-spacer")).toBeInTheDocument();
	},
};
