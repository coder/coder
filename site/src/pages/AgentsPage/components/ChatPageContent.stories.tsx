import { MessageScroller } from "@shadcn/react/message-scroller";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { expect, fn, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { ChatWorkspaceContext } from "../context/ChatWorkspaceContext";
import { createChatStore } from "./ChatConversation/chatStore";
import { FIXTURE_NOW } from "./ChatConversation/storyFixtures";
import { ChatPageInput, ChatPageTimeline } from "./ChatPageContent";

// These stories cover transcript rendering, so history paging stays idle.
const StoryChatPageTimeline: FC<{
	store: ReturnType<typeof createChatStore>;
}> = ({ store }) => (
	<MessageScroller.Provider autoScroll defaultScrollPosition="end">
		<ChatPageTimeline
			store={store}
			persistedError={undefined}
			hasMoreMessages={false}
			isFetchingMoreMessages={false}
			isHydratingMessages={false}
			hasFetchMoreError={false}
			onFetchMoreMessages={async () => {}}
		/>
	</MessageScroller.Provider>
);

const meta = {
	title: "pages/AgentsPage/ChatPageContent",
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const CHAT_ID = "chat-page-content-stories";

// Renders only the composer half of the chat page. chatId and
// organizationId stay undefined so the prompt-history and draft
// attachment queries stay disabled.
const StoryChatPageInput: FC<{
	store: ReturnType<typeof createChatStore>;
}> = ({ store }) => (
	<div className="mx-auto w-full max-w-3xl p-4">
		<ChatPageInput
			organizationId={undefined}
			store={store}
			compressionThreshold={undefined}
			onSend={fn()}
			sendShortcut="enter"
			onDeleteQueuedMessage={fn()}
			onPromoteQueuedMessage={fn()}
			onInterrupt={fn()}
			isInputDisabled={false}
			isSendPending={false}
			isInterruptPending={false}
			hasModelOptions
			selectedModel="model-config-1"
			onModelChange={fn()}
			modelOptions={[
				{
					id: "model-config-1",
					provider: "openai",
					model: "gpt-4o",
					displayName: "GPT-4o",
				},
			]}
			modelSelectorPlaceholder="Select model"
			canConfigureAgentSetup={false}
			isEditing={false}
			editingQueuedMessageID={null}
			onStartQueueEdit={fn()}
			onCancelQueueEdit={fn()}
			isEditingHistoryMessage={false}
			onCancelHistoryEdit={fn()}
			workspaceOptions={[]}
			selectedWorkspaceId={null}
			isWorkspaceLoading={false}
		/>
	</div>
);

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

// Matches the backend I1 state: an interruption has been requested
// and the stream has already been torn down, so the store holds no
// stream state while the chat status is still "interrupting".
const buildInterruptingStore = () => {
	const store = createChatStore();
	store.replaceMessages([
		buildMessage(1, "user", [{ type: "text", text: "Refactor the module" }]),
	]);
	store.setQueuedMessages([
		{
			id: 2,
			chat_id: CHAT_ID,
			content: [{ type: "text", text: "Also rename the helpers" }],
			created_at: new Date(FIXTURE_NOW).toISOString(),
		},
	]);
	store.setChatStatus("interrupting");
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
		// A following message is needed so the spacer renders.
		buildMessage(3, "user", [{ type: "text", text: "Any progress?" }]),
	]);

	return store;
};

export const SpacerVisibleWhenNotStreaming: Story = {
	render: () => {
		const store = buildThinkingSpacerStore();

		return <StoryChatPageTimeline store={store} />;
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
				<StoryChatPageTimeline store={store} />
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

		return <StoryChatPageTimeline store={store} />;
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

		return <StoryChatPageTimeline store={store} />;
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByTestId("conversation-timeline")).toHaveTextContent(
			/alpha[\s\S]*bravo[\s\S]*charlie[\s\S]*delta/,
		);
	},
};

// Interrupting is busy without stream state; interrupt retries are
// rejected by the backend, so Stop stays present but disabled.
export const InterruptingShowsBusyComposer: Story = {
	render: () => {
		const store = buildInterruptingStore();
		return <StoryChatPageInput store={store} />;
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Also rename the helpers")).toBeInTheDocument();
		expect(canvas.getByRole("button", { name: "Stop" })).toBeDisabled();
		expect(canvas.queryByRole("button", { name: "Send" })).toBeNull();
	},
};

export const RunningShowsBusyComposer: Story = {
	render: () => {
		const store = buildInterruptingStore();
		store.setChatStatus("running");
		return <StoryChatPageInput store={store} />;
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Also rename the helpers")).toBeInTheDocument();
		expect(canvas.getByRole("button", { name: "Stop" })).toBeEnabled();
		expect(canvas.queryByRole("button", { name: "Send" })).toBeNull();
	},
};
