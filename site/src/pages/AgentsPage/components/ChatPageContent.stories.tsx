import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import {
	chatPromptsKey,
	organizationChatModelOverridesKey,
	userCompactionThresholdsKey,
} from "#/api/queries/chats";
import { preferenceSettingsKey } from "#/api/queries/users";
import { workspacesKey } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChat,
	MockChatCompactionMessage,
	MockChatMessage,
	MockChatQueuedMessage,
} from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import {
	MockUserChatCompactionThresholds,
	MockUserOwner,
	MockUserPreferenceSettings,
} from "#/testHelpers/entities";
import { pixelWithPhone } from "#/testHelpers/pixel";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { MessageScroller } from "#/vendor/message-scroller";
import { ChatWorkspaceContext } from "../context/ChatWorkspaceContext";
import { createChatStore } from "./ChatConversation/chatStore";
import {
	buildQuestionResultMessage,
	FIXTURE_NOW,
	MockAsyncQuestionMessages,
	MockAsyncQuestionResult,
	MockQueuedQuestionAnswer,
	MockTwoAsyncQuestionsMessages,
	MockWorkingMessages,
} from "./ChatConversation/storyFixtures";
import { ChatPageInput, ChatPageTimeline } from "./ChatPageContent";

// These stories cover transcript rendering, so history paging stays idle.
const StoryChatPageTimeline: React.FC<{
	store: ReturnType<typeof createChatStore>;
	onSendAskUserQuestionResponse?: (message: string) => Promise<void> | void;
	isReadOnly?: boolean;
}> = ({ store, onSendAskUserQuestionResponse, isReadOnly = false }) => (
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
			urlTransform={(url) => url}
			mcpServers={[]}
			onSendAskUserQuestionResponse={onSendAskUserQuestionResponse}
			isReadOnly={isReadOnly}
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

const mockCompactionModelCatalog: TypesGen.OrganizationChatModelsResponse = {
	models: [{ ...MockChatModel, id: MockChat.last_model_config_id }],
	providers: [MockChatModelProviderDescriptor],
	unsupported_providers: [],
};

// Renders only the composer half of the chat page. Empty chat id and
// organization keep the prompt-history and draft attachment queries disabled.
const StoryChatPageInput: React.FC<{
	store: ReturnType<typeof createChatStore>;
	onInterrupt?: () => void;
	contextLimit?: number;
	isInputDisabled?: boolean;
}> = ({ store, onInterrupt, contextLimit, isInputDisabled = false }) => (
	<div className="mx-auto w-full max-w-3xl p-4">
		<ChatPageInput
			chat={{ ...MockChat, id: "", organization_id: "" }}
			store={store}
			modelCatalog={undefined}
			onSend={fn()}
			onDeleteQueuedMessage={fn()}
			onPromoteQueuedMessage={fn()}
			onInterrupt={onInterrupt ?? fn()}
			isInputDisabled={isInputDisabled}
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
					contextLimit,
				},
			]}
			modelSelectorPlaceholder="Select model"
			canConfigureAgentSetup={false}
			isEditing={false}
			onCancelHistoryEdit={fn()}
			isReadOnly={false}
			onReasoningEffortChange={fn()}
			unsupportedProviderNames={[]}
			onPlanModeToggle={fn()}
			isModelCatalogLoading={false}
			inputRef={{ current: null }}
			initialValue=""
			remountKey={0}
			onContentChange={fn()}
			editingFileBlocks={[]}
			mcpServers={[]}
			selectedMCPServerIds={[]}
			onMCPSelectionChange={fn()}
			onMCPAuthComplete={fn()}
			isWorkspaceLoading={false}
		/>
	</div>
);

export const ContextUsageAfterCompaction: Story = {
	render: () => {
		const store = createChatStore();
		store.replaceMessages([MockChatCompactionMessage]);
		store.setChatStatus("waiting");
		return <StoryChatPageInput store={store} contextLimit={200000} />;
	},
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(canvasElement).getByRole("button", { name: /context usage/i }),
		);
	},
};

export const UncommittedCompactionKeepsContextUsage: Story = {
	render: () => {
		const store = createChatStore();
		const previousMessage: TypesGen.ChatMessage = {
			...MockChatMessage,
			usage: { input_tokens: 90000, context_limit: 200000 },
		};
		store.replaceMessages([previousMessage]);
		store.setChatStatus("waiting");
		return <StoryChatPageInput store={store} contextLimit={200000} />;
	},
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(canvasElement).getByRole("button", { name: /context usage/i }),
		);
	},
};

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

// The advisor streams its reasoning and advice after its assistant message
// is already durable. Both must land on that message's card, with no second
// live card for the same call.
export const StreamedAdvisorResultOverlaysDurableCall: Story = {
	render: () => {
		const store = createChatStore();
		store.replaceMessages([
			buildMessage(1, "user", [
				{ type: "text", text: "Should the advisor be on by default?" },
			]),
			buildMessage(2, "assistant", [
				{
					type: "tool-call",
					tool_call_id: "advisor-call",
					tool_name: "advisor",
					args: {
						question: "on or off by default?",
						model_intent: "Checking the default",
					},
				},
			]),
		]);
		store.setChatStatus("running");
		store.applyMessageParts([
			{
				type: "tool-result",
				tool_call_id: "advisor-call",
				tool_name: "advisor",
				reasoning_delta: "Weighing the default against the rollout risk.",
			},
			{
				type: "tool-result",
				tool_call_id: "advisor-call",
				tool_name: "advisor",
				result_delta: "Keep it off by default and let teams opt in.",
			},
		]);

		return <StoryChatPageTimeline store={store} />;
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
	play: async () => {
		errorClearsStreamStore.batch(() => {
			errorClearsStreamStore.applyServerChatStatus("error");
			errorClearsStreamStore.setStreamError({
				kind: "generic",
				message: "The chat session ended unexpectedly.",
			});
			errorClearsStreamStore.clearStreamState();
		});
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
			<div className="flex h-full flex-col">
				<StoryChatPageTimeline store={store} />
				<StoryChatPageInput
					store={store}
					onInterrupt={interruptingOnInterrupt}
				/>
			</div>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Also rename the helpers")).toBeInTheDocument();
		expect(canvas.getByRole("button", { name: "Stop" })).toBeDisabled();
		expect(canvas.getByRole("status")).toHaveTextContent(
			"Interrupting. Waiting for the agent to stop.",
		);
		expect(canvas.queryByRole("button", { name: "Queue" })).toBeNull();
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
};

const CompactionChatPageInput: React.FC = () => {
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
				modelCatalog={mockCompactionModelCatalog}
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
				isReadOnly={false}
				onReasoningEffortChange={fn()}
				unsupportedProviderNames={[]}
				onPlanModeToggle={fn()}
				isModelCatalogLoading={false}
				inputRef={{ current: null }}
				initialValue=""
				remountKey={0}
				onContentChange={fn()}
				editingFileBlocks={[]}
				mcpServers={[]}
				selectedMCPServerIds={[]}
				onMCPSelectionChange={fn()}
				onMCPAuthComplete={fn()}
				isWorkspaceLoading={false}
			/>
		</div>
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
				key: organizationChatModelOverridesKey(MockChat.organization_id),
				data: { overrides: [] },
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
				key: organizationChatModelOverridesKey(MockChat.organization_id),
				data: { overrides: [] },
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
};

const buildAsyncQuestionStore = (
	messages: readonly TypesGen.ChatMessage[] = MockAsyncQuestionMessages,
) => {
	const store = createChatStore();
	store.replaceMessages(messages);
	return store;
};

const AsyncQuestionChatPage: React.FC<{
	store: ReturnType<typeof createChatStore>;
	className?: string;
	onSendAskUserQuestionResponse?: (message: string) => Promise<void> | void;
	isReadOnly?: boolean;
	isInputDisabled?: boolean;
}> = ({
	store,
	className = "flex h-160 flex-col",
	onSendAskUserQuestionResponse,
	isReadOnly,
	isInputDisabled,
}) => (
	<div className={className}>
		<StoryChatPageTimeline
			store={store}
			onSendAskUserQuestionResponse={onSendAskUserQuestionResponse}
			isReadOnly={isReadOnly}
		/>
		{!isReadOnly && (
			<StoryChatPageInput store={store} isInputDisabled={isInputDisabled} />
		)}
	</div>
);

export const AsyncQuestionDockedAboveInput: Story = {
	render: () => {
		return (
			<AsyncQuestionChatPage
				store={buildAsyncQuestionStore()}
				onSendAskUserQuestionResponse={fn()}
			/>
		);
	},
};

const mockTallQuestion = {
	header: "Rollout",
	question: "How should the new billing export reach customers?",
	options: [
		{
			label: "Feature flag",
			description:
				"Ship the export dark behind a per-organization flag, enable it for internal organizations first, and widen the rollout once a full billing cycle has run without discrepancies.",
		},
		{
			label: "Beta program",
			description:
				"Offer the export to customers who opted into the beta program, collect feedback through the support portal for two weeks, and then promote it to general availability.",
		},
		{
			label: "General availability",
			description:
				"Enable the export for every organization in the next release, announce it in the changelog, and rely on the existing billing alerts to catch regressions after launch.",
		},
		{
			label: "Hold",
			description:
				"Keep the export unreleased until the finance team signs off on the reconciliation report format, and revisit the rollout plan in the next planning cycle.",
		},
	],
};

const MockTallAsyncQuestionResultMessage = buildQuestionResultMessage({
	...MockAsyncQuestionResult,
	questions: [mockTallQuestion],
});

// The question outgrows the dock's height cap, so the dock scrolls on its own
// and the chat input stays on screen.
export const AsyncQuestionDockedAboveInputMobile: Story = {
	render: () => {
		return (
			<AsyncQuestionChatPage
				store={buildAsyncQuestionStore([
					...MockAsyncQuestionMessages.slice(0, 2),
					MockTallAsyncQuestionResultMessage,
				])}
				className="flex h-175 w-[390px] flex-col"
				onSendAskUserQuestionResponse={fn()}
			/>
		);
	},
	parameters: { pixel: { matrix: pixelWithPhone } },
};

const shortScreenClassName =
	"flex h-[390px] w-[844px] flex-col overflow-hidden";

// On a short landscape screen the dock gives up height before the chat input
// does, so Send stays on screen.
export const AsyncQuestionDockedShortScreen: Story = {
	render: () => (
		<AsyncQuestionChatPage
			store={buildAsyncQuestionStore([
				...MockAsyncQuestionMessages.slice(0, 2),
				MockTallAsyncQuestionResultMessage,
			])}
			className={shortScreenClassName}
			onSendAskUserQuestionResponse={fn()}
		/>
	),
};

const MockTwoTallAsyncQuestionsResultMessage = buildQuestionResultMessage({
	...MockAsyncQuestionResult,
	questions: [
		mockTallQuestion,
		{
			...mockTallQuestion,
			header: "Self-hosted",
			question: "How should the export reach self-hosted deployments?",
		},
	],
});

export const AsyncQuestionDockedNextStepStartsAtTop: Story = {
	render: () => (
		<AsyncQuestionChatPage
			store={buildAsyncQuestionStore([
				...MockAsyncQuestionMessages.slice(0, 2),
				MockTwoTallAsyncQuestionsResultMessage,
			])}
			className={shortScreenClassName}
			onSendAskUserQuestionResponse={fn()}
		/>
	),
	play: async ({ canvasElement }) => {
		const dock = within(canvasElement).getByRole("region", {
			name: "Open question",
		});
		dock.scrollTop = dock.scrollHeight;
		await userEvent.click(within(dock).getByRole("button", { name: "Next" }));
	},
};

export const AsyncQuestionArchivedChat: Story = {
	render: () => (
		<AsyncQuestionChatPage store={buildAsyncQuestionStore()} isReadOnly />
	),
};

// The page withholds the answer callback while the chat input is disabled.
export const AsyncQuestionDockedDisabled: Story = {
	render: () => {
		return (
			<AsyncQuestionChatPage
				store={buildAsyncQuestionStore()}
				isInputDisabled
			/>
		);
	},
};

export const AsyncQuestionDockedSubmitError: Story = {
	render: () => (
		<AsyncQuestionChatPage
			store={buildAsyncQuestionStore()}
			onSendAskUserQuestionResponse={fn(async () => {
				throw new Error("Request failed with status code 500");
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const dock = within(
			within(canvasElement).getByRole("region", { name: "Open question" }),
		);
		await userEvent.click(dock.getByRole("button", { name: "Submit" }));
		await dock.findByRole("alert");
	},
};

const buildLiveAsyncQuestionStore = () => {
	const store = buildAsyncQuestionStore(MockWorkingMessages.slice(0, 1));
	store.setChatStatus("running");
	store.applyMessagePart({
		type: "tool-call",
		tool_call_id: "question",
		tool_name: "ask_user_question",
		args: {},
	});
	store.applyMessagePart({
		type: "tool-result",
		tool_call_id: "question",
		tool_name: "ask_user_question",
		result: MockAsyncQuestionResult,
	});
	return store;
};

// The question arrives on the live stream before its step persists.
export const AsyncQuestionDockedFromStream: Story = {
	render: () => (
		<AsyncQuestionChatPage
			store={buildLiveAsyncQuestionStore()}
			onSendAskUserQuestionResponse={fn()}
		/>
	),
};

// While the answer waits in the queue, the dock hides and the card shows the
// answer, also before the question's step persists.
export const AsyncQuestionFromStreamAnswerQueued: Story = {
	render: () => {
		const store = buildLiveAsyncQuestionStore();
		store.setQueuedMessages([MockQueuedQuestionAnswer]);
		return (
			<AsyncQuestionChatPage
				store={store}
				onSendAskUserQuestionResponse={fn()}
			/>
		);
	},
};

// Send now takes the answer out of the queue before the transcript has it,
// and the dock stays hidden until then.
export const AsyncQuestionAnswerPromoting: Story = {
	render: () => {
		const store = buildAsyncQuestionStore();
		store.setChatStatus("running");
		store.suppressQueuedMessageID(MockQueuedQuestionAnswer.id);
		store.holdPromotingQueuedMessage(MockQueuedQuestionAnswer);
		return (
			<AsyncQuestionChatPage
				store={store}
				onSendAskUserQuestionResponse={fn()}
			/>
		);
	},
};

export const AsyncQuestionDockedSecondStep: Story = {
	render: () => (
		<AsyncQuestionChatPage
			store={buildAsyncQuestionStore(MockTwoAsyncQuestionsMessages)}
			onSendAskUserQuestionResponse={fn()}
		/>
	),
	play: async ({ canvasElement }) => {
		const dock = within(
			within(canvasElement).getByRole("region", { name: "Open question" }),
		);
		await userEvent.click(dock.getByRole("button", { name: "Next" }));
	},
};

const MockBlockingQuestionResultMessage = buildQuestionResultMessage({
	questions: MockAsyncQuestionResult.questions,
});

export const BlockingQuestionStaysInline: Story = {
	render: () => (
		<AsyncQuestionChatPage
			store={buildAsyncQuestionStore([
				...MockAsyncQuestionMessages.slice(0, 2),
				MockBlockingQuestionResultMessage,
			])}
			onSendAskUserQuestionResponse={fn()}
		/>
	),
};
