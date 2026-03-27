import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps, FC } from "react";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatDiffStatus, ChatMessagePart } from "#/api/typesGenerated";
import type { ModelSelectorOption } from "#/components/ai-elements";
import { MockUserOwner } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import type { ChatDetailError } from "../utils/usageLimitMessage";
import { createChatStore } from "./AgentDetail/ChatContext";
import {
	AgentDetailLoadingView,
	AgentDetailNotFoundView,
	AgentDetailView,
} from "./AgentDetailView";

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
	last_error: null,
	...overrides,
});

const buildEditing = (
	overrides: Partial<ComponentProps<typeof AgentDetailView>["editing"]> = {},
) => ({
	chatInputRef: { current: null },
	editorInitialValue: "",
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
	typeof AgentDetailView
>["gitWatcher"] => ({
	repositories: new Map(),
	refresh: fn(),
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
	Partial<ComponentProps<typeof AgentDetailView>>,
	"editing"
> & {
	editing?: Partial<ComponentProps<typeof AgentDetailView>["editing"]>;
};

const StoryAgentDetailView: FC<StoryProps> = ({ editing, ...overrides }) => {
	const props = {
		agentId: AGENT_ID,
		chatTitle: "Help me refactor",
		persistedError: undefined as ChatDetailError | undefined,
		parentChat: undefined as TypesGen.Chat | undefined,
		isArchived: false,
		hasWorkspace: true,
		store: createChatStore(),
		pendingEditMessageId: null as number | null,
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
			typeof AgentDetailView
		>["diffStatusData"],
		gitWatcher: buildGitWatcher(),
		canOpenEditors: false,
		canOpenWorkspace: false,
		sshCommand: undefined as string | undefined,
		handleOpenInEditor: fn(),
		handleViewWorkspace: fn(),
		handleOpenTerminal: fn(),
		handleCommit: fn(),
		handleInterrupt: fn(),
		handleDeleteQueuedMessage: fn(),
		handlePromoteQueuedMessage: fn(),
		handleArchiveAgentAction: fn(),
		handleUnarchiveAgentAction: fn(),
		handleArchiveAndDeleteWorkspaceAction: fn(),
		handleRegenerateTitle: fn(),
		scrollContainerRef: { current: null },
		hasMoreMessages: false,
		isFetchingMoreMessages: false,
		onFetchMoreMessages: fn(),
		mcpServers: [] as ComponentProps<typeof AgentDetailView>["mcpServers"],
		selectedMCPServerIds: [] as ComponentProps<
			typeof AgentDetailView
		>["selectedMCPServerIds"],
		onMCPSelectionChange: fn(),
		onMCPAuthComplete: fn(),
		...overrides,
		editing: buildEditing(editing),
	};
	return <AgentDetailView {...props} />;
};

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------
const meta: Meta<typeof AgentDetailView> = {
	title: "pages/AgentsPage/AgentDetailView",
	component: AgentDetailView,
	decorators: [withAuthProvider, withDashboardProvider],
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
type Story = StoryObj<typeof AgentDetailView>;

// ---------------------------------------------------------------------------
// AgentDetailView stories
// ---------------------------------------------------------------------------

/** Basic conversation view with a chat title, workspace, and no archive. */
export const Default: Story = {
	render: () => <StoryAgentDetailView />,
};

/** Archived agent displays the read-only banner below the top bar. */
export const Archived: Story = {
	render: () => <StoryAgentDetailView isArchived isInputDisabled />,
};

/** Shows the parent chat link in the top bar when a parent exists. */
export const WithParentChat: Story = {
	render: () => (
		<StoryAgentDetailView
			parentChat={buildChat({ id: "parent-chat-1", title: "Root agent" })}
		/>
	),
};

/** Persisted error reason shown in the timeline area. */
export const WithError: Story = {
	render: () => (
		<StoryAgentDetailView
			persistedError={{
				kind: "overloaded",
				message: "Anthropic is currently overloaded.",
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
			canvas.getByText(/anthropic is currently overloaded\./i),
		).toBeVisible();
		expect(canvas.queryByText(/please try again/i)).not.toBeInTheDocument();
		expect(canvas.queryByText(/^retryable$/i)).not.toBeInTheDocument();
		expect(canvas.getByText(/http 529/i)).toBeVisible();
	},
};

/** Input area appears disabled when `isInputDisabled` is true. */
export const InputDisabled: Story = {
	render: () => <StoryAgentDetailView isInputDisabled />,
};

/** Shows a sending/pending state for the input. */
export const SubmissionPending: Story = {
	render: () => <StoryAgentDetailView isSubmissionPending />,
};

/** Right sidebar panel is open with diff status data. */
export const WithSidebarPanel: Story = {
	render: () => (
		<StoryAgentDetailView
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

/** Left sidebar is collapsed. */
export const SidebarCollapsed: Story = {
	render: () => <StoryAgentDetailView isSidebarCollapsed />,
};

/** No model options available — shows a disabled status message. */
export const NoModelOptions: Story = {
	render: () => (
		<StoryAgentDetailView
			hasModelOptions={false}
			modelOptions={[]}
			isInputDisabled
		/>
	),
};

/** Top bar has workspace action buttons visible. */
export const WithWorkspaceActions: Story = {
	render: () => (
		<StoryAgentDetailView
			canOpenEditors
			canOpenWorkspace
			sshCommand="ssh coder.workspace"
		/>
	),
};

// ---------------------------------------------------------------------------
// AgentDetailLoadingView stories
// ---------------------------------------------------------------------------

/** Default loading state with skeleton placeholders. */
export const Loading: Story = {
	render: () => (
		<AgentDetailLoadingView
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
		<AgentDetailLoadingView
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
		<AgentDetailLoadingView
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
		<AgentDetailLoadingView
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
		<StoryAgentDetailView
			store={buildStoreWithMessages(editingMessages)}
			editing={{
				editingMessageId: 3,
				editorInitialValue: "Now tell me a joke",
			}}
		/>
	),
};

/** The saving state while an edit is in progress — shows the pending
 *  indicator on the message being saved. */
export const EditingSaving: Story = {
	render: () => (
		<StoryAgentDetailView
			store={buildStoreWithMessages(editingMessages)}
			editing={{
				editingMessageId: 3,
				editorInitialValue: "Now tell me a better joke",
			}}
			pendingEditMessageId={3}
			isSubmissionPending
		/>
	),
};

// ---------------------------------------------------------------------------
// AgentDetailNotFoundView stories
// ---------------------------------------------------------------------------

/** Shows the "Chat not found" message. */
export const NotFound: Story = {
	render: () => (
		<AgentDetailNotFoundView
			titleElement={<title>Not Found — Agents</title>}
			isSidebarCollapsed={false}
			onToggleSidebarCollapsed={fn()}
		/>
	),
};

/** "Chat not found" with the left sidebar collapsed. */
export const NotFoundSidebarCollapsed: Story = {
	render: () => (
		<AgentDetailNotFoundView
			titleElement={<title>Not Found — Agents</title>}
			isSidebarCollapsed
			onToggleSidebarCollapsed={fn()}
		/>
	),
};

// ---------------------------------------------------------------------------
// Scroll-to-bottom button stories
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

const scrollAwayFromBottom = (scrollContainer: HTMLElement) => {
	scrollContainer.scrollTop = 0;
	scrollContainer.dispatchEvent(new Event("scroll"));
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

/** Scroll-to-bottom button appears after scrolling up in a long
 *  conversation, and clicking it returns to the bottom. */
export const ScrollToBottomButton: Story = {
	decorators: scrollStoryDecorators,
	render: () => (
		<StoryAgentDetailView
			store={buildStoreWithMessages(buildLongConversation(40))}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The button should be hidden initially — it has aria-hidden="true"
		// when not shown, so queryByRole correctly returns null.
		expect(
			canvas.queryByRole("button", { name: "Scroll to bottom" }),
		).toBeNull();

		// Find the scroll container via data-testid.
		const scrollContainer = canvas.getByTestId("scroll-container");

		// Wait for content to render and create overflow.
		await waitForScrollOverflow(scrollContainer);

		// Wait for the initial bottom pin to settle before scrolling away.
		await waitFor(
			() => {
				const dist =
					scrollContainer.scrollHeight -
					scrollContainer.scrollTop -
					scrollContainer.clientHeight;
				expect(dist).toBeLessThan(5);
			},
			{ timeout: 2000 },
		);
		await new Promise<void>((resolve) =>
			requestAnimationFrame(() => resolve()),
		);

		// Scroll to the top (away from bottom). In normal top-to-bottom
		// flow, scrollTop = 0 is at the top and the user is farthest
		// from the bottom of the conversation.
		scrollAwayFromBottom(scrollContainer);

		// Button should become visible (enters the accessibility tree).
		const button = await waitFor(() => {
			const btn = canvas.getByRole("button", { name: "Scroll to bottom" });
			expect(btn).toBeVisible();
			return btn;
		});

		// Click the button to scroll back to the bottom.
		await userEvent.click(button);

		// Button should be hidden again. The click handler immediately
		// hides it, so this doesn't depend on smooth scroll completing.
		await waitFor(() => {
			expect(
				canvas.queryByRole("button", { name: "Scroll to bottom" }),
			).toBeNull();
		});
	},
};

// Each scroll story that mutates the store in its play function
// creates the store at module scope so the play closure can reach
// it. Stories in a file execute sequentially, so there is no
// cross-contamination.
const preservedScrollStore = buildStoreWithMessages(buildLongConversation(30));

/** When scrolled away from bottom, new content preserves scroll position. */
export const ScrollPositionPreservedOnNewContent: Story = {
	decorators: scrollStoryDecorators,
	render: () => <StoryAgentDetailView store={preservedScrollStore} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scrollContainer = canvas.getByTestId("scroll-container");

		await waitForScrollOverflow(scrollContainer);

		// Wait for the initial bottom pin to settle before scrolling away.
		await waitFor(
			() => {
				const dist =
					scrollContainer.scrollHeight -
					scrollContainer.scrollTop -
					scrollContainer.clientHeight;
				expect(dist).toBeLessThan(5);
			},
			{ timeout: 2000 },
		);
		await new Promise<void>((resolve) =>
			requestAnimationFrame(() => resolve()),
		);

		// Scroll away from bottom.
		scrollAwayFromBottom(scrollContainer);

		// Wait for the button to confirm we are away from the bottom.
		await waitFor(
			() => {
				expect(
					canvas.getByRole("button", { name: "Scroll to bottom" }),
				).toBeVisible();
			},
			{ timeout: 2000 },
		);

		// Record position while clearly away from the bottom.
		const distFromBottom =
			scrollContainer.scrollHeight -
			scrollContainer.scrollTop -
			scrollContainer.clientHeight;
		expect(distFromBottom).toBeGreaterThan(50);

		const existing = getStoreMessages(preservedScrollStore);
		preservedScrollStore.replaceMessages(
			existing.concat([
				buildMessage(
					31,
					"user",
					"Follow-up question about the implementation.",
				),
				buildMessage(
					32,
					"assistant",
					"Here is a detailed response about the implementation details you asked about.",
				),
			]),
		);

		// Wait for ResizeObserver + RAF compensation to settle.
		// We should remain significantly away from the bottom.
		await waitFor(
			() => {
				const dist =
					scrollContainer.scrollHeight -
					scrollContainer.scrollTop -
					scrollContainer.clientHeight;
				expect(dist).toBeGreaterThan(50);
			},
			{ timeout: 2000 },
		);

		expect(
			canvas.getByRole("button", { name: "Scroll to bottom" }),
		).toBeVisible();
	},
};

const pinnedScrollStore = buildStoreWithMessages(buildLongConversation(30));

/** When at bottom, new content keeps the user pinned to bottom. */
export const ScrollPinnedToBottomOnNewContent: Story = {
	decorators: scrollStoryDecorators,
	render: () => <StoryAgentDetailView store={pinnedScrollStore} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scrollContainer = canvas.getByTestId("scroll-container");

		await waitForScrollOverflow(scrollContainer);

		// Wait for the initial bottom pin (double-RAF) to settle.
		await waitFor(
			() => {
				const dist =
					scrollContainer.scrollHeight -
					scrollContainer.scrollTop -
					scrollContainer.clientHeight;
				expect(dist).toBeLessThan(5);
			},
			{ timeout: 2000 },
		);
		expect(
			canvas.queryByRole("button", { name: "Scroll to bottom" }),
		).toBeNull();

		const existing = getStoreMessages(pinnedScrollStore);
		pinnedScrollStore.replaceMessages(
			existing.concat([
				buildMessage(31, "user", "Another question."),
				buildMessage(32, "assistant", "Here is the answer with full details."),
				buildMessage(33, "user", "Thanks, one more thing."),
				buildMessage(
					34,
					"assistant",
					"Sure, here is the additional information you requested.",
				),
			]),
		);

		// Wait for the double-RAF pin to complete.
		await waitFor(
			() => {
				const dist =
					scrollContainer.scrollHeight -
					scrollContainer.scrollTop -
					scrollContainer.clientHeight;
				expect(dist).toBeLessThan(5);
			},
			{ timeout: 2000 },
		);

		expect(
			canvas.queryByRole("button", { name: "Scroll to bottom" }),
		).toBeNull();
	},
};
