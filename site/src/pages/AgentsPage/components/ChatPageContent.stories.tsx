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

// Reproduces the tool-result flicker window: a durable pending read_file
// call (tc-1) is already rendered while its result streams before the tool
// step commits, so the result-only row must be filtered out. A parallel
// in-stream read_file call (tc-2) is not pending in the durable transcript,
// so its streamed call and result merge normally and stay visible.
export const StreamedResultForPendingToolDoesNotDuplicate: Story = {
	render: () => {
		const store = createChatStore();
		store.replaceMessages([
			buildMessage(1, "user", [{ type: "text", text: "Read the files" }]),
			buildMessage(2, "assistant", [
				{
					type: "tool-call",
					tool_call_id: "tc-1",
					tool_name: "read_file",
					args: { path: "src/Alpha.ts" },
				},
			]),
		]);
		store.setChatStatus("running");

		// tc-1 result streams before its tool step commits. tc-2 is a new
		// in-stream call whose result merges with its own call.
		store.applyMessageParts([
			{
				type: "tool-result",
				tool_call_id: "tc-1",
				tool_name: "read_file",
				result: { content: "alpha body" },
			},
			{
				type: "tool-call",
				tool_call_id: "tc-2",
				tool_name: "read_file",
				args: { path: "src/Beta.ts" },
			},
			{
				type: "tool-result",
				tool_call_id: "tc-2",
				tool_name: "read_file",
				result: { content: "beta body" },
			},
		]);

		return <ChatPageTimeline store={store} persistedError={undefined} />;
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// tc-1 renders once as the durable pending row; its streamed result-only
		// duplicate ("Read file") is suppressed.
		expect(canvas.getAllByText("Reading Alpha.ts…")).toHaveLength(1);
		expect(canvas.queryByText("Read file")).toBeNull();
		// tc-2 was never durable pending, so its streamed row still renders.
		expect(canvas.getByText("Read Beta.ts")).toBeInTheDocument();
	},
};

// A durable pending advisor call keeps streaming its result_delta parts
// into the live tail: filterPendingStreamState preserves still-streaming
// results so the progressive advice UI keeps rendering for the pending id.
export const StreamedAdvisorAdviceForPendingToolStillRenders: Story = {
	render: () => {
		const store = createChatStore();
		store.replaceMessages([
			buildMessage(1, "user", [{ type: "text", text: "Plan the change" }]),
			buildMessage(2, "assistant", [
				{
					type: "tool-call",
					tool_call_id: "advisor-1",
					tool_name: "advisor",
					args: { question: "Is this migration safe?" },
				},
			]),
		]);
		store.setChatStatus("running");

		store.applyMessageParts([
			{
				type: "tool-result",
				tool_call_id: "advisor-1",
				tool_name: "advisor",
				result_delta: "Use ",
			},
			{
				type: "tool-result",
				tool_call_id: "advisor-1",
				tool_name: "advisor",
				result_delta: "small steps.",
			},
		]);

		return <ChatPageTimeline store={store} persistedError={undefined} />;
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The accumulated advice text is the progressive surface the filter
		// must keep, and it renders exactly once.
		await canvas.findByText("Use small steps.");
		expect(canvas.getAllByText("Use small steps.")).toHaveLength(1);
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

export const MergedMessagesRenderInIDOrder: Story = {
	render: () => {
		const store = createChatStore();
		// One created_at for all four, so id is the only ordering signal.
		const batchCreatedAt = new Date(FIXTURE_NOW).toISOString();
		const batched = (
			id: number,
			role: TypesGen.ChatMessageRole,
			text: string,
		): TypesGen.ChatMessage => ({
			...buildMessage(id, role, [{ type: "text", text }]),
			created_at: batchCreatedAt,
		});

		store.replaceMessages([
			batched(3, "user", "charlie"),
			batched(4, "assistant", "delta"),
		]);
		store.upsertDurableMessages([
			batched(1, "user", "alpha"),
			batched(2, "assistant", "bravo"),
		]);

		return <ChatPageTimeline store={store} persistedError={undefined} />;
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByTestId("conversation-timeline")).toHaveTextContent(
			/alpha[\s\S]*bravo[\s\S]*charlie[\s\S]*delta/,
		);
	},
};
