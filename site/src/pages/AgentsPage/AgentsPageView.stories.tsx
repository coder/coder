import type { Meta, StoryObj } from "@storybook/react-vite";
import dayjs from "dayjs";
import { type ComponentProps, useState } from "react";
import { Navigate, useOutletContext } from "react-router";
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
import { AgentSettingsAgentsPageView } from "./AgentSettingsAgentsPageView";
import AgentSettingsCompactionPage from "./AgentSettingsCompactionPage";
import AgentSettingsExperimentsPage from "./AgentSettingsExperimentsPage";
import AgentSettingsGeneralPage from "./AgentSettingsGeneralPage";
import AgentSettingsInstructionsPage from "./AgentSettingsInstructionsPage";
import AgentSettingsLifecyclePage from "./AgentSettingsLifecyclePage";
import AgentSettingsPage from "./AgentSettingsPage";
import AgentSettingsSpendPage from "./AgentSettingsSpendPage";
import { type AgentsOutletContext, AgentsPageView } from "./AgentsPageView";
import type { ModelSelectorOption } from "./components/ChatElements";
import { ChatTopBar } from "./components/ChatTopBar";

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
	children: [],
	...overrides,
});

// Use local noon so the rendered range label stays stable
// across timezones.
const fixedNow = dayjs("2026-03-12T12:00:00");

const AgentsRouteElement = () => (
	<AgentSettingsAgentsPageView
		adminOverridesData={{ allow_users: false }}
		onSaveAdminOverrides={fn()}
		isSavingAdminOverrides={false}
		isSaveAdminOverridesError={false}
		exploreModelOverrideData={{
			context: "explore",
			model_config_id: "",
			is_malformed: false,
		}}
		titleGenerationModelOverrideData={{
			context: "title_generation",
			model_config_id: "",
			is_malformed: false,
		}}
		modelConfigsData={[]}
		modelConfigsError={undefined}
		isLoadingModelConfigs={false}
		onSaveTitleGenerationModel={fn()}
		isSavingTitleGenerationModel={false}
		isSaveTitleGenerationModelError={false}
		onSaveExploreModelOverride={fn()}
		isSavingExploreModelOverride={false}
		isSaveExploreModelOverrideError={false}
	/>
);

const agentsRouting = {
	path: "/agents",
	useStoryElement: true,
	children: [
		{
			path: "settings",
			element: <AgentSettingsPage />,
			children: [
				{ index: true, element: <AgentSettingsGeneralPage /> },
				{ path: "general", element: <AgentSettingsGeneralPage /> },
				{ path: "compaction", element: <AgentSettingsCompactionPage /> },
				{
					path: "instructions",
					element: <AgentSettingsInstructionsPage />,
				},
				{ path: "experiments", element: <AgentSettingsExperimentsPage /> },
				{ path: "lifecycle", element: <AgentSettingsLifecyclePage /> },
				{ path: "admin", element: <AgentsRouteElement /> },
				{ path: "agents", element: <AgentsRouteElement /> },
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

const AgentTopBarRouteElement = () => {
	const { isSidebarCollapsed, onToggleSidebarCollapsed } =
		useOutletContext<AgentsOutletContext>();
	return (
		<ChatTopBar
			chatTitle="Collapsed sidebar agent"
			panel={{ showSidebarPanel: false, onToggleSidebar: fn() }}
			onArchiveAgent={fn()}
			onArchiveAndDeleteWorkspace={fn()}
			onRegenerateTitle={fn()}
			onUnarchiveAgent={fn()}
			isSidebarCollapsed={isSidebarCollapsed}
			onToggleSidebarCollapsed={onToggleSidebarCollapsed}
		/>
	);
};

const agentsWithChatTopBarRouting = {
	...agentsRouting,
	children: agentsRouting.children.map((route) =>
		"path" in route && route.path === ":agentId"
			? { ...route, element: <AgentTopBarRouteElement /> }
			: route,
	),
};

const defaultArgs: ComponentProps<typeof AgentsPageView> = {
	agentId: undefined,
	chatList: [],
	catalogModelOptions: defaultModelOptions,
	modelConfigs: defaultModelConfigs,
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
	onRegenerateTitle: fn(async () => "Generated title"),
	onProposeTitle: fn(async () => "Proposed title"),
	onRenameTitle: fn(async () => {}),
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
		spyOn(API.experimental, "updateChatDesktopEnabled").mockResolvedValue();
		spyOn(API.experimental, "getChatDebugLogging").mockResolvedValue({
			allow_users: false,
			forced_by_deployment: false,
		});
		spyOn(API.experimental, "updateChatDebugLogging").mockResolvedValue();
		spyOn(API.experimental, "getUserChatDebugLogging").mockResolvedValue({
			debug_logging_enabled: false,
			forced_by_deployment: false,
			user_toggle_allowed: false,
		});
		spyOn(API.experimental, "updateUserChatDebugLogging").mockResolvedValue();
		spyOn(API.experimental, "getChatPlanModeInstructions").mockResolvedValue({
			plan_mode_instructions: "",
		});
		spyOn(
			API.experimental,
			"updateChatPlanModeInstructions",
		).mockResolvedValue();
		spyOn(
			API.experimental,
			"getUserChatCompactionThresholds",
		).mockResolvedValue({
			thresholds: [],
		});
		spyOn(
			API.experimental,
			"updateUserChatCompactionThreshold",
		).mockResolvedValue({
			model_config_id: defaultModelConfigID,
			threshold_percent: 70,
		});
		spyOn(
			API.experimental,
			"deleteUserChatCompactionThreshold",
		).mockResolvedValue();
		spyOn(API.experimental, "getChatWorkspaceTTL").mockResolvedValue({
			workspace_ttl_ms: 0,
		});
		spyOn(API.experimental, "updateChatWorkspaceTTL").mockResolvedValue();
		spyOn(API.experimental, "getChatRetentionDays").mockResolvedValue({
			retention_days: 30,
		});
		spyOn(API.experimental, "updateChatRetentionDays").mockResolvedValue();
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

export const ArchivedEmptyState: Story = {
	args: {
		archivedFilter: "archived",
		chatList: [],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents",
				searchParams: { archived: "archived" },
			},
			routing: agentsRouting,
		}),
	},
	play: async () => {
		await expect(await screen.findByText("No archived agents")).toBeVisible();
		await expect(
			screen.getByRole("button", { name: /back to active/i }),
		).toBeVisible();
	},
};

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
				last_error: {
					message: "Connection timeout",
					kind: "generic",
					retryable: false,
				},
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

export const EmptyStateZoom200Desktop: Story = {
	parameters: {
		viewport: { defaultViewport: "desktopZoom200" },
		chromatic: { viewports: [720] },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const layout = await canvas.findByTestId("agents-page-layout");
		const sidebar = await canvas.findByTestId("agents-sidebar-panel");
		const main = await canvas.findByTestId("agents-main-panel");

		await waitFor(() => {
			const layoutStyles = getComputedStyle(layout);
			const sidebarStyles = getComputedStyle(sidebar);
			const mainStyles = getComputedStyle(main);
			const sidebarRect = sidebar.getBoundingClientRect();
			const mainRect = main.getBoundingClientRect();

			expect(layoutStyles.flexDirection).toBe("row");
			expect(sidebarStyles.display).not.toBe("none");
			expect(mainStyles.display).toBe("flex");
			expect(sidebarRect.width).toBeGreaterThan(0);
			expect(mainRect.width).toBeGreaterThan(0);
			expect(sidebarRect.left).toBeLessThan(mainRect.left);
			expect(sidebarRect.right).toBeLessThanOrEqual(mainRect.left + 1);
		});

		await expect(canvas.getByRole("link", { name: "Settings" })).toBeVisible();
		await expect(canvas.getByRole("link", { name: "New Agent" })).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: "Collapse sidebar" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: /TestUser/ }),
		).toBeVisible();
	},
};

export const CollapsedSidebarZoom200Desktop: Story = {
	args: {
		isSidebarCollapsed: true,
	},
	parameters: {
		viewport: { defaultViewport: "desktopZoom200" },
		chromatic: { viewports: [720] },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const expandButton = await canvas.findByRole("button", {
			name: "Expand sidebar",
		});

		await expect(expandButton).toBeVisible();
	},
};

export const CollapsedSidebarZoom200DesktopWithAgent: Story = {
	args: {
		agentId: "chat-1",
		isSidebarCollapsed: true,
		chatList: [
			buildChat({
				id: "chat-1",
				title: "Collapsed sidebar agent",
				updated_at: todayTimestamp,
			}),
		],
	},
	parameters: {
		viewport: { defaultViewport: "desktopZoom200" },
		chromatic: { viewports: [720] },
		reactRouter: reactRouterParameters({
			location: { path: "/agents/chat-1" },
			routing: agentsWithChatTopBarRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const expandButton = await canvas.findByRole("button", {
			name: "Expand sidebar",
		});

		await expect(expandButton).toBeVisible();
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
	const settingsLink = canvas.queryByRole("link", { name: "Settings" });
	if (settingsLink) {
		await userEvent.click(settingsLink);
		return;
	}

	const mobileMoreOptionsButton = canvas
		.getAllByRole("button", { name: "More options" })
		.find((button) => button.getAttribute("aria-haspopup") === "menu");
	if (!mobileMoreOptionsButton) {
		throw new Error("Expected a mobile More options menu button.");
	}
	await userEvent.click(mobileMoreOptionsButton);
	const body = within(canvasElement.ownerDocument.body);
	await userEvent.click(
		await body.findByRole("menuitem", { name: "Settings" }),
	);
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
				screen.getByText("Personal preferences for your chat experience."),
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
				screen.getByText("Personal preferences for your chat experience."),
			).toBeInTheDocument();
		});

		expect(
			screen.queryByRole("link", { name: "Manage Agents" }),
		).not.toBeInTheDocument();
	},
};

export const OpensAdminSubPanelOnMobile: Story = {
	args: {
		isAgentsAdmin: true,
	},
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		reactRouter: reactRouterParameters({
			location: { path: "/agents/settings" },
			routing: agentsRouting,
		}),
	},
	play: async () => {
		await userEvent.click(
			await screen.findByRole("link", { name: "Manage Agents" }),
		);

		await expect(
			await screen.findByRole("link", { name: "Providers" }),
		).toBeInTheDocument();
		await expect(
			await screen.findByRole("link", { name: "Spend" }),
		).toBeInTheDocument();
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
				screen.getByText("Personal preferences for your chat experience."),
			).toBeInTheDocument();
		});

		// Navigate to the admin panel, then open the Spend section.
		await userEvent.click(screen.getByRole("link", { name: "Manage Agents" }));
		await userEvent.click(await screen.findByRole("link", { name: "Spend" }));
		await waitFor(() => {
			expect(
				screen.getByText(
					"Configure spend limits and monitor usage across your deployment.",
				),
			).toBeInTheDocument();
		});

		// Step back to the top-level settings panel, then back to conversations.
		const backToSettingsButton = await screen.findByRole("link", {
			name: "Back to Settings",
		});
		await userEvent.click(backToSettingsButton);
		const backToAgentsButton = await screen.findByRole("link", {
			name: "Back to Agents",
		});
		await userEvent.click(backToAgentsButton);

		// Re-open settings, should reset to General
		await openSettingsView(canvasElement);
		await waitFor(() => {
			expect(
				screen.getByText("Personal preferences for your chat experience."),
			).toBeInTheDocument();
		});
	},
};
