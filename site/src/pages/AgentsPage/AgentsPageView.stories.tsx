import { MockUserOwner } from "testHelpers/entities";
import { withAuthProvider, withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import type { Chat } from "api/typesGenerated";
import type { ModelSelectorOption } from "components/ai-elements";
import { fn, spyOn } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { AgentsPageView } from "./AgentsPageView";

const defaultModelOptions: ModelSelectorOption[] = [
	{
		id: "openai:gpt-4o",
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
];

const defaultModelConfigs: TypesGen.ChatModelConfig[] = [
	{
		id: "config-openai-gpt-4o",
		provider: "openai",
		model: "gpt-4o",
		display_name: "GPT-4o",
		enabled: true,
		is_default: false,
		context_limit: 200000,
		compression_threshold: 70,
		created_at: "2026-02-18T00:00:00.000Z",
		updated_at: "2026-02-18T00:00:00.000Z",
	},
];

const oneWeekAgo = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString();
const todayTimestamp = new Date().toISOString();

const buildChat = (overrides: Partial<Chat> = {}): Chat => ({
	id: "chat-default",
	owner_id: "owner-1",
	title: "Agent",
	status: "completed",
	last_model_config_id: defaultModelConfigs[0].id,
	created_at: oneWeekAgo,
	updated_at: oneWeekAgo,
	archived: false,
	last_error: null,
	...overrides,
});

const agentsRouting = [
	{ path: "/agents/:agentId", useStoryElement: true },
	{ path: "/agents", useStoryElement: true },
] satisfies [
	{ path: string; useStoryElement: boolean },
	...{ path: string; useStoryElement: boolean }[],
];

const meta: Meta<typeof AgentsPageView> = {
	title: "pages/AgentsPage/AgentsPageView",
	component: AgentsPageView,
	decorators: [withAuthProvider, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	args: {
		agentId: undefined,
		chatList: [],
		catalogModelOptions: defaultModelOptions,
		modelConfigs: defaultModelConfigs,
		logoUrl: "",
		handleNewAgent: fn(),
		isCreating: false,
		isArchiving: false,
		archivingChatId: undefined,
		isChatsLoading: false,
		chatsLoadError: null,
		onRetryChatsLoad: fn(),
		onCollapseSidebar: fn(),
		isSidebarCollapsed: false,
		onExpandSidebar: fn(),
		outletContext: {
			chatErrorReasons: {},
			setChatErrorReason: fn(),
			clearChatErrorReason: fn(),
			requestArchiveAgent: fn(),
			requestUnarchiveAgent: fn(),
			requestArchiveAndDeleteWorkspace: fn(),
			isSidebarCollapsed: false,
			onToggleSidebarCollapsed: fn(),
		},
		isAgentsAdmin: false,
		archivedFilter: "active" as const,
		onArchivedFilterChange: fn(),
		isFetchingNextPage: false,
		onCreateChat: fn(),
		createError: undefined,
		modelCatalog: undefined,
		isModelCatalogLoading: false,
		isModelConfigsLoading: false,
		modelCatalogError: undefined,
	},
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [],
			count: 0,
		});
	},
};

export default meta;
type Story = StoryObj<typeof AgentsPageView>;

export const EmptyState: Story = {};

export const WithChatList: Story = {
	args: {
		chatList: [
			buildChat({
				id: "chat-1",
				title: "Refactor authentication module",
				status: "completed",
				updated_at: todayTimestamp,
			}),
			buildChat({
				id: "chat-2",
				title: "Add unit tests for API layer",
				status: "running",
				updated_at: todayTimestamp,
			}),
			buildChat({
				id: "chat-3",
				title: "Fix database migration issue",
				status: "error",
				last_error: "Connection timeout",
				updated_at: todayTimestamp,
			}),
			buildChat({
				id: "chat-4",
				title: "Update CI/CD pipeline config",
				status: "waiting",
				updated_at: todayTimestamp,
			}),
			buildChat({
				id: "chat-5",
				title: "Implement WebSocket handler",
				status: "completed",
				updated_at: todayTimestamp,
			}),
			buildChat({
				id: "chat-6",
				title: "Debug memory leak in worker",
				status: "paused",
				updated_at: todayTimestamp,
			}),
		],
	},
};

export const LoadingChats: Story = {
	args: {
		isChatsLoading: true,
		chatList: [],
	},
};

export const ChatsLoadError: Story = {
	args: {
		chatsLoadError: new Error("Failed to fetch chats"),
	},
};

export const SidebarCollapsed: Story = {
	args: {
		isSidebarCollapsed: true,
		chatList: [
			buildChat({
				id: "chat-1",
				title: "Collapsed sidebar agent",
				updated_at: todayTimestamp,
			}),
		],
		outletContext: {
			chatErrorReasons: {},
			setChatErrorReason: fn(),
			clearChatErrorReason: fn(),
			requestArchiveAgent: fn(),
			requestUnarchiveAgent: fn(),
			requestArchiveAndDeleteWorkspace: fn(),
			isSidebarCollapsed: true,
			onToggleSidebarCollapsed: fn(),
		},
	},
};

export const WithToolbarEndContent: Story = {
	args: {
		isAgentsAdmin: true,
	},
};

export const CreatingAgent: Story = {
	args: {
		isCreating: true,
		chatList: [
			buildChat({
				id: "chat-1",
				title: "Existing agent",
				updated_at: todayTimestamp,
			}),
		],
	},
};

export const ArchivingAgent: Story = {
	args: {
		isArchiving: true,
		archivingChatId: "chat-1",
		chatList: [
			buildChat({
				id: "chat-1",
				title: "Agent being archived",
				updated_at: todayTimestamp,
			}),
			buildChat({
				id: "chat-2",
				title: "Another agent",
				updated_at: todayTimestamp,
			}),
		],
	},
};

export const WithAgentSelected: Story = {
	args: {
		agentId: "chat-1",
		chatList: [
			buildChat({
				id: "chat-1",
				title: "Selected agent",
				status: "running",
				updated_at: todayTimestamp,
			}),
			buildChat({
				id: "chat-2",
				title: "Another agent",
				updated_at: todayTimestamp,
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents/chat-1",
				pathParams: { agentId: "chat-1" },
			},
			routing: agentsRouting,
		}),
	},
};

export const WithErrorReasons: Story = {
	args: {
		chatList: [
			buildChat({
				id: "chat-1",
				title: "Rate limited agent",
				status: "error",
				updated_at: todayTimestamp,
			}),
			buildChat({
				id: "chat-2",
				title: "Healthy agent",
				status: "running",
				updated_at: todayTimestamp,
			}),
			buildChat({
				id: "chat-3",
				title: "Another errored agent",
				status: "error",
				updated_at: todayTimestamp,
			}),
		],
		outletContext: {
			chatErrorReasons: {
				"chat-1": "Model rate limited",
				"chat-3": "Context window exceeded",
			},
			setChatErrorReason: fn(),
			clearChatErrorReason: fn(),
			requestArchiveAgent: fn(),
			requestUnarchiveAgent: fn(),
			requestArchiveAndDeleteWorkspace: fn(),
			isSidebarCollapsed: false,
			onToggleSidebarCollapsed: fn(),
		},
	},
};
