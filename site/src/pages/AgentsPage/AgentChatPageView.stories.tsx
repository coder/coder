import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { type ComponentProps, type FC, useRef } from "react";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatDiffStatus, ChatMessagePart } from "#/api/typesGenerated";
import {
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withProxyProvider,
	withWebSocket,
} from "#/testHelpers/storybook";
import {
	AgentChatPageLoadingView,
	AgentChatPageNotFoundView,
	AgentChatPageView,
} from "./AgentChatPageView";
import {
	createChatStore,
	useChatSelector,
} from "./components/ChatConversation/chatStore";
import type { ModelSelectorOption } from "./components/ChatElements";
import type { ChatDetailError } from "./utils/usageLimitMessage";

// ---------------------------------------------------------------------------
// Shared constants & helpers
// ---------------------------------------------------------------------------
const AGENT_ID = "agent-detail-view-1";

const defaultModelConfigID = "model-config-1";

const defaultModelOptions: ModelSelectorOption[] = [
	{
		id: defaultModelConfigID,
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
];

const oneWeekAgo = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString();

const buildChat = (overrides: Partial<TypesGen.Chat> = {}): TypesGen.Chat => ({
	id: AGENT_ID,
	organization_id: "test-org-id",
	owner_id: "owner-1",
	title: "Help me refactor",
	status: "completed",
	last_model_config_id: defaultModelConfigID,
	mcp_server_ids: [],
	labels: {},
	created_at: oneWeekAgo,
	updated_at: oneWeekAgo,
	archived: false,
	pin_order: 0,
	has_unread: false,
	client_type: "ui",
	last_error: null,
	children: [],
	...overrides,
});

const buildEditing = (
	overrides: Partial<ComponentProps<typeof AgentChatPageView>["editing"]> = {},
) => ({
	chatInputRef: { current: null },
	editorInitialValue: "",
	initialEditorState: undefined,
	remountKey: 0,
	editingMessageId: null as number | null,
	editingFileBlocks: [] as readonly ChatMessagePart[],
	handleEditUserMessage: fn(),
	handleCancelHistoryEdit: fn(),
	editingQueuedMessageID: null,
	handleStartQueueEdit: fn(),
	handleCancelQueueEdit: fn(),
	handleSendFromInput: fn(),
	handleContentChange: fn(),
	...overrides,
});

const buildGitWatcher = (): ComponentProps<
	typeof AgentChatPageView
>["gitWatcher"] => ({
	repositories: new Map(),
	everDirty: new Set(),
	refresh: fn().mockReturnValue(true),
});

const agentsRouting = [
	{ path: "/agents/:agentId", useStoryElement: true },
	{ path: "/agents", useStoryElement: true },
] satisfies [
	{ path: string; useStoryElement: boolean },
	...{ path: string; useStoryElement: boolean }[],
];

// ---------------------------------------------------------------------------
// Wrapper component.
//
// Storybook's composeStory deep-merges meta.args into every story.
// When meta.args contains many fn() mocks, Maps, and closure-bound
// stores the merge hangs the browser. This wrapper builds fresh
// default props on each render, accepting only the overrides each
// story cares about.
// ---------------------------------------------------------------------------
type StoryProps = Omit<
	Partial<ComponentProps<typeof AgentChatPageView>>,
	"editing"
> & {
	editing?: Partial<ComponentProps<typeof AgentChatPageView>["editing"]>;
};

const StoryAgentChatPageView: FC<StoryProps> = ({ editing, ...overrides }) => {
	const defaultStoreRef = useRef(createChatStore());
	const defaultScrollContainerRef = useRef<HTMLDivElement | null>(null);
	const defaultScrollToBottomRef = useRef<(() => void) | null>(null);
	const store = overrides.store ?? defaultStoreRef.current;
	const messageCount = useChatSelector(
		store,
		(state) => state.messagesByID.size,
	);

	const props = {
		agentId: AGENT_ID,
		organizationId: "test-org-id",
		chatTitle: "Help me refactor",
		persistedError: undefined as ChatDetailError | undefined,
		parentChat: undefined as TypesGen.Chat | undefined,
		isArchived: false,
		effectiveSelectedModel: defaultModelConfigID,
		setSelectedModel: fn(),
		modelOptions: defaultModelOptions,
		modelSelectorPlaceholder: "Select a model",
		hasModelOptions: true,
		compressionThreshold: undefined as number | undefined,
		isInputDisabled: false,
		isSubmissionPending: false,
		isInterruptPending: false,
		isSidebarCollapsed: false,
		onToggleSidebarCollapsed: fn(),
		showSidebarPanel: false,
		onSetShowSidebarPanel: fn(),
		prNumber: undefined as number | undefined,
		diffStatusData: undefined as ComponentProps<
			typeof AgentChatPageView
		>["diffStatusData"],
		debugLoggingEnabled: false,
		gitWatcher: buildGitWatcher(),
		sshCommand: undefined as string | undefined,
		handleCommit: fn(),
		handleInterrupt: fn(),
		handleDeleteQueuedMessage: fn(),
		handlePromoteQueuedMessage: fn(),
		handleArchiveAgentAction: fn(),
		handleUnarchiveAgentAction: fn(),
		handleArchiveAndDeleteWorkspaceAction: fn(),
		handleRegenerateTitle: fn(),
		scrollContainerRef:
			overrides.scrollContainerRef ?? defaultScrollContainerRef,
		scrollToBottomRef: overrides.scrollToBottomRef ?? defaultScrollToBottomRef,
		hasMoreMessages: false,
		isFetchingMoreMessages: false,
		onFetchMoreMessages: fn(),
		mcpServers: [] as ComponentProps<typeof AgentChatPageView>["mcpServers"],
		selectedMCPServerIds: [] as ComponentProps<
			typeof AgentChatPageView
		>["selectedMCPServerIds"],
		onMCPSelectionChange: fn(),
		onMCPAuthComplete: fn(),
		...overrides,
		store,
		messageCount: overrides.messageCount ?? messageCount,
		editing: buildEditing(editing),
	};
	return <AgentChatPageView {...props} />;
};

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------
const meta: Meta<typeof AgentChatPageView> = {
	title: "pages/AgentsPage/AgentChatPageView",
	component: AgentChatPageView,
	decorators: [withAuthProvider, withDashboardProvider, withProxyProvider()],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		reactRouter: reactRouterParameters({
			location: {
				path: `/agents/${AGENT_ID}`,
				pathParams: { agentId: AGENT_ID },
			},
			routing: agentsRouting,
		}),
	},
};

export default meta;
type Story = StoryObj<typeof AgentChatPageView>;

// ---------------------------------------------------------------------------
// AgentChatPageView stories
// ---------------------------------------------------------------------------

/** Basic conversation view with a chat title, workspace, and no archive. */
export const Default: Story = {
	render: () => <StoryAgentChatPageView />,
};

/** Archived agent displays the read-only banner below the top bar. */
export const Archived: Story = {
	render: () => <StoryAgentChatPageView isArchived isInputDisabled />,
};

/** Shows the parent chat link in the top bar when a parent exists. */
export const WithParentChat: Story = {
	render: () => (
		<StoryAgentChatPageView
			parentChat={buildChat({ id: "parent-chat-1", title: "Root agent" })}
		/>
	),
};

/** Persisted error reason shown in the timeline area. */
export const WithError: Story = {
	render: () => (
		<StoryAgentChatPageView
			persistedError={{
				kind: "overloaded",
				message: "Anthropic is temporarily overloaded (HTTP 529).",
				provider: "anthropic",
				retryable: true,
				statusCode: 529,
			}}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("heading", { name: /service overloaded/i }),
		).toBeVisible();
		expect(
			canvas.getByText(/anthropic is temporarily overloaded \(http 529\)/i),
		).toBeVisible();
		expect(canvas.queryByText(/please try again/i)).not.toBeInTheDocument();
		expect(canvas.queryByText(/^retryable$/i)).not.toBeInTheDocument();
	},
};

/** Input area appears disabled when `isInputDisabled` is true. */
export const InputDisabled: Story = {
	render: () => <StoryAgentChatPageView isInputDisabled />,
};

/** Shows a sending/pending state for the input. */
export const SubmissionPending: Story = {
	render: () => <StoryAgentChatPageView isSubmissionPending />,
};

/** Right sidebar panel is open with diff status data. */
export const WithSidebarPanel: Story = {
	render: () => (
		<StoryAgentChatPageView
			showSidebarPanel
			prNumber={123}
			diffStatusData={
				{
					chat_id: AGENT_ID,
					url: "https://github.com/coder/coder/pull/123",
					pull_request_title: "fix: resolve race condition in workspace builds",
					pull_request_draft: false,
					changes_requested: false,
					additions: 42,
					deletions: 7,
					changed_files: 5,
				} satisfies ChatDiffStatus
			}
		/>
	),
	beforeEach: () => {
		spyOn(API.experimental, "getChatDiffContents").mockResolvedValue({
			chat_id: AGENT_ID,
			diff: `diff --git a/src/main.ts b/src/main.ts
index abc1234..def5678 100644
--- a/src/main.ts
+++ b/src/main.ts
@@ -1,3 +1,5 @@
 import { start } from "./server";
+import { logger } from "./logger";
 const port = 3000;
+logger.info("Starting server...");
 start(port);`,
		});
	},
};

/**
 * Clicking the refresh button in the git panel invalidates the
 * cached PR diff contents so that React Query re-fetches from
 * the server.
 */
export const RefreshInvalidatesPRDiff: Story = {
	render: () => (
		<StoryAgentChatPageView
			showSidebarPanel
			prNumber={123}
			diffStatusData={
				{
					chat_id: AGENT_ID,
					url: "https://github.com/coder/coder/pull/123",
					pull_request_title: "fix: resolve race condition in workspace builds",
					pull_request_draft: false,
					changes_requested: false,
					additions: 42,
					deletions: 7,
					changed_files: 5,
				} satisfies ChatDiffStatus
			}
		/>
	),
	beforeEach: () => {
		spyOn(API.experimental, "getChatDiffContents").mockResolvedValue({
			chat_id: AGENT_ID,
			diff: `diff --git a/src/main.ts b/src/main.ts
index abc1234..def5678 100644
--- a/src/main.ts
+++ b/src/main.ts
@@ -1,3 +1,5 @@
 import { start } from "./server";
+import { logger } from "./logger";
 const port = 3000;
+logger.info("Starting server...");
 start(port);`,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Wait for the initial diff fetch triggered by React Query.
		await waitFor(() => {
			expect(API.experimental.getChatDiffContents).toHaveBeenCalled();
		});
		const callsBefore = (
			API.experimental.getChatDiffContents as ReturnType<typeof fn>
		).mock.calls.length;

		// Click the refresh button in the git panel toolbar.
		const refreshButton = canvas.getByRole("button", { name: "Refresh" });
		await userEvent.click(refreshButton);

		// The query should be re-fetched, resulting in an additional call.
		await waitFor(() => {
			expect(
				(API.experimental.getChatDiffContents as ReturnType<typeof fn>).mock
					.calls.length,
			).toBeGreaterThan(callsBefore);
		});
	},
};

/** Left sidebar is collapsed. */
export const SidebarCollapsed: Story = {
	render: () => <StoryAgentChatPageView isSidebarCollapsed />,
};

/** No model options available — shows a disabled status message. */
export const NoModelOptions: Story = {
	render: () => (
		<StoryAgentChatPageView
			hasModelOptions={false}
			modelOptions={[]}
			isInputDisabled
		/>
	),
};

export const WithWorkspace: Story = {
	render: () => (
		<StoryAgentChatPageView
			workspace={MockWorkspace}
			workspaceAgent={MockWorkspaceAgent}
			sshCommand="ssh coder.workspace"
		/>
	),
};

// ---------------------------------------------------------------------------
// Workspace status pill stories
// ---------------------------------------------------------------------------

export const WorkspaceAgentStarting: Story = {
	render: () => (
		<StoryAgentChatPageView
			workspace={MockWorkspace}
			workspaceAgent={{
				...MockWorkspaceAgent,
				lifecycle_state: "starting",
			}}
		/>
	),
};

export const WorkspaceAgentCreated: Story = {
	render: () => (
		<StoryAgentChatPageView
			workspace={MockWorkspace}
			workspaceAgent={{
				...MockWorkspaceAgent,
				lifecycle_state: "created",
			}}
		/>
	),
};

export const WorkspaceAgentReady: Story = {
	render: () => (
		<StoryAgentChatPageView
			workspace={MockWorkspace}
			workspaceAgent={{
				...MockWorkspaceAgent,
				lifecycle_state: "ready",
			}}
		/>
	),
};

export const WorkspaceAgentStartError: Story = {
	render: () => (
		<StoryAgentChatPageView
			workspace={MockWorkspace}
			workspaceAgent={{
				...MockWorkspaceAgent,
				lifecycle_state: "start_error",
			}}
		/>
	),
};

export const WorkspaceAgentStartTimeout: Story = {
	render: () => (
		<StoryAgentChatPageView
			workspace={MockWorkspace}
			workspaceAgent={{
				...MockWorkspaceAgent,
				lifecycle_state: "start_timeout",
			}}
		/>
	),
};

export const WorkspaceNoAgent: Story = {
	render: () => <StoryAgentChatPageView workspace={MockWorkspace} />,
};

// ---------------------------------------------------------------------------
// AgentChatPageLoadingView stories
// ---------------------------------------------------------------------------

/** Default loading state with skeleton placeholders. */
export const Loading: Story = {
	render: () => (
		<AgentChatPageLoadingView
			titleElement={<title>Loading — Agents</title>}
			isInputDisabled
			effectiveSelectedModel={defaultModelConfigID}
			setSelectedModel={fn()}
			modelOptions={defaultModelOptions}
			modelSelectorPlaceholder="Select a model"
			hasModelOptions
			isSidebarCollapsed={false}
			onToggleSidebarCollapsed={fn()}
			showRightPanel={false}
		/>
	),
};

/** Loading state with the model selector populated. */
export const LoadingWithModelOptions: Story = {
	render: () => (
		<AgentChatPageLoadingView
			titleElement={<title>Loading — Agents</title>}
			isInputDisabled={false}
			effectiveSelectedModel={defaultModelConfigID}
			setSelectedModel={fn()}
			modelOptions={defaultModelOptions}
			modelSelectorPlaceholder="Select a model"
			hasModelOptions
			isSidebarCollapsed={false}
			onToggleSidebarCollapsed={fn()}
			showRightPanel={false}
		/>
	),
};
/** Loading state with the right panel pre-opened. */
export const LoadingWithRightPanel: Story = {
	render: () => (
		<AgentChatPageLoadingView
			titleElement={<title>Loading — Agents</title>}
			isInputDisabled
			effectiveSelectedModel={defaultModelConfigID}
			setSelectedModel={fn()}
			modelOptions={defaultModelOptions}
			modelSelectorPlaceholder="Select a model"
			hasModelOptions
			isSidebarCollapsed={false}
			onToggleSidebarCollapsed={fn()}
			showRightPanel
		/>
	),
};

/** Loading state with the left sidebar collapsed. */
export const LoadingSidebarCollapsed: Story = {
	render: () => (
		<AgentChatPageLoadingView
			titleElement={<title>Loading — Agents</title>}
			isInputDisabled
			effectiveSelectedModel={defaultModelConfigID}
			setSelectedModel={fn()}
			modelOptions={defaultModelOptions}
			modelSelectorPlaceholder="Select a model"
			hasModelOptions
			isSidebarCollapsed
			onToggleSidebarCollapsed={fn()}
			showRightPanel={false}
		/>
	),
};

// ---------------------------------------------------------------------------
// Helpers for seeding stores with messages
// ---------------------------------------------------------------------------

const buildMessage = (
	id: number,
	role: TypesGen.ChatMessageRole,
	text: string,
): TypesGen.ChatMessage => ({
	id,
	chat_id: AGENT_ID,
	created_at: new Date(Date.now() - (10 - id) * 60_000).toISOString(),
	role,
	content: [{ type: "text", text }],
});

const buildStoreWithMessages = (
	msgs: TypesGen.ChatMessage[],
	status: TypesGen.ChatStatus = "completed",
) => {
	const store = createChatStore();
	store.replaceMessages(msgs);
	store.setChatStatus(status);
	return store;
};

// ---------------------------------------------------------------------------
// Editing flow stories
// ---------------------------------------------------------------------------

const editingMessages = [
	buildMessage(1, "user", "Say hi back"),
	buildMessage(2, "assistant", "Hi!"),
	buildMessage(3, "user", "Now tell me a joke"),
	buildMessage(
		4,
		"assistant",
		"Why did the developer quit? Because they didn't get arrays.",
	),
	buildMessage(5, "user", "That was terrible, try again"),
];

/** Editing a message in the middle of the conversation — shows the warning
 *  border on the edited message, faded subsequent messages, and the editing
 *  banner + outline on the chat input. */
export const EditingMessage: Story = {
	render: () => (
		<StoryAgentChatPageView
			store={buildStoreWithMessages(editingMessages)}
			editing={{
				editingMessageId: 3,
				editorInitialValue: "Now tell me a joke",
			}}
		/>
	),
};

// ---------------------------------------------------------------------------
// AgentChatPageNotFoundView stories
// ---------------------------------------------------------------------------

/** Shows the "Chat not found" message. */
export const NotFound: Story = {
	render: () => (
		<AgentChatPageNotFoundView
			titleElement={<title>Not Found — Agents</title>}
			isSidebarCollapsed={false}
			onToggleSidebarCollapsed={fn()}
		/>
	),
};

/** "Chat not found" with the left sidebar collapsed. */
export const NotFoundSidebarCollapsed: Story = {
	render: () => (
		<AgentChatPageNotFoundView
			titleElement={<title>Not Found — Agents</title>}
			isSidebarCollapsed
			onToggleSidebarCollapsed={fn()}
		/>
	),
};

// ---------------------------------------------------------------------------
// Infinite scroll stories
// ---------------------------------------------------------------------------

/** Generate a long conversation so the scroll container overflows. */
const buildLongConversation = (count: number): TypesGen.ChatMessage[] => {
	const messages: TypesGen.ChatMessage[] = [];
	for (let i = 1; i <= count; i++) {
		const role: TypesGen.ChatMessageRole = i % 2 === 1 ? "user" : "assistant";
		const text =
			role === "user"
				? `Question ${Math.ceil(i / 2)}: Can you explain concept ${Math.ceil(i / 2)} in detail?`
				: `Sure! Here is a detailed explanation of concept ${Math.floor(i / 2)}. `.repeat(
						4,
					);
		messages.push(buildMessage(i, role, text));
	}
	return messages;
};

const scrollStoryDecorators: Decorator[] = [
	(Story) => (
		<div
			style={{
				height: "600px",
				display: "flex",
				flexDirection: "column",
			}}
		>
			<Story />
		</div>
	),
];

const waitForScrollOverflow = async (scrollContainer: HTMLElement) => {
	await waitFor(() => {
		expect(scrollContainer.scrollHeight).toBeGreaterThan(
			scrollContainer.clientHeight,
		);
	});
};

const scrollToHistoryTop = (scrollContainer: HTMLElement) => {
	// In the library's documented column-reverse layout, older history is
	// reached by driving the scroll offset toward the negative extreme.
	scrollContainer.scrollTop = -scrollContainer.scrollHeight;
	scrollContainer.dispatchEvent(new Event("scroll"));
};

const scrollToLatestMessages = (scrollContainer: HTMLElement) => {
	scrollContainer.scrollTop = 0;
	scrollContainer.dispatchEvent(new Event("scroll"));
};

const waitForFetchCount = async (
	fetchSpy: ReturnType<typeof fn>,
	count: number,
) => {
	await waitFor(() => {
		expect(fetchSpy).toHaveBeenCalledTimes(count);
	});
};

const waitForVisibleText = async (
	canvas: ReturnType<typeof within>,
	text: string,
) => {
	await waitFor(() => {
		// The chat timeline renders hidden measurement copies for some message
		// layouts, so pick any visible match instead of assuming the first node is
		// the one a user sees.
		const matches = canvas.queryAllByText(text);
		const hasVisibleMatch = matches.some((element: Element) => {
			const style = window.getComputedStyle(element);
			return (
				style.display !== "none" &&
				style.visibility !== "hidden" &&
				element.getClientRects().length > 0
			);
		});
		expect(hasVisibleMatch).toBe(true);
	});
};

const waitForIntersectionObserverTick = async () => {
	await new Promise<void>((resolve) => {
		requestAnimationFrame(() => {
			requestAnimationFrame(() => {
				resolve();
			});
		});
	});
};

/** Helper that extracts the current messages array from a store. */
const getStoreMessages = (
	store: ReturnType<typeof createChatStore>,
): TypesGen.ChatMessage[] => {
	const snapshot = store.getSnapshot();
	const messages: TypesGen.ChatMessage[] = [];
	for (const id of snapshot.orderedMessageIDs) {
		const message = snapshot.messagesByID.get(id);
		if (message) {
			messages.push(message);
		}
	}
	return messages;
};

const prependOlderMessages = (
	store: ReturnType<typeof createChatStore>,
	count: number,
) => {
	const existing = getStoreMessages(store);
	const oldestMessage = existing[0];
	const oldestID = oldestMessage?.id ?? 1;
	const olderMessages = Array.from({ length: count }, (_, index) => {
		const id = oldestID - count + index;
		const role: TypesGen.ChatMessageRole = id % 2 === 0 ? "assistant" : "user";
		const text =
			role === "user"
				? `Older question ${Math.abs(id)}.`
				: `Older answer ${Math.abs(id)}.`;
		return buildMessage(id, role, text);
	});
	store.replaceMessages([...olderMessages, ...existing]);
};

const resetScrollStoryStore = (
	store: ReturnType<typeof createChatStore>,
	// Default to a transcript long enough to overflow the 600px decorator so the
	// inverse-scroll stories exercise the fetch threshold immediately.
	count = 80,
) => {
	store.replaceMessages(buildLongConversation(count));
	store.setChatStatus("completed");
};

const inverseScrollStore = buildStoreWithMessages(buildLongConversation(80));
const inverseScrollFetchSpy = fn(() => {
	prependOlderMessages(inverseScrollStore, 10);
});

/**
 * Scrolling upward in the library's inverse mode loads older messages into the
 * top of the transcript.
 */
export const InverseScrollLoadsOlderMessages: Story = {
	parameters: { chromatic: { disableSnapshot: true } },
	decorators: scrollStoryDecorators,
	render: () => (
		<StoryAgentChatPageView
			store={inverseScrollStore}
			hasMoreMessages
			onFetchMoreMessages={inverseScrollFetchSpy}
		/>
	),
	play: async ({ canvasElement }) => {
		resetScrollStoryStore(inverseScrollStore);
		inverseScrollFetchSpy.mockClear();
		const canvas = within(canvasElement);
		const scrollContainer = canvas.getByTestId("scroll-container");

		await waitForScrollOverflow(scrollContainer);
		expect(inverseScrollFetchSpy).not.toHaveBeenCalled();

		scrollToHistoryTop(scrollContainer);

		await waitForFetchCount(inverseScrollFetchSpy, 1);
		await waitForVisibleText(canvas, "Older question 9.");
	},
};

const multiPageScrollStore = buildStoreWithMessages(buildLongConversation(80));
const multiPageFetchSpy = fn(() => {
	prependOlderMessages(multiPageScrollStore, 10);
});

/**
 * The library resets its one-shot load guard when dataLength changes, so a
 * second upward reveal can load another page.
 */
export const InverseScrollCanLoadMultiplePages: Story = {
	parameters: { chromatic: { disableSnapshot: true } },
	decorators: scrollStoryDecorators,
	render: () => (
		<StoryAgentChatPageView
			store={multiPageScrollStore}
			hasMoreMessages
			onFetchMoreMessages={multiPageFetchSpy}
		/>
	),
	play: async ({ canvasElement }) => {
		resetScrollStoryStore(multiPageScrollStore);
		multiPageFetchSpy.mockClear();
		const canvas = within(canvasElement);
		const scrollContainer = canvas.getByTestId("scroll-container");

		await waitForScrollOverflow(scrollContainer);

		scrollToHistoryTop(scrollContainer);
		await waitForFetchCount(multiPageFetchSpy, 1);
		await waitForVisibleText(canvas, "Older question 9.");

		scrollToLatestMessages(scrollContainer);
		await waitFor(() => {
			expect(scrollContainer.scrollTop).toBe(0);
		});
		await waitForIntersectionObserverTick();
		scrollToHistoryTop(scrollContainer);

		await waitForFetchCount(multiPageFetchSpy, 2);
		await waitForVisibleText(canvas, "Older answer 10.");
	},
};

const scrollToBottomButtonStoryStore = buildStoreWithMessages(
	buildLongConversation(80),
);

/**
 * The replacement container should keep the floating affordance that returns a
 * user from older history to the newest messages.
 */
export const ScrollToBottomButtonWorksWithInverseScroll: Story = {
	parameters: { chromatic: { disableSnapshot: true } },
	decorators: scrollStoryDecorators,
	render: () => (
		<StoryAgentChatPageView store={scrollToBottomButtonStoryStore} />
	),
	play: async ({ canvasElement }) => {
		resetScrollStoryStore(scrollToBottomButtonStoryStore);
		const canvas = within(canvasElement);
		const scrollContainer = canvas.getByTestId("scroll-container");

		await waitForScrollOverflow(scrollContainer);
		expect(
			canvas.queryByRole("button", { name: /scroll to bottom/i }),
		).toBeNull();

		scrollToHistoryTop(scrollContainer);

		await waitFor(() => {
			expect(
				canvas.getByRole("button", { name: /scroll to bottom/i }),
			).toBeVisible();
		});

		await userEvent.click(
			canvas.getByRole("button", { name: /scroll to bottom/i }),
		);

		await waitFor(() => {
			expect(scrollContainer.scrollTop).toBe(0);
			expect(
				canvas.queryByRole("button", { name: /scroll to bottom/i }),
			).toBeNull();
		});
	},
};

const scrollToBottomStoryStore = buildStoreWithMessages(
	buildLongConversation(80),
);
// Story objects live at module scope, so use a ref-shaped object instead of a
// hook to capture the imperative callback across the render and play phases.
const scrollToBottomStoryRef: { current: (() => void) | null } = {
	current: null,
};

/**
 * Page-level send and edit flows still rely on an imperative scroll-to-bottom
 * hook, so the replacement container must keep that contract working.
 */
export const ScrollToBottomRefStillWorks: Story = {
	parameters: { chromatic: { disableSnapshot: true } },
	decorators: scrollStoryDecorators,
	render: () => (
		<StoryAgentChatPageView
			store={scrollToBottomStoryStore}
			scrollToBottomRef={scrollToBottomStoryRef}
		/>
	),
	play: async ({ canvasElement }) => {
		resetScrollStoryStore(scrollToBottomStoryStore);
		const canvas = within(canvasElement);
		const scrollContainer = canvas.getByTestId("scroll-container");

		await waitForScrollOverflow(scrollContainer);
		scrollToHistoryTop(scrollContainer);

		await waitFor(() => {
			expect(scrollContainer.scrollTop).toBeLessThan(0);
			expect(typeof scrollToBottomStoryRef.current).toBe("function");
		});

		const scrollToBottom = scrollToBottomStoryRef.current;
		if (!scrollToBottom) {
			throw new Error("Expected scrollToBottomRef to be available.");
		}
		scrollToBottom();

		await waitFor(() => {
			expect(scrollContainer.scrollTop).toBe(0);
		});
	},
};

const messageOrderStore = buildStoreWithMessages([
	buildMessage(1, "user", "Oldest message"),
	buildMessage(2, "assistant", "Older response"),
	buildMessage(3, "user", "Newer question"),
	buildMessage(4, "assistant", "Newest reply"),
]);

/**
 * The reversed container layout must not invert the transcript's visible order.
 */
export const MessageOrderIsStillCorrect: Story = {
	parameters: { chromatic: { disableSnapshot: true } },
	decorators: scrollStoryDecorators,
	render: () => <StoryAgentChatPageView store={messageOrderStore} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const oldest = canvas.getByText("Oldest message");
		const newer = canvas.getByText("Newest reply");

		await waitFor(() => {
			expect(oldest.getBoundingClientRect().top).toBeLessThan(
				newer.getBoundingClientRect().top,
			);
		});
	},
};

/**
 * Selecting the Terminal tab in the sidebar must move keyboard focus into
 * the terminal so typing goes there, not the chat input.
 */
export const TerminalFocusOnTabSwitch: Story = {
	parameters: {
		chromatic: { disableSnapshot: true },
		webSocket: { "/api/v2/workspaceagents/": [{ event: "message", data: "" }] },
	},
	decorators: [withWebSocket],
	render: () => (
		<StoryAgentChatPageView
			showSidebarPanel
			workspace={MockWorkspace}
			workspaceAgent={MockWorkspaceAgent}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The sidebar should open on the Git tab by default.
		const terminalTab = await canvas.findByRole("tab", { name: "Terminal" });

		// 1. Click the Terminal tab.
		await userEvent.click(terminalTab);

		// Wait for the terminal container to appear.
		const terminalContainer = await waitFor(() => {
			const el = canvas.getByTestId("agents-sidebar-terminal");
			expect(el).toBeVisible();
			return el;
		});

		// The xterm focus target is a textarea inside the terminal container.
		await waitFor(
			() => {
				const textarea = terminalContainer.querySelector("textarea");
				expect(textarea).not.toBeNull();
				expect(document.activeElement).toBe(textarea);
			},
			{ timeout: 3000 },
		);

		// 2. Switch to Git, then back to Terminal.
		const gitTab = canvas.getByRole("tab", { name: "Git" });
		await userEvent.click(gitTab);
		await userEvent.click(terminalTab);

		// Focus should return to the terminal textarea.
		await waitFor(
			() => {
				const textarea = terminalContainer.querySelector("textarea");
				expect(textarea).not.toBeNull();
				expect(document.activeElement).toBe(textarea);
			},
			{ timeout: 3000 },
		);
	},
};
