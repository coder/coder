import { MessageScroller } from "@shadcn/react/message-scroller";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import {
	chatPromptsKey,
	userCompactionThresholdsKey,
} from "#/api/queries/chats";
import { preferenceSettingsKey } from "#/api/queries/users";
import { workspacesKey } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat, MockChatQueuedMessage } from "#/testHelpers/chatEntities";
import { MockChatModel } from "#/testHelpers/chatModels";
import {
	MockUserChatCompactionThresholds,
	MockUserOwner,
	MockUserPreferenceSettings,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
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
			organizationId="organization-id"
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
	decorators: [withAuthProvider, withDashboardProvider],
	parameters: {
		user: MockUserOwner,
		queries: [
			{
				key: preferenceSettingsKey,
				data: MockUserPreferenceSettings,
			},
			{
				key: userCompactionThresholdsKey,
				data: MockUserChatCompactionThresholds,
			},
			{
				key: workspacesKey({ q: "owner:me", limit: 0 }),
				data: {
					workspaces: [],
					count: 0,
				} satisfies TypesGen.WorkspacesResponse,
			},
		],
	},
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const CHAT_ID = "chat-page-content-stories";

const mockUserChatCompactionThresholdsWithOverride: TypesGen.UserChatCompactionThresholds =
	{
		...MockUserChatCompactionThresholds,
		thresholds: [
			{
				model_config_id: MockChat.last_model_config_id,
				threshold_percent: 60,
			},
		],
	};

const mockCompactionModels: readonly TypesGen.ChatModel[] = [
	{
		...MockChatModel,
		id: MockChat.last_model_config_id,
	},
];

// Renders only the composer half of the chat page. Empty chat id and
// organization keep the prompt-history and draft attachment queries disabled.
const StoryChatPageInput: FC<{
	store: ReturnType<typeof createChatStore>;
	onInterrupt?: () => void;
}> = ({ store, onInterrupt }) => (
	<div className="mx-auto w-full max-w-3xl p-4">
		<ChatPageInput
			chat={{ ...MockChat, id: "", organization_id: "" }}
			store={store}
			models={[]}
			onSend={fn()}
			onDeleteQueuedMessage={fn()}
			onPromoteQueuedMessage={fn()}
			onInterrupt={onInterrupt ?? fn()}
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
			onCancelHistoryEdit={fn()}
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
			...MockChatQueuedMessage,
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
};

// Matches the fixed terminal error path.
const errorClearsStreamStore = createChatStore();
export const ErrorClearsStreamingTool: Story = {
	render: () => {
		errorClearsStreamStore.resetTransientState();
		errorClearsStreamStore.replaceMessages([
			buildMessage(1, "user", [{ type: "text", text: "Create a workspace" }]),
		]);
		errorClearsStreamStore.setChatStatus("running");
		errorClearsStreamStore.applyMessagePart({
			type: "tool-call",
			tool_call_id: "create-workspace-call",
			tool_name: "create_workspace",
			args: { name: "dev" },
		});

		return (
			<ChatWorkspaceContext value={{ workspaceId: "workspace-1" }}>
				<StoryChatPageTimeline store={errorClearsStreamStore} />
			</ChatWorkspaceContext>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Creating workspace…")).toBeInTheDocument();

		errorClearsStreamStore.batch(() => {
			errorClearsStreamStore.applyServerChatStatus("error");
			errorClearsStreamStore.setStreamError({
				kind: "generic",
				message: "The chat session ended unexpectedly.",
			});
			errorClearsStreamStore.clearStreamState();
		});

		await waitFor(() => {
			expect(canvas.queryByText("Creating workspace…")).toBeNull();
		});
		expect(canvas.getByText("Request failed")).toBeInTheDocument();
		expect(
			canvas.getByText("The chat session ended unexpectedly."),
		).toBeInTheDocument();
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
};

// Interrupting is busy without stream state; interrupt retries are
// rejected by the backend, so Stop stays present but disabled.
const interruptingOnInterrupt = fn();
export const InterruptingShowsBusyComposer: Story = {
	render: () => {
		const store = buildInterruptingStore();
		return (
			<MessageScroller.Provider autoScroll defaultScrollPosition="end">
				<div className="flex h-full flex-col">
					<ChatPageTimeline
						organizationId="organization-id"
						store={store}
						persistedError={undefined}
						hasMoreMessages={false}
						isFetchingMoreMessages={false}
						isHydratingMessages={false}
						hasFetchMoreError={false}
						onFetchMoreMessages={async () => {}}
					/>
					<StoryChatPageInput
						store={store}
						onInterrupt={interruptingOnInterrupt}
					/>
				</div>
			</MessageScroller.Provider>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Also rename the helpers")).toBeInTheDocument();
		expect(canvas.getByRole("button", { name: "Stop" })).toBeDisabled();
		expect(canvas.getByRole("status")).toHaveTextContent(
			"Interrupting. Waiting for the agent to stop.",
		);
		expect(canvas.queryByRole("button", { name: "Send" })).toBeNull();
		expect(canvas.getByText("Interrupting")).toBeInTheDocument();
		expect(canvas.queryByText("Thinking")).toBeNull();

		await userEvent.click(
			canvas.getByRole("textbox", { name: "Chat message" }),
		);
		await userEvent.keyboard("{Escape}");
		expect(interruptingOnInterrupt).not.toHaveBeenCalled();
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

const CompactionChatPageInput: FC = () => {
	const store = createChatStore();
	store.replaceMessages([
		buildMessage(1, "user", [{ type: "text", text: "Summarize the diff" }]),
		{
			...buildMessage(2, "assistant", [
				{ type: "text", text: "The diff is a rename." },
			]),
			usage: {
				input_tokens: 30_000,
				output_tokens: 10_000,
				context_limit: 128_000,
			},
		},
	]);

	return (
		<div className="mx-auto w-full max-w-3xl p-4">
			<ChatPageInput
				chat={MockChat}
				store={store}
				models={mockCompactionModels}
				onSend={fn()}
				onDeleteQueuedMessage={fn()}
				onPromoteQueuedMessage={fn()}
				onInterrupt={fn()}
				isInputDisabled={false}
				isSendPending={false}
				isInterruptPending={false}
				hasModelOptions={false}
				selectedModel={MockChat.last_model_config_id}
				onModelChange={fn()}
				modelOptions={[]}
				modelSelectorPlaceholder="Select model"
				canConfigureAgentSetup={false}
				isEditing={false}
				onCancelHistoryEdit={fn()}
			/>
		</div>
	);
};

const openContextUsage = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	await userEvent.click(
		await canvas.findByRole("button", { name: /Context usage/ }),
	);
};

export const CompactsAtUserOverride: Story = {
	parameters: {
		pixel: { exclude: true },
		queries: [
			{
				key: preferenceSettingsKey,
				data: MockUserPreferenceSettings,
			},
			{
				key: userCompactionThresholdsKey,
				data: mockUserChatCompactionThresholdsWithOverride,
			},
			{
				key: chatPromptsKey(MockChat.id),
				data: { prompts: [] } satisfies TypesGen.ChatPromptsResponse,
			},
			{
				key: workspacesKey({ q: "owner:me", limit: 0 }),
				data: {
					workspaces: [],
					count: 0,
				} satisfies TypesGen.WorkspacesResponse,
			},
		],
	},
	render: () => <CompactionChatPageInput />,
	play: async ({ canvasElement }) => {
		await openContextUsage(canvasElement);
		await waitFor(() => {
			expect(within(document.body).getByText("Compacts at 60%")).toBeVisible();
		});
		expect(within(document.body).queryByText("Compacts at 70%")).toBeNull();
	},
};

export const CompactsAtHistoricalModelDefault: Story = {
	parameters: {
		pixel: { exclude: true },
		queries: [
			{
				key: preferenceSettingsKey,
				data: MockUserPreferenceSettings,
			},
			{
				key: userCompactionThresholdsKey,
				data: MockUserChatCompactionThresholds,
			},
			{
				key: chatPromptsKey(MockChat.id),
				data: { prompts: [] } satisfies TypesGen.ChatPromptsResponse,
			},
			{
				key: workspacesKey({ q: "owner:me", limit: 0 }),
				data: {
					workspaces: [],
					count: 0,
				} satisfies TypesGen.WorkspacesResponse,
			},
		],
	},
	render: () => <CompactionChatPageInput />,
	play: async ({ canvasElement }) => {
		await openContextUsage(canvasElement);
		await waitFor(() => {
			expect(within(document.body).getByText("Compacts at 70%")).toBeVisible();
		});
	},
};
