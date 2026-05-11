import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { createChatStore } from "./ChatConversation/chatStore";
import { buildStreamRenderState } from "./ChatConversation/storyFixtures";
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
	created_at: new Date(Date.now() - (10 - id) * 60_000).toISOString(),
	role,
	content,
});

/**
 * Regression guard: `ChatPageTimeline` must pass `isTurnActive={hasStream}`
 * to `ConversationTimeline`. When the last completed assistant message has
 * reasoning but no markdown text, `needsAssistantBottomSpacer` would insert
 * a 24 px invisible spacer below it, creating a ~32 px gap before the live
 * streaming tool call. With `isTurnActive=true` the spacer is suppressed and
 * the gap collapses to the standard `gap-2` (8 px).
 *
 * This story exercises the real store → `hasStream` → `isTurnActive` wiring
 * in `ChatPageTimeline`. If that wiring is removed the play assertion fails.
 */
export const StreamingToolCallGapRegression: Story = {
	render: () => {
		const store = createChatStore();

		store.replaceMessages([
			buildMessage(1, "user", [
				{ type: "text", text: "Read the source files" },
			]),
			// Reasoning + tool calls, no markdown text: hasCopyableContent=false
			// and parsed.reasoning is non-empty, so needsAssistantBottomSpacer
			// would fire if isTurnActive were false.
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

		// A live streaming tool call makes hasStream=true in the store,
		// which flows through as isTurnActive={true} to ConversationTimeline.
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
		// isTurnActive=true flows from hasStream → ConversationTimeline, so
		// the assistant-bottom-spacer must not be in the layout.
		expect(canvas.queryByTestId("assistant-bottom-spacer")).toBeNull();
	},
};
