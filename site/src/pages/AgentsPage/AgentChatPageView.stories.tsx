import { MessageScroller } from "@shadcn/react/message-scroller";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { type ComponentProps, type FC, useRef, useState } from "react";
import {
	expect,
	fireEvent,
	fn,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import type { ChatDiffStatus, ChatMessagePart } from "#/api/typesGenerated";
import { AGENT_BROWSER_APP_SLUG } from "#/modules/apps/apps";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockDefaultOrganization,
	MockGroup,
	MockOrganizationMember,
	MockOrganizationMember2,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceResource,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withProxyProvider,
	withWebSocket,
} from "#/testHelpers/storybook";
import { docs } from "#/utils/docs";
import {
	AgentChatPageLoadingView,
	AgentChatPageNotFoundView,
	AgentChatPageView,
} from "./AgentChatPageView";
import type { ChatDetailError } from "./components/ChatConversation/chatError";
import { createChatStore } from "./components/ChatConversation/chatStore";
import { buildLongConversation } from "./components/ChatConversation/storyFixtures";
import type { ModelSelectorOption } from "./components/ChatElements";
import { lastActiveSidebarTabStorageKeyPrefix } from "./utils/sidebarTabStorage";

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
	...MockChat,
	id: AGENT_ID,
	owner_id: "owner-1",
	owner_username: "owner",
	owner_name: "Owner",
	title: "Help me refactor",
	last_model_config_id: defaultModelConfigID,
	created_at: oneWeekAgo,
	updated_at: oneWeekAgo,
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
	handleSendFromInput: fn(),
	handleContentChange: fn(),
	...overrides,
});

const buildGitWatcher = (): ComponentProps<
	typeof AgentChatPageView
>["gitWatcher"] => ({
	repositories: new Map(),
	everDirty: new Set(),
	hasReceivedChanges: true,
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
	const store = overrides.store ?? defaultStoreRef.current;

	const props = {
		agentId: AGENT_ID,
		sendShortcut: "enter" as const,
		organizationId: "test-org-id",
		chatTitle: "Help me refactor",
		persistedError: undefined as ChatDetailError | undefined,
		parentChat: undefined as TypesGen.Chat | undefined,
		isArchived: false,
		isSharedChat: false,
		chatOwner: undefined as ComponentProps<
			typeof AgentChatPageView
		>["chatOwner"],
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
		hasMoreMessages: false,
		isFetchingMoreMessages: false,
		isHydratingMessages: false,
		hasFetchMoreError: false,
		onFetchMoreMessages: fn(async () => {}),
		mcpServers: [] as ComponentProps<typeof AgentChatPageView>["mcpServers"],
		selectedMCPServerIds: [] as ComponentProps<
			typeof AgentChatPageView
		>["selectedMCPServerIds"],
		onMCPSelectionChange: fn(),
		onMCPAuthComplete: fn(),
		canShareChat: false,
		canConfigureAgentSetup: true,
		providerCount: 1,
		modelCount: 1,
		initialChatStatus: "waiting" as const,
		initialMessages: [],
		...overrides,
		store,
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
	// Summary is the default tab and reads the chat, so mock it for the sidebar.
	// Cost needs no mock: these stories leave the aibridge feature off, so the
	// summary panel never requests it.
	beforeEach: () => {
		spyOn(API.experimental, "getChat").mockResolvedValue(buildChat());
	},
	decorators: [
		(Story) => (
			<MessageScroller.Provider autoScroll defaultScrollPosition="end">
				<Story />
			</MessageScroller.Provider>
		),
		withAuthProvider,
		withDashboardProvider,
		withProxyProvider(),
	],
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.queryByText(/^This chat is owned by/),
		).not.toBeInTheDocument();
	},
};

/** Archived agent displays the read-only banner below the top bar. */
export const Archived: Story = {
	render: () => <StoryAgentChatPageView isArchived isInputDisabled />,
};

export const OtherUserChatReadOnly: Story = {
	render: () => (
		<StoryAgentChatPageView
			chatOwner={{ username: "OtherUser", name: "Other User" }}
			isInputDisabled
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const banner = canvas.getByText(
			"This chat is owned by Other User. It is read-only.",
		);
		expect(banner).toBeVisible();
		expect(banner).toHaveAttribute("role", "status");
		expect(canvas.getByLabelText("Chat message")).toHaveAttribute(
			"aria-disabled",
			"true",
		);
	},
};

export const OtherUserChatUsernameFallback: Story = {
	render: () => (
		<StoryAgentChatPageView
			chatOwner={{ username: "OtherUser" }}
			isInputDisabled
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const banner = canvas.getByText(
			"This chat is owned by @OtherUser. It is read-only.",
		);
		expect(banner).toBeVisible();
		expect(banner).toHaveAttribute("role", "status");
		expect(canvas.getByLabelText("Chat message")).toHaveAttribute(
			"aria-disabled",
			"true",
		);
	},
};

export const OtherUserChatOwnerFallback: Story = {
	render: () => <StoryAgentChatPageView chatOwner={{}} isInputDisabled />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const banner = canvas.getByText(
			"This chat is owned by another user. It is read-only.",
		);
		expect(banner).toBeVisible();
		expect(banner).toHaveAttribute("role", "status");
		expect(canvas.getByLabelText("Chat message")).toHaveAttribute(
			"aria-disabled",
			"true",
		);
	},
};

/** Archived chats stay read-only without the owner banner. */
export const ArchivedOtherUserChat: Story = {
	render: () => (
		<StoryAgentChatPageView
			isArchived
			isInputDisabled
			chatOwner={{ username: "OtherUser" }}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.queryByText(/^This chat is owned by/),
		).not.toBeInTheDocument();
		expect(
			canvas.getByText("This agent has been archived and is read-only."),
		).toBeVisible();
	},
};

export const QueuedForCapacityCommunityAdmin: Story = {
	parameters: {
		permissions: { viewAllLicenses: true },
	},
	render: () => <StoryAgentChatPageView queuedForCapacity />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const callout = within(canvas.getByRole("alert"));
		const message = callout.getByText(
			/reached the Community license limit for active agents/,
		);
		expect(message).toBeVisible();
		expect(message).toHaveTextContent(
			"This agent is queued and will start automatically when capacity is available.",
		);
		const trialLink = canvas.getByRole("link", {
			name: /start an unlimited trial/i,
		});
		expect(trialLink).toHaveAttribute("href", "/deployment/premium");
		const learnMoreLink = canvas.getByRole("link", { name: /learn more/i });
		expect(learnMoreLink).toHaveAttribute(
			"href",
			docs("/ai-coder/agents/platform-controls#concurrent-agents"),
		);
	},
};

export const QueuedForCapacityCommunityMember: Story = {
	render: () => <StoryAgentChatPageView queuedForCapacity />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const message = canvas.getByText(
			/reached the Community license limit for active agents/,
		);
		expect(message).toBeVisible();
		expect(
			canvas.queryByRole("link", { name: /start an unlimited trial/i }),
		).not.toBeInTheDocument();
		const learnMoreLink = canvas.getByRole("link", { name: /learn more/i });
		expect(learnMoreLink).toHaveAttribute(
			"href",
			docs("/ai-coder/agents/platform-controls#concurrent-agents"),
		);
	},
};

export const QueuedForCapacityPremiumAdmin: Story = {
	parameters: {
		features: ["multiple_organizations"],
		permissions: { viewAllLicenses: true },
	},
	render: () => <StoryAgentChatPageView queuedForCapacity />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const message = canvas.getByText(
			/reached your license’s limit for active agents/,
		);
		expect(message).toBeVisible();
		expect(message).toHaveTextContent(
			"Contact your Coder account team or sales@coder.com to upgrade to unlimited concurrent agents.",
		);
		const salesLink = canvas.getByRole("link", { name: /sales@coder\.com/ });
		expect(salesLink).toHaveAttribute("href", "mailto:sales@coder.com");
		expect(
			canvas.queryByRole("link", { name: /learn more/i }),
		).not.toBeInTheDocument();
	},
};

export const QueuedForCapacityPremiumMember: Story = {
	parameters: {
		features: ["multiple_organizations"],
	},
	render: () => <StoryAgentChatPageView queuedForCapacity />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const message = canvas.getByText(
			/reached your license’s limit for active agents/,
		);
		expect(message).toBeVisible();
		expect(
			canvas.queryByRole("link", { name: /sales@coder\.com/ }),
		).not.toBeInTheDocument();
		const learnMoreLink = canvas.getByRole("link", { name: /learn more/i });
		expect(learnMoreLink).toHaveAttribute(
			"href",
			docs("/ai-coder/agents/platform-controls#concurrent-agents"),
		);
	},
};

export const QueuedForCapacityPremiumHardLimit: Story = {
	parameters: {
		features: [
			"multiple_organizations",
			{
				name: "agent_runtime_hours",
				limit: 3000,
				hard_limit: 4000,
				actual: 4000,
			},
		],
		permissions: { viewAllLicenses: true },
	},
	render: () => <StoryAgentChatPageView queuedForCapacity />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const message = canvas.getByText(
			/reached the 4000-hour Agent Hours hard limit/,
		);
		expect(message).toBeVisible();
		expect(message).toHaveTextContent(
			"This agent is queued and will start automatically when capacity is available.",
		);
		const salesLink = canvas.getByRole("link", { name: /sales@coder\.com/ });
		expect(salesLink).toHaveAttribute("href", "mailto:sales@coder.com");
	},
};

export const NotQueuedForCapacity: Story = {
	render: () => <StoryAgentChatPageView />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.queryByText(/limit for active agents/),
		).not.toBeInTheDocument();
	},
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
				message: "Anthropic is temporarily overloaded.",
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
			canvas.getByText(/anthropic is temporarily overloaded\./i),
		).toBeVisible();
		expect(canvas.getByText(/^HTTP 529$/)).toBeVisible();
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

export const NarrowWithSidebarPanel: Story = {
	render: () => <StoryAgentChatPageView showSidebarPanel />,
	decorators: [
		(Story) => (
			<div
				data-testid="narrow-agents-layout"
				style={{
					display: "flex",
					height: "100vh",
					overflow: "hidden",
					width: 1024,
				}}
			>
				<div style={{ minWidth: 320, width: 320 }} />
				<div style={{ display: "flex", flex: 1, minWidth: 0 }}>
					<Story />
				</div>
			</div>
		),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const layout = await canvas.findByTestId("narrow-agents-layout");
		const chatPanel = await canvas.findByTestId("agents-chat-panel");
		const rightPanel = await canvas.findByTestId("agents-right-panel");
		const composer = await canvas.findByTestId("chat-composer");
		const sendButton = canvas.getByRole("button", { name: "Send" });

		await waitFor(() => {
			const layoutRect = layout.getBoundingClientRect();
			const chatPanelRect = chatPanel.getBoundingClientRect();
			const rightPanelRect = rightPanel.getBoundingClientRect();
			const composerRect = composer.getBoundingClientRect();
			const sendButtonRect = sendButton.getBoundingClientRect();

			expect(chatPanelRect.width).toBeGreaterThanOrEqual(359);
			expect(sendButtonRect.left).toBeGreaterThanOrEqual(composerRect.left);
			expect(sendButtonRect.right).toBeLessThanOrEqual(composerRect.right);
			expect(rightPanelRect.right).toBeLessThanOrEqual(layoutRect.right + 1);
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

		// Summary is the default tab; switch to Git to view the PR diff.
		await userEvent.click(canvas.getByRole("tab", { name: "Git" }));

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

export const MissingProviderAndModelSetup: Story = {
	render: () => (
		<StoryAgentChatPageView
			canConfigureAgentSetup
			providerCount={0}
			modelCount={0}
			hasModelOptions={false}
			modelOptions={[]}
			isInputDisabled
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await waitFor(() => {
			expect(
				canvas.getAllByText((_content, element) => {
					return (
						element?.textContent ===
						"To chat with Coder Agents, set up a provider then add a model."
					);
				})[0],
			).toBeVisible();
		});
		expect(canvas.getByRole("link", { name: "provider" })).toHaveAttribute(
			"href",
			"/ai/settings/providers",
		);
		expect(canvas.getByRole("link", { name: "model" })).toHaveAttribute(
			"href",
			"/ai/settings/models",
		);
	},
};

export const MissingModelSetup: Story = {
	render: () => (
		<StoryAgentChatPageView
			canConfigureAgentSetup
			providerCount={1}
			modelCount={0}
			hasModelOptions={false}
			modelOptions={[]}
			isInputDisabled
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await waitFor(() => {
			expect(
				canvas.getAllByText((_content, element) => {
					return (
						element?.textContent ===
						"To chat with Coder Agents, set up a model."
					);
				})[0],
			).toBeVisible();
		});
		expect(canvas.getByRole("link", { name: "model" })).toHaveAttribute(
			"href",
			"/ai/settings/models",
		);
	},
};

export const MissingProviderSetup: Story = {
	render: () => (
		<StoryAgentChatPageView
			canConfigureAgentSetup
			providerCount={0}
			modelCount={1}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await waitFor(() => {
			expect(
				canvas.getAllByText((_content, element) => {
					return (
						element?.textContent ===
						"To chat with Coder Agents, set up a provider."
					);
				})[0],
			).toBeVisible();
		});
		expect(canvas.getByRole("link", { name: "provider" })).toHaveAttribute(
			"href",
			"/ai/settings/providers",
		);
	},
};

export const MemberNoModelsAvailable: Story = {
	render: () => (
		<StoryAgentChatPageView
			canConfigureAgentSetup={false}
			providerCount={0}
			modelCount={0}
			hasModelOptions={false}
			modelOptions={[]}
			isInputDisabled
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await waitFor(() => {
			expect(
				canvas.getByText(
					"AI models aren't available yet. Your admin is still getting things set up.",
				),
			).toBeVisible();
		});
	},
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
	render: () => (
		<StoryAgentChatPageView
			workspace={MockWorkspace}
			workspaceOptions={[MockWorkspace]}
			selectedWorkspaceId={MockWorkspace.id}
			onWorkspaceChange={fn()}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", {
				name: `Remove workspace ${MockWorkspace.name}`,
			}),
		).toBeVisible();
	},
};

// ---------------------------------------------------------------------------
// AgentChatPageLoadingView stories
// ---------------------------------------------------------------------------

/** Default loading state with skeleton placeholders. */
export const Loading: Story = {
	render: () => (
		<AgentChatPageLoadingView
			sendShortcut="enter"
			titleElement={<title>Loading — Agents</title>}
			inputRef={{ current: null }}
			initialValue=""
			initialEditorState={undefined}
			remountKey={0}
			onContentChange={fn()}
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
			sendShortcut="enter"
			titleElement={<title>Loading — Agents</title>}
			inputRef={{ current: null }}
			initialValue=""
			initialEditorState={undefined}
			remountKey={0}
			onContentChange={fn()}
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
			sendShortcut="enter"
			titleElement={<title>Loading — Agents</title>}
			inputRef={{ current: null }}
			initialValue=""
			initialEditorState={undefined}
			remountKey={0}
			onContentChange={fn()}
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
			sendShortcut="enter"
			titleElement={<title>Loading — Agents</title>}
			inputRef={{ current: null }}
			initialValue=""
			initialEditorState={undefined}
			remountKey={0}
			onContentChange={fn()}
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

const buildMessageWithContent = (
	id: number,
	role: TypesGen.ChatMessageRole,
	content: TypesGen.ChatMessagePart[],
): TypesGen.ChatMessage => ({
	id,
	chat_id: AGENT_ID,
	created_at: new Date(Date.now() - (10 - id) * 60_000).toISOString(),
	role,
	content,
});

const buildMessage = (
	id: number,
	role: TypesGen.ChatMessageRole,
	text: string,
): TypesGen.ChatMessage =>
	buildMessageWithContent(id, role, [{ type: "text", text }]);

const buildStoreWithMessages = (
	msgs: TypesGen.ChatMessage[],
	status: TypesGen.ChatStatus = "waiting",
) => {
	const store = createChatStore();
	store.replaceMessages(msgs);
	store.setChatStatus(status);
	return store;
};

const otherUserActionMessages: TypesGen.ChatMessage[] = [
	buildMessage(1, "user", "Please review this plan."),
	buildMessageWithContent(2, "assistant", [
		{ type: "text", text: "I prepared a plan." },
		{
			type: "tool-call",
			tool_call_id: "other-user-plan",
			tool_name: "propose_plan",
			args: { path: "/home/coder/PLAN.md" },
		},
		{
			type: "tool-result",
			tool_call_id: "other-user-plan",
			tool_name: "propose_plan",
			result: {
				file_id: "other-user-plan-file",
				content: "# Plan\n\n1. Keep this chat read-only.",
			},
		},
	]),
];

export const OtherUserChatHidesInlineActions: Story = {
	render: () => (
		<StoryAgentChatPageView
			chatOwner={{ username: "OtherUser", name: "Other User" }}
			isInputDisabled
			onImplementPlan={fn()}
			store={buildStoreWithMessages(otherUserActionMessages)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText("This chat is owned by Other User. It is read-only."),
		).toBeVisible();
		expect(await canvas.findByText("Please review this plan.")).toBeVisible();
		expect(
			canvas.queryByRole("button", { name: "Edit message" }),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Implement plan" }),
		).not.toBeInTheDocument();
	},
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
// Transcript scrolling stories
// ---------------------------------------------------------------------------

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

const getViewport = (canvas: ReturnType<typeof within>) =>
	canvas.getByRole("region", { name: "Messages" });

const waitForScrollOverflow = async (viewport: HTMLElement) => {
	await waitFor(() => {
		expect(viewport.scrollHeight).toBeGreaterThan(viewport.clientHeight);
	});
};

/** The scroller commits its state on an animation frame. */
const settleScroller = async () => {
	await new Promise<void>((resolve) => {
		requestAnimationFrame(() => {
			requestAnimationFrame(() => {
				resolve();
			});
		});
	});
};

const scrollTo = (viewport: HTMLElement, scrollTop: number) => {
	viewport.scrollTop = scrollTop;
	fireEvent.scroll(viewport);
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
	const oldestID = existing[0]?.id ?? 1;
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

const singleRenderStore = buildStoreWithMessages([
	buildMessage(1, "user", "Only rendered once"),
	buildMessage(2, "assistant", "Understood."),
]);

/** Every prompt renders exactly one row: no sticky copy trails the original. */
export const UserPromptsRenderOnce: Story = {
	parameters: { pixel: { exclude: true } },
	decorators: scrollStoryDecorators,
	render: () => <StoryAgentChatPageView store={singleRenderStore} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getAllByText("Only rendered once")).toHaveLength(1);
		expect(canvas.getAllByTestId("chat-message-message:1")).toHaveLength(1);
	},
};

const startEdgeStore = buildStoreWithMessages(
	buildLongConversation(AGENT_ID, 40).slice(1),
);

const streamCompletionStore = buildStoreWithMessages(
	buildLongConversation(AGENT_ID, 40),
);

let releaseStartEdgeFetch: (() => void) | undefined;
let startEdgeFetchGate: Promise<void>;
const startEdgeFetchSpy = fn(async () => {
	await startEdgeFetchGate;
	prependOlderMessages(startEdgeStore, 10);
});

/** Reaching the start of the loaded history requests the previous page once. */
export const ReachingTheStartLoadsEarlierMessages: Story = {
	parameters: { pixel: { exclude: true } },
	decorators: scrollStoryDecorators,
	beforeEach: () => {
		startEdgeFetchGate = new Promise<void>((resolve) => {
			releaseStartEdgeFetch = resolve;
		});
	},
	render: () => (
		<StoryAgentChatPageView
			store={startEdgeStore}
			hasMoreMessages
			onFetchMoreMessages={startEdgeFetchSpy}
		/>
	),
	play: async ({ canvasElement }) => {
		startEdgeStore.replaceMessages(
			buildLongConversation(AGENT_ID, 40).slice(1),
		);
		startEdgeStore.setChatStatus("waiting");
		startEdgeFetchSpy.mockClear();
		const canvas = within(canvasElement);
		const viewport = getViewport(canvas);

		await waitForScrollOverflow(viewport);
		await settleScroller();
		expect(startEdgeFetchSpy).not.toHaveBeenCalled();

		const marker = canvas.getByTestId("chat-message-message:2");
		fireEvent.wheel(viewport, { deltaY: -100 });
		scrollTo(viewport, 0);
		await waitFor(() => {
			expect(startEdgeFetchSpy).toHaveBeenCalledTimes(1);
		});
		const markerOffset =
			marker.getBoundingClientRect().top - viewport.getBoundingClientRect().top;
		releaseStartEdgeFetch?.();

		await waitFor(() => {
			expect(canvas.getByTestId("chat-message-message:1")).toBeInTheDocument();
		});
		await settleScroller();
		expect(
			Math.abs(
				marker.getBoundingClientRect().top -
					viewport.getBoundingClientRect().top -
					markerOffset,
			),
		).toBeLessThan(4);
	},
};

/**
 * Stream completion must not yank the transcript to the oldest message. The
 * live assistant row is replaced by its durable row (a remount, not an
 * in-place reconciliation), and the only anchored row is the active turn's
 * prompt, so the scroller has no unhandled historical anchor to jump to.
 */
export const StreamCompletionKeepsViewportPosition: Story = {
	parameters: { pixel: { exclude: true } },
	decorators: scrollStoryDecorators,
	render: () => <StoryAgentChatPageView store={streamCompletionStore} />,
	play: async ({ canvasElement }) => {
		streamCompletionStore.replaceMessages(buildLongConversation(AGENT_ID, 40));
		streamCompletionStore.setChatStatus("waiting");
		const canvas = within(canvasElement);
		const viewport = getViewport(canvas);

		await waitForScrollOverflow(viewport);
		await settleScroller();

		// Sit at the live edge; the oldest message is scrolled above the viewport.
		scrollTo(viewport, viewport.scrollHeight);
		await settleScroller();
		const oldest = canvas.getByTestId("chat-message-message:1");
		const oldestAboveViewport = () =>
			oldest.getBoundingClientRect().bottom <=
			viewport.getBoundingClientRect().top;
		expect(oldestAboveViewport()).toBe(true);

		// A new turn begins: the prompt is appended and the assistant starts
		// streaming into the live row.
		streamCompletionStore.batch(() => {
			streamCompletionStore.upsertDurableMessages([
				buildMessage(41, "user", "Final question."),
			]);
			streamCompletionStore.setChatStatus("running");
			streamCompletionStore.applyMessageParts([
				{ type: "text", text: "Streaming the final answer." },
			]);
		});
		// The live assistant row mounts under its ephemeral key.
		await waitFor(() => {
			expect(
				canvas.getByTestId("chat-message-live-assistant"),
			).toBeInTheDocument();
		});
		await settleScroller();

		// The durable assistant row replaces the live row, then the turn ends.
		streamCompletionStore.batch(() => {
			streamCompletionStore.upsertDurableMessages([
				buildMessage(42, "assistant", "Streaming the final answer."),
			]);
			streamCompletionStore.clearStreamState();
			streamCompletionStore.setChatStatus("waiting");
		});
		await waitFor(() => {
			expect(canvas.getByTestId("chat-message-message:42")).toBeInTheDocument();
		});
		await settleScroller();

		// The viewport never jumped back to the oldest message.
		expect(oldestAboveViewport()).toBe(true);
	},
};

const thinkingShiftStore = buildStoreWithMessages(
	buildLongConversation(AGENT_ID, 40),
);

/**
 * The Thinking indicator must hand off to streaming text without collapsing
 * the live row for a frame. The anchored prompt must not move.
 */
export const ThinkingHandoffKeepsPromptPosition: Story = {
	parameters: { pixel: { exclude: true } },
	decorators: scrollStoryDecorators,
	render: () => <StoryAgentChatPageView store={thinkingShiftStore} />,
	play: async ({ canvasElement }) => {
		thinkingShiftStore.replaceMessages(buildLongConversation(AGENT_ID, 40));
		thinkingShiftStore.setChatStatus("waiting");
		const canvas = within(canvasElement);
		const viewport = getViewport(canvas);
		await waitForScrollOverflow(viewport);
		await settleScroller();
		scrollTo(viewport, viewport.scrollHeight);
		await settleScroller();

		// Begin a turn: the prompt is appended and the chat goes running, so
		// the live row shows the Thinking indicator with no stream output yet.
		thinkingShiftStore.batch(() => {
			thinkingShiftStore.upsertDurableMessages([
				buildMessage(41, "user", "Follow-up question."),
			]);
			thinkingShiftStore.setChatStatus("running");
		});
		await canvas.findByTestId("chat-message-live-assistant");
		await canvas.findByTestId("live-activity-slot");
		await settleScroller();

		const prompt = canvas.getByTestId("chat-message-message:41");
		const liveRow = canvas.getByTestId("chat-message-live-assistant");
		const promptTop = () =>
			prompt.getBoundingClientRect().top - viewport.getBoundingClientRect().top;
		// Capture the baseline while the Thinking indicator is shown: the bug
		// shrank the live row below this for one frame when the first chunk
		// arrived. Guard against a degenerate unpainted baseline so the height
		// assertion below cannot silently become a tautology.
		const anchoredTop = promptTop();
		const thinkingHeight = liveRow.getBoundingClientRect().height;
		expect(thinkingHeight).toBeGreaterThan(0);

		// The first stream chunk replaces the Thinking indicator with text. The
		// live row must never shrink below its Thinking-indicator height, so the
		// anchored prompt and everything above it must stay put. Position uses a
		// tolerance because rect tops are fractional; a 24px drop is 6x it.
		thinkingShiftStore.applyMessageParts([
			{ type: "text", text: "Here is the start of the answer." },
		]);
		for (let i = 0; i < 6; i++) {
			expect(liveRow.getBoundingClientRect().height).toBeGreaterThanOrEqual(
				thinkingHeight,
			);
			expect(Math.abs(promptTop() - anchoredTop)).toBeLessThan(4);
			await new Promise<void>((r) => requestAnimationFrame(() => r()));
		}
	},
};

const underflowFetchSpy = fn();

const UnderflowPaginationStory: FC = () => {
	const store = useRef(
		buildStoreWithMessages([
			buildMessage(9, "assistant", "The newest loaded message. ".repeat(6)),
		]),
	).current;
	const [loadedPages, setLoadedPages] = useState(0);
	const [isFetching, setIsFetching] = useState(false);

	const completePage = () => {
		const nextPage = loadedPages + 1;
		store.replaceMessages([
			buildMessage(
				9 - nextPage,
				nextPage % 2 === 1 ? "user" : "assistant",
				`Loaded underflow page ${nextPage}. `.repeat(20),
			),
			...getStoreMessages(store),
		]);
		setLoadedPages(nextPage);
		setIsFetching(false);
	};

	return (
		<div className="flex h-[600px] flex-col">
			<StoryAgentChatPageView
				store={store}
				hasMoreMessages={loadedPages < 2}
				isFetchingMoreMessages={isFetching}
				onFetchMoreMessages={async () => {
					underflowFetchSpy();
					setIsFetching(true);
				}}
			/>
			<button type="button" disabled={!isFetching} onClick={completePage}>
				Complete history page
			</button>
		</div>
	);
};

/**
 * A transcript that does not fill the viewport keeps asking for history, one
 * page at a time, until it overflows or the history ends.
 */
export const ShortTranscriptLoadsUntilHistoryIsExhausted: Story = {
	parameters: { pixel: { exclude: true } },
	beforeEach: () => {
		underflowFetchSpy.mockClear();
	},
	render: () => <UnderflowPaginationStory />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const completePage = canvas.getByRole("button", {
			name: "Complete history page",
		});

		await waitFor(() => {
			expect(underflowFetchSpy).toHaveBeenCalledTimes(1);
			expect(
				canvas.getByRole("status", { name: "Loading earlier messages" }),
			).toBeVisible();
		});
		scrollTo(getViewport(canvas), 0);
		expect(underflowFetchSpy).toHaveBeenCalledTimes(1);

		await userEvent.click(completePage);
		await waitFor(() => {
			expect(canvas.getByText(/Loaded underflow page 1\./)).toBeVisible();
			expect(underflowFetchSpy).toHaveBeenCalledTimes(2);
		});

		await userEvent.click(completePage);
		await waitFor(() => {
			expect(canvas.getByText(/Loaded underflow page 2\./)).toBeVisible();
			expect(
				canvas.queryByRole("status", { name: "Loading earlier messages" }),
			).not.toBeInTheDocument();
		});
		expect(underflowFetchSpy).toHaveBeenCalledTimes(2);
	},
};

const retryFetchSpy = fn();

const RetryPaginationStory: FC = () => {
	const store = useRef(
		buildStoreWithMessages(buildLongConversation(AGENT_ID, 40)),
	).current;
	const [hasError, setHasError] = useState(true);
	const [isFetching, setIsFetching] = useState(false);

	const completePage = () => {
		prependOlderMessages(store, 4);
		setIsFetching(false);
	};

	return (
		<div className="flex h-[600px] flex-col">
			<StoryAgentChatPageView
				store={store}
				hasMoreMessages
				isFetchingMoreMessages={isFetching}
				hasFetchMoreError={hasError}
				onFetchMoreMessages={async () => {
					retryFetchSpy();
					setHasError(false);
					setIsFetching(true);
				}}
			/>
			<button type="button" disabled={!isFetching} onClick={completePage}>
				Complete retry page
			</button>
		</div>
	);
};

/**
 * A failed history request stops the automatic loading and offers a retry that
 * keyboard users can reach.
 */
export const FailedHistoryPageOffersKeyboardRetry: Story = {
	parameters: { pixel: { exclude: true } },
	beforeEach: () => {
		retryFetchSpy.mockClear();
	},
	render: () => <RetryPaginationStory />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const viewport = getViewport(canvas);
		const retry = canvas.getByRole("button", {
			name: "Retry loading earlier messages",
		});

		scrollTo(viewport, 0);
		await settleScroller();
		expect(retry).toBeVisible();
		expect(retryFetchSpy).not.toHaveBeenCalled();

		retry.focus();
		expect(retry).toHaveFocus();
		await userEvent.keyboard("{Enter}");

		await waitFor(() => {
			expect(retryFetchSpy).toHaveBeenCalledTimes(1);
			expect(
				canvas.getByRole("status", { name: "Loading earlier messages" }),
			).toBeVisible();
		});

		await userEvent.click(
			canvas.getByRole("button", { name: "Complete retry page" }),
		);
		await waitFor(() => {
			expect(canvas.getByText("Older question 3.")).toBeInTheDocument();
		});
	},
};

/**
 * Selecting the Terminal tab in the sidebar must move keyboard focus into
 * the terminal so typing goes there, not the chat input.
 */
export const TerminalFocusOnTabSwitch: Story = {
	parameters: {
		pixel: { exclude: true },
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

		// Sidebar defaults to Summary; this story drives the Terminal tab instead.
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

const sidebarTabStorageKey = `${lastActiveSidebarTabStorageKeyPrefix}${AGENT_ID}`;

/**
 * When localStorage contains a persisted tab ID for this chat, the sidebar
 * should restore it on mount. Seed localStorage with "git" and verify that
 * the Git tab is selected instead of the default Summary tab.
 */
export const RestoresPersistedSidebarTab: Story = {
	beforeEach: () => {
		localStorage.setItem(sidebarTabStorageKey, "git");
		return () => {
			localStorage.removeItem(sidebarTabStorageKey);
		};
	},
	render: () => (
		<StoryAgentChatPageView
			showSidebarPanel
			workspace={MockWorkspace}
			workspaceAgent={MockWorkspaceAgent}
			sshCommand="ssh coder.workspace"
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await waitFor(() => {
			const gitTab = canvas.getByRole("tab", { name: "Git" });
			expect(gitTab).toHaveAttribute("aria-selected", "true");
		});

		const summaryTab = canvas.getByRole("tab", { name: "Summary" });
		expect(summaryTab).toHaveAttribute("aria-selected", "false");
	},
};

/**
 * Clicking a sidebar tab persists the selection to localStorage so that
 * it is restored across session switches.
 */
export const PersistsSidebarTabClick: Story = {
	beforeEach: () => {
		localStorage.removeItem(sidebarTabStorageKey);
		return () => {
			localStorage.removeItem(sidebarTabStorageKey);
		};
	},
	render: () => (
		<StoryAgentChatPageView
			showSidebarPanel
			workspace={MockWorkspace}
			workspaceAgent={MockWorkspaceAgent}
			sshCommand="ssh coder.workspace"
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await waitFor(() => {
			const summaryTab = canvas.getByRole("tab", { name: "Summary" });
			expect(summaryTab).toHaveAttribute("aria-selected", "true");
		});

		const gitTab = canvas.getByRole("tab", { name: "Git" });
		await userEvent.click(gitTab);

		await waitFor(() => {
			expect(gitTab).toHaveAttribute("aria-selected", "true");
		});

		expect(localStorage.getItem(sidebarTabStorageKey)).toBe("git");
	},
};

/**
 * When localStorage holds an unavailable tab ID (e.g. `"terminal"` while the
 * workspace is stopped), the sidebar falls back to the first available tab
 * (Summary) while preserving the stored value for when the tab reappears.
 * Guards the `getEffectiveTabId` contract: it only reads `sidebarTabId`, never
 * writes back, so restore-after-recovery cannot silently break.
 */
export const PreservesUnavailableSidebarTab: Story = {
	beforeEach: () => {
		localStorage.setItem(sidebarTabStorageKey, "terminal");
		return () => {
			localStorage.removeItem(sidebarTabStorageKey);
		};
	},
	render: () => <StoryAgentChatPageView showSidebarPanel />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await waitFor(() => {
			const summaryTab = canvas.getByRole("tab", { name: "Summary" });
			expect(summaryTab).toHaveAttribute("aria-selected", "true");
		});

		expect(canvas.queryByRole("tab", { name: "Terminal" })).toBeNull();

		expect(localStorage.getItem(sidebarTabStorageKey)).toBe("terminal");
	},
};

const mockAgentBrowserApp: TypesGen.WorkspaceApp = {
	...MockWorkspaceApp,
	id: "agent-browser-app",
	slug: AGENT_BROWSER_APP_SLUG,
	display_name: "agent-browser",
	health: "healthy",
};

const mockAgentWithBrowserApp: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	apps: [...MockWorkspaceAgent.apps, mockAgentBrowserApp],
};

export const BrowserTabForHealthyAgentBrowserApp: Story = {
	render: () => (
		<StoryAgentChatPageView
			showSidebarPanel
			workspace={MockWorkspace}
			workspaceAgent={mockAgentWithBrowserApp}
			sshCommand="ssh coder.workspace"
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const browserTab = await canvas.findByRole("tab", { name: "Browser" });
		const tabLabels = canvas.getAllByRole("tab").map((tab) => tab.textContent);
		expect(tabLabels).toEqual(["Summary", "Git", "Browser", "Terminal"]);

		// The frame stays mounted while inactive to preserve app state, so
		// assert visibility rather than presence.
		const frame = canvas.getByTitle("agent-browser");
		expect(frame.checkVisibility()).toBe(false);

		await userEvent.click(browserTab);

		await waitFor(() => {
			expect(browserTab).toHaveAttribute("aria-selected", "true");
		});
		expect(frame.checkVisibility()).toBe(true);
	},
};

export const BrowserTabForHealthDisabledAgentBrowserApp: Story = {
	render: () => (
		<StoryAgentChatPageView
			showSidebarPanel
			workspace={MockWorkspace}
			workspaceAgent={{
				...MockWorkspaceAgent,
				apps: [{ ...mockAgentBrowserApp, health: "disabled" }],
			}}
			sshCommand="ssh coder.workspace"
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const browserTab = await canvas.findByRole("tab", { name: "Browser" });
		await userEvent.click(browserTab);

		await waitFor(() => {
			expect(browserTab).toHaveAttribute("aria-selected", "true");
		});
		expect(canvas.getByTitle("agent-browser").checkVisibility()).toBe(true);
	},
};

export const NoBrowserTabForUnhealthyAgentBrowserApp: Story = {
	render: () => (
		<StoryAgentChatPageView
			showSidebarPanel
			workspace={MockWorkspace}
			workspaceAgent={{
				...MockWorkspaceAgent,
				apps: [{ ...mockAgentBrowserApp, health: "unhealthy" }],
			}}
			sshCommand="ssh coder.workspace"
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByRole("tab", { name: "Summary" });
		expect(canvas.queryByRole("tab", { name: "Browser" })).toBeNull();
	},
};

export const NoBrowserTabForAppOnNonBoundAgent: Story = {
	render: () => (
		<StoryAgentChatPageView
			showSidebarPanel
			workspace={{
				...MockWorkspace,
				latest_build: {
					...MockWorkspace.latest_build,
					resources: [
						{
							...MockWorkspaceResource,
							agents: [
								MockWorkspaceAgent,
								{
									...mockAgentWithBrowserApp,
									id: "other-agent",
									name: "other-agent",
								},
							],
						},
					],
				},
			}}
			workspaceAgent={MockWorkspaceAgent}
			sshCommand="ssh coder.workspace"
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByRole("tab", { name: "Summary" });
		expect(canvas.queryByRole("tab", { name: "Browser" })).toBeNull();
	},
};

export const PreservesUnavailableBrowserTab: Story = {
	beforeEach: () => {
		localStorage.setItem(sidebarTabStorageKey, "browser");
		return () => {
			localStorage.removeItem(sidebarTabStorageKey);
		};
	},
	render: () => (
		<StoryAgentChatPageView
			showSidebarPanel
			workspace={MockWorkspace}
			workspaceAgent={MockWorkspaceAgent}
			sshCommand="ssh coder.workspace"
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await waitFor(() => {
			const summaryTab = canvas.getByRole("tab", { name: "Summary" });
			expect(summaryTab).toHaveAttribute("aria-selected", "true");
		});

		expect(canvas.queryByRole("tab", { name: "Browser" })).toBeNull();

		expect(localStorage.getItem(sidebarTabStorageKey)).toBe("browser");
	},
};

/**
 * When a chat is archived, clicking a sidebar tab must not persist the
 * selection to localStorage. The archive flow clears the entry on
 * purpose so that a subsequent unarchive starts from the default tab;
 * persisting here would silently recreate the entry for any tab the
 * user clicks while viewing the read-only archived view.
 *
 * This locks down the fix for the codex P2 review comment.
 */
export const DoesNotPersistForArchivedChat: Story = {
	beforeEach: () => {
		localStorage.removeItem(sidebarTabStorageKey);
		return () => {
			localStorage.removeItem(sidebarTabStorageKey);
		};
	},
	render: () => (
		<StoryAgentChatPageView
			showSidebarPanel
			isArchived
			isInputDisabled
			workspace={MockWorkspace}
			workspaceAgent={MockWorkspaceAgent}
			sshCommand="ssh coder.workspace"
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await waitFor(() => {
			const summaryTab = canvas.getByRole("tab", { name: "Summary" });
			expect(summaryTab).toHaveAttribute("aria-selected", "true");
		});

		const gitTab = canvas.getByRole("tab", { name: "Git" });
		await userEvent.click(gitTab);

		await waitFor(() => {
			expect(gitTab).toHaveAttribute("aria-selected", "true");
		});

		expect(localStorage.getItem(sidebarTabStorageKey)).toBeNull();
	},
};

export const ArchivedWithSharing: Story = {
	render: () => (
		<StoryAgentChatPageView
			isArchived
			isInputDisabled
			canShareChat
			organizationId={MockDefaultOrganization.id}
		/>
	),
	beforeEach: () => {
		spyOn(API.experimental, "getChatACL").mockResolvedValue({
			users: [],
			groups: [],
		});
		spyOn(API.experimental, "updateChatACL").mockResolvedValue(undefined);
		spyOn(API, "getOrganizationPaginatedMembers").mockResolvedValue({
			members: [MockOrganizationMember, MockOrganizationMember2],
			count: 2,
		});
		spyOn(API, "getGroupsByOrganization").mockResolvedValue([MockGroup]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText("This agent has been archived and is read-only."),
		).toBeVisible();

		await userEvent.click(canvas.getByLabelText("Share chat"));
		const body = within(document.body);
		await waitFor(() => {
			expect(body.getByText("Chat sharing")).toBeVisible();
		});
		await waitFor(() => {
			expect(body.getByText("No shared members or groups yet")).toBeVisible();
		});
	},
};

export const ShareChatPopoverFromTopBar: Story = {
	render: () => (
		<StoryAgentChatPageView
			canShareChat
			organizationId={MockDefaultOrganization.id}
		/>
	),
	beforeEach: () => {
		spyOn(API.experimental, "getChatACL").mockResolvedValue({
			users: [],
			groups: [],
		});
		spyOn(API.experimental, "updateChatACL").mockResolvedValue(undefined);
		spyOn(API, "getOrganizationPaginatedMembers").mockResolvedValue({
			members: [MockOrganizationMember, MockOrganizationMember2],
			count: 2,
		});
		spyOn(API, "getGroupsByOrganization").mockResolvedValue([MockGroup]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByLabelText("Share chat"));
		const body = within(document.body);
		await waitFor(() => {
			expect(body.getByText("Chat sharing")).toBeVisible();
		});
		await waitFor(() => {
			expect(body.getByText("No shared members or groups yet")).toBeVisible();
		});
	},
};
