import { MockUserOwner } from "testHelpers/entities";
import { withAuthProvider, withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import type { Chat } from "api/typesGenerated";
import type { ModelSelectorOption } from "components/ai-elements";
import {
	expect,
	fn,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
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

const mockAnalyticsSummary: TypesGen.ChatCostSummary = {
	start_date: "2026-02-10T00:00:00Z",
	end_date: "2026-03-12T00:00:00Z",
	total_cost_micros: 1_500_000,
	priced_message_count: 12,
	unpriced_message_count: 1,
	total_input_tokens: 123_456,
	total_output_tokens: 654_321,
	total_cache_read_tokens: 9_876,
	total_cache_creation_tokens: 5_432,
	by_model: [
		{
			model_config_id: "model-config-1",
			display_name: "GPT-4.1",
			provider: "OpenAI",
			model: "gpt-4.1",
			total_cost_micros: 1_250_000,
			message_count: 9,
			total_input_tokens: 100_000,
			total_output_tokens: 200_000,
			total_cache_read_tokens: 7_654,
			total_cache_creation_tokens: 3_210,
		},
	],
	by_chat: [
		{
			root_chat_id: "chat-1",
			chat_title: "Quarterly review",
			total_cost_micros: 750_000,
			message_count: 5,
			total_input_tokens: 60_000,
			total_output_tokens: 80_000,
			total_cache_read_tokens: 4_321,
			total_cache_creation_tokens: 1_234,
		},
	],
};

const mockUsageUsers: TypesGen.ChatCostUsersResponse = {
	start_date: "2026-02-10T00:00:00Z",
	end_date: "2026-03-12T00:00:00Z",
	count: 1,
	users: [
		{
			user_id: "user-1",
			username: "alice",
			name: "Alice Example",
			avatar_url: "https://example.com/alice.png",
			total_cost_micros: 1_200_000,
			message_count: 12,
			chat_count: 3,
			total_input_tokens: 120_000,
			total_output_tokens: 45_000,
			total_cache_read_tokens: 6_789,
			total_cache_creation_tokens: 2_468,
		},
	],
};

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
		spyOn(API, "getChatCostSummary").mockResolvedValue(mockAnalyticsSummary);
		spyOn(API, "getChatCostUsers").mockResolvedValue(mockUsageUsers);
		spyOn(API, "getChatSystemPrompt").mockResolvedValue({
			system_prompt: "",
		});
		spyOn(API, "updateChatSystemPrompt").mockResolvedValue();
		spyOn(API, "getUserChatCustomPrompt").mockResolvedValue({
			custom_prompt: "",
		});
		spyOn(API, "updateUserChatCustomPrompt").mockResolvedValue({
			custom_prompt: "",
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
				"chat-1": { kind: "generic", message: "Model rate limited" },
				"chat-3": { kind: "generic", message: "Context window exceeded" },
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

type ChatCostSummaryCall = [
	user: string,
	params?: {
		start_date?: string;
		end_date?: string;
	},
];

const getChatCostSummaryCalls = (): ChatCostSummaryCall[] => {
	return (
		API.getChatCostSummary as typeof API.getChatCostSummary & {
			mock: { calls: ChatCostSummaryCall[] };
		}
	).mock.calls;
};

const openAnalyticsDialog = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	await userEvent.click(canvas.getByRole("button", { name: "Analytics" }));
	return screen.findByRole("dialog", { name: "Analytics" });
};

const openSettingsDialog = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	await userEvent.click(canvas.getByRole("button", { name: "Settings" }));
	return screen.findByRole("dialog", { name: "Settings" });
};

export const OpensAnalyticsForAdmins: Story = {
	args: {
		isAgentsAdmin: true,
	},
	play: async ({ canvasElement }) => {
		const dialog = await openAnalyticsDialog(canvasElement);

		await expect(dialog).toBeInTheDocument();
		expect(
			screen.queryByRole("dialog", { name: "Settings" }),
		).not.toBeInTheDocument();
	},
};

export const OpensAnalyticsForNonAdmins: Story = {
	args: {
		isAgentsAdmin: false,
	},
	play: async ({ canvasElement }) => {
		const dialog = await openAnalyticsDialog(canvasElement);

		await expect(dialog).toBeInTheDocument();
		expect(
			screen.queryByRole("dialog", { name: "Settings" }),
		).not.toBeInTheDocument();
	},
};

export const OpensSettingsForAdmins: Story = {
	args: {
		isAgentsAdmin: true,
	},
	play: async ({ canvasElement }) => {
		const dialog = await openSettingsDialog(canvasElement);

		await expect(dialog).toBeInTheDocument();
		await expect(
			within(dialog).getByText(
				"Custom instructions that shape how the agent responds in your chats.",
			),
		).toBeInTheDocument();
		expect(
			screen.queryByRole("dialog", { name: "Analytics" }),
		).not.toBeInTheDocument();
	},
};

export const OpensSettingsForNonAdmins: Story = {
	args: {
		isAgentsAdmin: false,
	},
	play: async ({ canvasElement }) => {
		const dialog = await openSettingsDialog(canvasElement);

		await expect(dialog).toBeInTheDocument();
		await expect(
			within(dialog).getByText(
				"Custom instructions that shape how the agent responds in your chats.",
			),
		).toBeInTheDocument();
	},
};

export const RemountsConfigureDialogWhenReopened: Story = {
	args: {
		isAgentsAdmin: true,
	},
	play: async ({ canvasElement }) => {
		let dialog = await openSettingsDialog(canvasElement);

		await userEvent.click(
			within(dialog).getByRole("button", { name: "Usage" }),
		);
		await waitFor(() => {
			expect(
				screen.getByText(
					"Review deployment chat usage and drill into individual users.",
				),
			).toBeInTheDocument();
		});

		await userEvent.click(
			within(dialog).getByRole("button", { name: "Close" }),
		);
		await waitFor(() => {
			expect(
				screen.queryByRole("dialog", { name: "Settings" }),
			).not.toBeInTheDocument();
		});

		dialog = await openSettingsDialog(canvasElement);

		await expect(
			within(dialog).getByText(
				"Custom instructions that shape how the agent responds in your chats.",
			),
		).toBeInTheDocument();
		expect(
			screen.queryByText(
				"Review deployment chat usage and drill into individual users.",
			),
		).not.toBeInTheDocument();
	},
};

export const RemountsAnalyticsDialogWhenReopened: Story = {
	args: {
		isAgentsAdmin: true,
	},
	play: async ({ canvasElement }) => {
		let dialog = await openAnalyticsDialog(canvasElement);

		await expect(dialog).toBeInTheDocument();
		await waitFor(() => {
			expect(getChatCostSummaryCalls().length).toBeGreaterThan(0);
		});
		const initialCallCount = getChatCostSummaryCalls().length;
		const initialEndDates = new Set(
			getChatCostSummaryCalls()
				.map(([, params]) => params?.end_date)
				.filter((endDate): endDate is string => Boolean(endDate)),
		);

		await userEvent.click(
			within(dialog).getByRole("button", { name: "Close" }),
		);
		await waitFor(() => {
			expect(
				screen.queryByRole("dialog", { name: "Analytics" }),
			).not.toBeInTheDocument();
		});

		dialog = await openAnalyticsDialog(canvasElement);
		await expect(dialog).toBeInTheDocument();
		await waitFor(() => {
			expect(getChatCostSummaryCalls().length).toBeGreaterThan(
				initialCallCount,
			);
		});

		const reopenedCalls = getChatCostSummaryCalls().slice(initialCallCount);
		expect(
			reopenedCalls.some(([, params]) => {
				const endDate = params?.end_date;
				return typeof endDate === "string" && !initialEndDates.has(endDate);
			}),
		).toBe(true);
	},
};
