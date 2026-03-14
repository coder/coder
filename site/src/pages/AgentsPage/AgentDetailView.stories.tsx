import { MockUserOwner } from "testHelpers/entities";
import { withAuthProvider, withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type * as TypesGen from "api/typesGenerated";
import type { ChatDiffStatus } from "api/typesGenerated";
import type { ModelSelectorOption } from "components/ai-elements";
import { fn } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
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

const defaultModelOptions: ModelSelectorOption[] = [
	{
		id: "openai:gpt-4o",
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
	last_model_config_id: "model-config-1",
	created_at: oneWeekAgo,
	updated_at: oneWeekAgo,
	archived: false,
	last_error: null,
	...overrides,
});

const defaultEditing = {
	chatInputRef: { current: null },
	editorInitialValue: "",
	editingMessageId: null,
	editingFileBlocks: [] as readonly {
		mediaType: string;
		data?: string;
		fileId?: string;
	}[],
	handleEditUserMessage: fn(),
	handleCancelHistoryEdit: fn(),
	editingQueuedMessageID: null,
	handleStartQueueEdit: fn(),
	handleCancelQueueEdit: fn(),
	handleSendFromInput: fn(),
	handleContentChange: fn(),
};

const defaultGitWatcher: {
	repositories: ReadonlyMap<string, TypesGen.WorkspaceAgentRepoChanges>;
	refresh: () => void;
} = {
	repositories: new Map(),
	refresh: fn(),
};

const agentsRouting = [
	{ path: "/agents/:agentId", useStoryElement: true },
	{ path: "/agents", useStoryElement: true },
] satisfies [
	{ path: string; useStoryElement: boolean },
	...{ path: string; useStoryElement: boolean }[],
];

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
	args: {
		agentId: AGENT_ID,
		chatTitle: "Help me refactor",
		parentChat: undefined,
		chatErrorReasons: {},
		chatRecord: buildChat(),
		isArchived: false,
		hasWorkspace: true,
		store: createChatStore(),
		editing: defaultEditing,
		pendingEditMessageId: null,
		effectiveSelectedModel: "openai:gpt-4o",
		setSelectedModel: fn(),
		modelOptions: defaultModelOptions,
		modelSelectorPlaceholder: "Select a model",
		hasModelOptions: true,
		inputStatusText: null,
		modelCatalogStatusMessage: null,
		compressionThreshold: undefined,
		isInputDisabled: false,
		isSubmissionPending: false,
		isInterruptPending: false,
		isSidebarCollapsed: false,
		onToggleSidebarCollapsed: fn(),
		showSidebarPanel: false,
		onSetShowSidebarPanel: fn(),
		prNumber: undefined,
		diffStatusData: undefined,
		gitWatcher: defaultGitWatcher,
		canOpenEditors: false,
		canOpenWorkspace: false,
		sshCommand: undefined,
		handleOpenInEditor: fn(),
		handleViewWorkspace: fn(),
		handleOpenTerminal: fn(),
		handleCommit: fn(),
		onNavigateToChat: fn(),
		handleInterrupt: fn(),
		handleDeleteQueuedMessage: fn(),
		handlePromoteQueuedMessage: fn(),
		handleArchiveAgentAction: fn(),
		handleUnarchiveAgentAction: fn(),
		handleArchiveAndDeleteWorkspaceAction: fn(),
		scrollContainerRef: { current: null },
	},
};

export default meta;
type Story = StoryObj<typeof AgentDetailView>;

// ---------------------------------------------------------------------------
// AgentDetailView stories
// ---------------------------------------------------------------------------

/** Basic conversation view with a chat title, workspace, and no archive. */
export const Default: Story = {};

/** Archived agent displays the read-only banner below the top bar. */
export const Archived: Story = {
	args: {
		isArchived: true,
		chatRecord: buildChat({ archived: true }),
		isInputDisabled: true,
	},
};

/** Shows the parent chat link in the top bar when a parent exists. */
export const WithParentChat: Story = {
	args: {
		parentChat: buildChat({
			id: "parent-chat-1",
			title: "Root agent",
		}),
	},
};

/** Persisted error reason shown in the timeline area. */
export const WithError: Story = {
	args: {
		chatErrorReasons: { [AGENT_ID]: "Model rate limited" },
	},
};

/** Input area appears disabled when `isInputDisabled` is true. */
export const InputDisabled: Story = {
	args: {
		isInputDisabled: true,
	},
};

/** Shows a sending/pending state for the input. */
export const SubmissionPending: Story = {
	args: {
		isSubmissionPending: true,
	},
};

/** Right sidebar panel is open with diff status data. */
export const WithSidebarPanel: Story = {
	args: {
		showSidebarPanel: true,
		prNumber: 123,
		diffStatusData: {
			chat_id: AGENT_ID,
			url: "https://github.com/coder/coder/pull/123",
			pull_request_title: "fix: resolve race condition in workspace builds",
			pull_request_draft: false,
			changes_requested: false,
			additions: 42,
			deletions: 7,
			changed_files: 5,
		} satisfies ChatDiffStatus,
	},
};

/** Left sidebar is collapsed. */
export const SidebarCollapsed: Story = {
	args: {
		isSidebarCollapsed: true,
	},
};

/** No model options available — shows a disabled status message. */
export const NoModelOptions: Story = {
	args: {
		hasModelOptions: false,
		modelOptions: [],
		inputStatusText: "No models configured. Ask an admin.",
		isInputDisabled: true,
	},
};

/** Top bar has workspace action buttons visible. */
export const WithWorkspaceActions: Story = {
	args: {
		canOpenEditors: true,
		canOpenWorkspace: true,
		sshCommand: "ssh coder.workspace",
	},
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
			effectiveSelectedModel="openai:gpt-4o"
			setSelectedModel={fn()}
			modelOptions={defaultModelOptions}
			modelSelectorPlaceholder="Select a model"
			hasModelOptions
			inputStatusText={null}
			modelCatalogStatusMessage={null}
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
			effectiveSelectedModel="openai:gpt-4o"
			setSelectedModel={fn()}
			modelOptions={defaultModelOptions}
			modelSelectorPlaceholder="Select a model"
			hasModelOptions
			inputStatusText={null}
			modelCatalogStatusMessage={null}
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
			effectiveSelectedModel="openai:gpt-4o"
			setSelectedModel={fn()}
			modelOptions={defaultModelOptions}
			modelSelectorPlaceholder="Select a model"
			hasModelOptions
			inputStatusText={null}
			modelCatalogStatusMessage={null}
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
			effectiveSelectedModel="openai:gpt-4o"
			setSelectedModel={fn()}
			modelOptions={defaultModelOptions}
			modelSelectorPlaceholder="Select a model"
			hasModelOptions
			inputStatusText={null}
			modelCatalogStatusMessage={null}
			isSidebarCollapsed
			onToggleSidebarCollapsed={fn()}
			showRightPanel={false}
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
