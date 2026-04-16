import type { Meta, StoryObj } from "@storybook/react-vite";
import dayjs from "dayjs";
import { type ComponentProps, useState } from "react";
import { Navigate } from "react-router";
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
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import type { Chat } from "#/api/typesGenerated";
import { DeleteDialog } from "#/components/Dialogs/DeleteDialog/DeleteDialog";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	MockNoPermissions,
	MockPermissions,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import AgentAnalyticsPage from "./AgentAnalyticsPage";
import AgentCreatePage from "./AgentCreatePage";
import { AgentSettingsBehaviorPageView } from "./AgentSettingsBehaviorPageView";
import AgentSettingsPage from "./AgentSettingsPage";
import AgentSettingsSpendPage from "./AgentSettingsSpendPage";
import { AgentsPageView } from "./AgentsPageView";
import type { ModelSelectorOption } from "./components/ChatElements";

const defaultModelConfigID = "model-config-1";

const defaultModelOptions: ModelSelectorOption[] = [
	{
		id: defaultModelConfigID,
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
];

const defaultModelConfigs: TypesGen.ChatModelConfig[] = [
	{
		id: defaultModelConfigID,
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
	total_runtime_ms: 0,
	by_model: [
		{
			model_config_id: defaultModelConfigID,
			display_name: "GPT-4.1",
			provider: "OpenAI",
			model: "gpt-4.1",
			total_cost_micros: 1_250_000,
			message_count: 9,
			total_input_tokens: 100_000,
			total_output_tokens: 200_000,
			total_cache_read_tokens: 7_654,
			total_cache_creation_tokens: 3_210,
			total_runtime_ms: 0,
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
			total_runtime_ms: 0,
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
			total_runtime_ms: 0,
		},
	],
};

const oneWeekAgo = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString();
const todayTimestamp = new Date().toISOString();

const buildChat = (overrides: Partial<Chat> = {}): Chat => ({
	id: "chat-default",
	organization_id: "test-org-id",
	owner_id: "owner-1",
	title: "Agent",
	status: "completed",
	last_model_config_id: defaultModelConfigs[0].id,
	mcp_server_ids: [],
	labels: {},
	created_at: oneWeekAgo,
	updated_at: oneWeekAgo,
	archived: false,
	pin_order: 0,
	has_unread: false,
	client_type: "ui",
	last_error: null,
	...overrides,
});

// Use local noon so the rendered range label stays stable
// across timezones.
const fixedNow = dayjs("2026-03-12T12:00:00");

// Renders the real PageView components with mock data so the
// visual snapshots match the actual UI.
const BehaviorRouteElement = () => {
	const { permissions } = useAuthenticated();
	return (
		<AgentSettingsBehaviorPageView
			canSetSystemPrompt={permissions.editDeploymentConfig}
			systemPromptData={{
				system_prompt: "",
				include_default_system_prompt: true,
				default_system_prompt: "You are Coder, an AI coding assistant...",
			}}
			planModeInstructionsData={{
				plan_mode_instructions: "",
			}}
			userPromptData={{ custom_prompt: "" }}
			desktopEnabledData={{ enable_desktop: false }}
			workspaceTTLData={{ workspace_ttl_ms: 0 }}
			isWorkspaceTTLLoading={false}
			isWorkspaceTTLLoadError={false}
			modelConfigsData={[]}
			modelConfigsError={undefined}
			isLoadingModelConfigs={false}
			thresholds={[]}
			isThresholdsLoading={false}
			thresholdsError={undefined}
			onSaveSystemPrompt={fn()}
			isSavingSystemPrompt={false}
			isSaveSystemPromptError={false}
			onSavePlanModeInstructions={fn()}
			isSavingPlanModeInstructions={false}
			isSavePlanModeInstructionsError={false}
			onSaveUserPrompt={fn()}
			isSavingUserPrompt={false}
			isSaveUserPromptError={false}
			onSaveDesktopEnabled={fn()}
			isSavingDesktopEnabled={false}
			isSaveDesktopEnabledError={false}
			onSaveWorkspaceTTL={fn()}
			isSavingWorkspaceTTL={false}
			isSaveWorkspaceTTLError={false}
			retentionDaysData={{ retention_days: 30 }}
			isRetentionDaysLoading={false}
			isRetentionDaysLoadError={false}
			onSaveRetentionDays={fn()}
			isSavingRetentionDays={false}
			isSaveRetentionDaysError={false}
			onSaveThreshold={fn(async () => undefined)}
			onResetThreshold={fn(async () => undefined)}
		/>
	);
};

const agentsRouting = {
	path: "/agents",
	useStoryElement: true,
	children: [
		{
			path: "settings",
			element: <AgentSettingsPage />,
			children: [
				{ index: true, element: <Navigate to="behavior" replace /> },
				{ path: "behavior", element: <BehaviorRouteElement /> },
				{ path: "spend", element: <AgentSettingsSpendPage now={fixedNow} /> },
				{
					path: "usage",
					element: <Navigate to="/agents/settings/spend" replace />,
				},
			],
		},
		{ path: "analytics", element: <AgentAnalyticsPage now={fixedNow} /> },
		{ path: ":agentId", element: <div /> },
		{ index: true, element: <AgentCreatePage /> },
	],
};

const defaultArgs: ComponentProps<typeof AgentsPageView> = {
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
	chatErrorReasons: {},
	setChatErrorReason: fn(),
	clearChatErrorReason: fn(),
	requestArchiveAgent: fn(),
	requestUnarchiveAgent: fn(),
	requestArchiveAndDeleteWorkspace: fn(),
	requestPinAgent: fn(),
	requestUnpinAgent: fn(),
	onRegenerateTitle: fn(),
	regeneratingTitleChatIds: [],
	onToggleSidebarCollapsed: fn(),
	isAgentsAdmin: false,
	archivedFilter: "active",
	onArchivedFilterChange: fn(),
	hasNextPage: false,
	onLoadMore: fn(),
	isFetchingNextPage: false,
};

const meta: Meta<typeof AgentsPageView> = {
	title: "pages/AgentsPage/AgentsPageView",
	component: AgentsPageView,
	decorators: [withAuthProvider, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		permissions: MockPermissions,
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	args: defaultArgs,
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [],
			count: 0,
		});
		spyOn(API.experimental, "getChatCostSummary").mockResolvedValue(
			mockAnalyticsSummary,
		);
		spyOn(API.experimental, "getChatCostUsers").mockResolvedValue(
			mockUsageUsers,
		);
		spyOn(API.experimental, "getChatSystemPrompt").mockResolvedValue({
			system_prompt: "",
			include_default_system_prompt: true,
			default_system_prompt: "You are Coder, an AI coding assistant...",
		});
		spyOn(API.experimental, "updateChatSystemPrompt").mockResolvedValue();
		spyOn(API.experimental, "getUserChatCustomPrompt").mockResolvedValue({
			custom_prompt: "",
		});
		spyOn(API.experimental, "updateUserChatCustomPrompt").mockResolvedValue({
			custom_prompt: "",
		});
		// Mocks for child route pages that fetch their own data.
		spyOn(API.experimental, "getChatModels").mockResolvedValue({
			providers: [
				{
					provider: "openai",
					available: true,
					models: [
						{
							id: "openai:gpt-4o",
							provider: "openai",
							model: "gpt-4o",
							display_name: "GPT-4o",
						},
					],
				},
			],
		});
		spyOn(API.experimental, "getChatModelConfigs").mockResolvedValue([
			{
				id: defaultModelConfigID,
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
		]);
		spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
		spyOn(API.experimental, "getChatDesktopEnabled").mockResolvedValue({
			enable_desktop: false,
		});
		spyOn(API.experimental, "getChatWorkspaceTTL").mockResolvedValue({
			workspace_ttl_ms: 0,
		});
		spyOn(API.experimental, "updateChatWorkspaceTTL").mockResolvedValue();
		spyOn(API.experimental, "getChatUsageLimitConfig").mockResolvedValue({
			spend_limit_micros: null,
			period: "month",
			updated_at: "2026-02-18T00:00:00.000Z",
			unpriced_model_count: 0,
			overrides: [],
			group_overrides: [],
		});
		spyOn(API, "getGroups").mockResolvedValue([]);
		spyOn(API.experimental, "getChatCostUsers").mockResolvedValue({
			start_date: "2026-02-10T00:00:00Z",
			end_date: "2026-03-12T00:00:00Z",
			count: 0,
			users: [],
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
		chatErrorReasons: {},
		setChatErrorReason: fn(),
		clearChatErrorReason: fn(),
		requestArchiveAgent: fn(),
		requestUnarchiveAgent: fn(),
		requestArchiveAndDeleteWorkspace: fn(),
		onToggleSidebarCollapsed: fn(),
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

/**
 * Standalone story for the delete-confirmation dialog with
 * agents-specific copy (title, verb, info). The dialog now lives in
 * AgentsPage (the container) rather than AgentsPageView, so we
 * render it directly here to preserve interaction-test coverage.
 */
export const DeleteConfirmationDialog: Story = {
	render: function Render() {
		const [isOpen, setIsOpen] = useState(true);
		const [isLoading, setIsLoading] = useState(false);
		const onConfirm = fn();
		return (
			<DeleteDialog
				key="my-workspace"
				isOpen={isOpen}
				onConfirm={() => {
					onConfirm();
					setIsLoading(true);
				}}
				onCancel={() => setIsOpen(false)}
				entity="workspace"
				name="my-workspace"
				confirmLoading={isLoading}
				title="Archive agent & delete workspace"
				verb="Archiving and deleting"
				info="This will archive the agent and permanently delete the associated workspace and all its resources."
			/>
		);
	},
	play: async () => {
		const dialog = await screen.findByRole("dialog");
		await expect(dialog).toBeInTheDocument();
		await expect(
			within(dialog).getByText("Archive agent & delete workspace"),
		).toBeInTheDocument();

		// Confirm button should be disabled before typing the workspace name.
		const confirmButton = within(dialog).getByRole("button", {
			name: /delete/i,
		});
		await expect(confirmButton).toBeDisabled();

		// Type the workspace name to satisfy the confirmation guard.
		const input = within(dialog).getByLabelText(/name of the workspace/i);
		await userEvent.type(input, "my-workspace");
		await expect(confirmButton).toBeEnabled();

		// Click confirm and verify the callback fires, then enters loading state.
		await userEvent.click(confirmButton);
		await waitFor(() => {
			expect(confirmButton).toBeDisabled();
		});
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
		chatErrorReasons: {
			"chat-1": { kind: "generic", message: "Model rate limited" },
			"chat-3": { kind: "generic", message: "Context window exceeded" },
		},
		setChatErrorReason: fn(),
		clearChatErrorReason: fn(),
		requestArchiveAgent: fn(),
		requestUnarchiveAgent: fn(),
		requestArchiveAndDeleteWorkspace: fn(),
		onToggleSidebarCollapsed: fn(),
	},
};

const openSettingsView = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	const link = await waitFor(() =>
		canvas.getByRole("link", { name: "Settings" }),
	);
	await userEvent.click(link);
};

export const OpensAnalyticsForAdmins: Story = {
	args: {
		isAgentsAdmin: true,
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents/analytics" },
			routing: agentsRouting,
		}),
	},
	play: async () => {
		await waitFor(() => {
			expect(
				screen.getByText(
					"Review your personal Coder Agents usage and cost breakdowns.",
				),
			).toBeInTheDocument();
		});
	},
};

export const OpensAnalyticsForNonAdmins: Story = {
	args: {
		isAgentsAdmin: false,
	},
	parameters: {
		permissions: MockNoPermissions,
		reactRouter: reactRouterParameters({
			location: { path: "/agents/analytics" },
			routing: agentsRouting,
		}),
	},
	play: async () => {
		await waitFor(() => {
			expect(
				screen.getByText(
					"Review your personal Coder Agents usage and cost breakdowns.",
				),
			).toBeInTheDocument();
		});
	},
};

export const OpensSettingsForAdmins: Story = {
	args: {
		isAgentsAdmin: true,
	},
	play: async ({ canvasElement }) => {
		await openSettingsView(canvasElement);

		await waitFor(() => {
			expect(
				screen.getByText(
					"Custom instructions that shape how the agent responds in your conversations.",
				),
			).toBeInTheDocument();
		});
	},
};

export const OpensSettingsForNonAdmins: Story = {
	args: {
		isAgentsAdmin: false,
	},
	parameters: {
		permissions: MockNoPermissions,
	},
	play: async ({ canvasElement }) => {
		await openSettingsView(canvasElement);

		await waitFor(() => {
			expect(
				screen.getByText(
					"Custom instructions that shape how the agent responds in your conversations.",
				),
			).toBeInTheDocument();
		});
	},
};

export const SettingsViewResets: Story = {
	args: {
		isAgentsAdmin: true,
	},
	play: async ({ canvasElement }) => {
		// Open settings
		await openSettingsView(canvasElement);

		await waitFor(() => {
			expect(
				screen.getByText(
					"Custom instructions that shape how the agent responds in your conversations.",
				),
			).toBeInTheDocument();
		});

		// Navigate to Spend section
		await userEvent.click(screen.getByText("Spend"));
		await waitFor(() => {
			expect(
				screen.getByText(
					"Configure spend limits and monitor usage across your deployment.",
				),
			).toBeInTheDocument();
		});

		// Go back to conversations
		const backButton = screen.getByLabelText("Back to Agents");
		await userEvent.click(backButton);

		// Re-open settings, should reset to Behavior
		await openSettingsView(canvasElement);
		await waitFor(() => {
			expect(
				screen.getByText(
					"Custom instructions that shape how the agent responds in your conversations.",
				),
			).toBeInTheDocument();
		});
	},
};
