import type { Meta, StoryObj } from "@storybook/react-vite";
import dayjs from "dayjs";
import { type ComponentProps, useState } from "react";
import { Navigate, useOutletContext } from "react-router";
import {
	expect,
	fireEvent,
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
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockNoPermissions,
	MockPermissions,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { CoderAgentsPageView } from "../AISettingsPage/CoderAgentsPage/CoderAgentsPageView";
import AgentAnalyticsPage from "./AgentAnalyticsPage";
import AgentCreatePage from "./AgentCreatePage";
import AgentSettingsCompactionPage from "./AgentSettingsCompactionPage";
import AgentSettingsGeneralPage from "./AgentSettingsGeneralPage";
import AgentSettingsPage from "./AgentSettingsPage";
import { type AgentsOutletContext, AgentsPageView } from "./AgentsPageView";
import type { ModelSelectorOption } from "./components/ChatElements";
import {
	AGENTS_MAIN_PANEL_MIN_WIDTH,
	clampLeftSidebarWidth,
	getLeftSidebarMaxWidth,
	LEFT_SIDEBAR_DEFAULT_WIDTH,
	LEFT_SIDEBAR_KEYBOARD_RESIZE_STEP,
	LEFT_SIDEBAR_MIN_WIDTH,
	LEFT_SIDEBAR_STORAGE_KEY,
} from "./components/ChatsSidebar/sidebarWidth";
import { ChatTopBar } from "./components/ChatTopBar";
import type { AgentSidebarFilters } from "./utils/agentSidebarFilters";

const defaultModelConfigID = "model-config-1";

const defaultSidebarFilters: AgentSidebarFilters = {
	archiveStatus: "active",
	groupBy: "date",
	prStatuses: [],
	chatStatuses: ["unread", "read"],
	sources: ["created_by_me"],
};

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
		ai_provider_id: "provider-openai",
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
	...MockChat,
	id: "chat-default",
	owner_id: "owner-1",
	owner_username: "owner",
	owner_name: undefined,
	last_model_config_id: defaultModelConfigs[0].id,
	created_at: oneWeekAgo,
	updated_at: oneWeekAgo,
	...overrides,
});

// Use local noon so the rendered range label stays stable
// across timezones.
const fixedNow = dayjs("2026-03-12T12:00:00");

const AgentsRouteElement = () => (
	<CoderAgentsPageView
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
		providerTypeByID={new Map()}
		modelConfigsError={undefined}
		isLoadingModelConfigs={false}
		isFetchingModelConfigs={false}
		onSaveTitleGenerationModel={fn()}
		isSavingTitleGenerationModel={false}
		isSaveTitleGenerationModelError={false}
		onSaveExploreModelOverride={fn()}
		isSavingExploreModelOverride={false}
		isSaveExploreModelOverrideError={false}
		showAdvisorSettings={false}
		advisorConfigData={undefined}
		isAdvisorConfigLoading={false}
		isAdvisorConfigFetching={false}
		isAdvisorConfigLoadError={false}
		onSaveAdvisorConfig={fn()}
		isSavingAdvisorConfig={false}
		isSaveAdvisorConfigError={false}
		saveAdvisorConfigError={undefined}
		showVirtualDesktopSettings={false}
		computerUseProviderData={undefined}
		isLoadingComputerUseProvider={false}
		onSaveComputerUseProvider={fn()}
		isSavingComputerUseProvider={false}
		computerUseProviderSaveError={null}
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
					element: <Navigate to="/ai/settings/instructions" replace />,
				},
				{
					path: "lifecycle",
					element: <Navigate to="/ai/settings/lifecycle" replace />,
				},
				{
					path: "admin",
					element: <Navigate to="/ai/settings/coder-agents" replace />,
				},
				{
					path: "agents",
					element: <Navigate to="/ai/settings/coder-agents" replace />,
				},
				{
					path: "coder-agents",
					element: <Navigate to="/ai/settings/coder-agents" replace />,
				},
				{
					path: "spend",
					element: <Navigate to="/ai/settings/spend" replace />,
				},
				{
					path: "usage",
					element: <Navigate to="/ai/settings/spend" replace />,
				},
			],
		},
		{ path: "analytics", element: <AgentAnalyticsPage now={fixedNow} /> },
		{ path: ":agentId", element: <div /> },
		{ index: true, element: <AgentCreatePage /> },
	],
};

const aiSettingsRouting = {
	path: "/ai/settings",
	children: [
		{ path: "coder-agents", element: <AgentsRouteElement /> },
		{ path: "spend", element: <div>Spend limits and usage</div> },
	],
};

const setInnerWidthForStory = (width: number) => {
	const descriptor = Object.getOwnPropertyDescriptor(globalThis, "innerWidth");
	Object.defineProperty(globalThis, "innerWidth", {
		configurable: true,
		value: width,
	});

	return () => {
		if (descriptor) {
			Object.defineProperty(globalThis, "innerWidth", descriptor);
			return;
		}

		Reflect.deleteProperty(globalThis, "innerWidth");
	};
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
			onUnarchiveAgent={fn()}
			isSidebarCollapsed={isSidebarCollapsed}
			onToggleSidebarCollapsed={onToggleSidebarCollapsed}
		/>
	);
};

const ChatPaneMinimumRouteElement = () => (
	<div
		data-testid="agents-chat-panel"
		className="flex h-full min-h-0 flex-1 flex-col bg-surface-primary"
		style={{ minWidth: AGENTS_MAIN_PANEL_MIN_WIDTH }}
	>
		<div className="mt-auto px-4 pb-3">
			<div
				data-testid="chat-composer"
				className="flex items-center justify-between rounded-2xl border border-border-default/80 bg-surface-secondary/45 p-2"
			>
				<span className="truncate text-xs text-content-secondary">
					Chat message
				</span>
				<button
					type="button"
					className="size-7 shrink-0 rounded-full border-0 bg-content-link text-content-invert"
				>
					Send
				</button>
			</div>
		</div>
	</div>
);

const agentsWithChatPaneMinimumRouting = {
	...agentsRouting,
	children: agentsRouting.children.map((route) =>
		"path" in route && route.path === ":agentId"
			? { ...route, element: <ChatPaneMinimumRouteElement /> }
			: route,
	),
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
	currentUserId: MockUserOwner.id,
	catalogModelOptions: defaultModelOptions,
	modelConfigs: defaultModelConfigs,
	handleNewAgent: fn(),
	isSearchDialogOpen: false,
	onSearchDialogOpenChange: fn(),
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
	onProposeTitle: fn(async () => "Proposed title"),
	onRenameTitle: fn(async () => {}),
	onToggleSidebarCollapsed: fn(),
	isAgentsAdmin: false,
	sidebarFilters: defaultSidebarFilters,
	onSidebarFiltersChange: fn(),
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
			routing: [agentsRouting, aiSettingsRouting],
		}),
	},
	args: defaultArgs,
	beforeEach: () => {
		localStorage.removeItem(LEFT_SIDEBAR_STORAGE_KEY);
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
			unsupported_providers: [],
		});
		spyOn(API.experimental, "getChatModelConfigs").mockResolvedValue([
			{
				id: defaultModelConfigID,
				ai_provider_id: "provider-openai",
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
		spyOn(API.experimental, "getUserAIProviderKeyConfigs").mockResolvedValue([
			{
				provider: {
					id: "provider-openai",
					type: "openai",
					name: "openai",
					display_name: "OpenAI",
					enabled: true,
					deleted: false,
				},
				has_user_api_key: false,
				has_provider_api_key: true,
				byok_enabled: true,
			},
		]);
		spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
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

export const ResizableSidebar: Story = {
	args: {
		chatList: [
			buildChat({
				id: "chat-resize",
				title: "Resizable sidebar agent",
				updated_at: todayTimestamp,
			}),
		],
	},
	parameters: {
		viewport: { defaultViewport: "ipad" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const sidebar = canvas.getByTestId("agents-sidebar-panel");
		const handle = canvas.getByRole("separator", {
			name: "Resize agents sidebar",
		});

		handle.setPointerCapture = () => {};
		handle.releasePointerCapture = () => {};
		handle.hasPointerCapture = () => true;

		const sidebarWidth = () =>
			sidebar.style.getPropertyValue("--agents-left-sidebar-width");
		const dragSidebar = (fromX: number, toX: number) => {
			fireEvent.pointerDown(handle, { clientX: fromX, pointerId: 1 });
			fireEvent.pointerMove(handle, { clientX: toX, pointerId: 1 });
			fireEvent.pointerUp(handle, { clientX: toX, pointerId: 1 });
		};

		const initialWidth = clampLeftSidebarWidth(LEFT_SIDEBAR_DEFAULT_WIDTH);
		const expandedWidth = Math.min(getLeftSidebarMaxWidth(), initialWidth + 40);

		await expect(handle).toBeVisible();
		await expect(handle).toHaveAttribute("aria-valuenow", String(initialWidth));
		await expect(sidebarWidth()).toBe(`${initialWidth}px`);

		dragSidebar(initialWidth, initialWidth + 40);
		await waitFor(() => {
			expect(sidebarWidth()).toBe(`${expandedWidth}px`);
		});

		dragSidebar(expandedWidth, 0);
		await waitFor(() => {
			expect(sidebarWidth()).toBe(`${LEFT_SIDEBAR_MIN_WIDTH}px`);
		});

		const maxWidth = getLeftSidebarMaxWidth();
		dragSidebar(LEFT_SIDEBAR_MIN_WIDTH, maxWidth + 1000);
		await waitFor(() => {
			expect(sidebarWidth()).toBe(`${maxWidth}px`);
		});
		await waitFor(() => {
			expect(localStorage.getItem(LEFT_SIDEBAR_STORAGE_KEY)).toBe(
				String(maxWidth),
			);
		});
	},
};

const persistedLeftSidebarWidth = 380;

export const PersistedResizableSidebarWidth: Story = {
	args: {
		chatList: [
			buildChat({
				id: "chat-resize-persisted",
				title: "Persisted sidebar width agent",
				updated_at: todayTimestamp,
			}),
		],
	},
	decorators: [
		(Story) => {
			localStorage.setItem(
				LEFT_SIDEBAR_STORAGE_KEY,
				String(persistedLeftSidebarWidth),
			);
			return <Story />;
		},
	],
	parameters: {
		viewport: { defaultViewport: "ipad" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const sidebar = canvas.getByTestId("agents-sidebar-panel");
		const handle = canvas.getByRole("separator", {
			name: "Resize agents sidebar",
		});
		const sidebarWidth = () =>
			sidebar.style.getPropertyValue("--agents-left-sidebar-width");

		await expect(handle).toHaveAttribute(
			"aria-valuenow",
			String(persistedLeftSidebarWidth),
		);
		await expect(sidebarWidth()).toBe(`${persistedLeftSidebarWidth}px`);
	},
};

const narrowAgentsLayoutWidth = 720;

export const WideSidebarPreservesChatPaneWidth: Story = {
	args: {
		agentId: "chat-wide-sidebar",
		chatList: [
			buildChat({
				id: "chat-wide-sidebar",
				title: "Wide sidebar agent",
				updated_at: todayTimestamp,
			}),
		],
	},
	beforeEach: () => {
		localStorage.setItem(LEFT_SIDEBAR_STORAGE_KEY, "660");
		return setInnerWidthForStory(narrowAgentsLayoutWidth);
	},
	decorators: [
		(Story) => (
			<div
				style={{
					height: "100vh",
					overflow: "hidden",
					width: narrowAgentsLayoutWidth,
				}}
			>
				<Story />
			</div>
		),
	],
	parameters: {
		viewport: { defaultViewport: "desktopZoom200" },
		chromatic: { viewports: [720] },
		reactRouter: reactRouterParameters({
			location: { path: "/agents/chat-wide-sidebar" },
			routing: agentsWithChatPaneMinimumRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const layout = await canvas.findByTestId("agents-page-layout");
		const sidebar = await canvas.findByTestId("agents-sidebar-panel");
		const main = await canvas.findByTestId("agents-main-panel");
		const chatPanel = await canvas.findByTestId("agents-chat-panel");
		const composer = await canvas.findByTestId("chat-composer");
		const sendButton = within(composer).getByRole("button", { name: "Send" });

		await waitFor(() => {
			const layoutRect = layout.getBoundingClientRect();
			const sidebarRect = sidebar.getBoundingClientRect();
			const mainRect = main.getBoundingClientRect();
			const chatPanelRect = chatPanel.getBoundingClientRect();
			const composerRect = composer.getBoundingClientRect();
			const sendButtonRect = sendButton.getBoundingClientRect();
			const maxSidebarWidth = layoutRect.width - AGENTS_MAIN_PANEL_MIN_WIDTH;

			expect(layoutRect.width).toBe(narrowAgentsLayoutWidth);
			expect(sidebarRect.width).toBeLessThanOrEqual(maxSidebarWidth + 1);
			expect(mainRect.width).toBeGreaterThanOrEqual(
				AGENTS_MAIN_PANEL_MIN_WIDTH - 1,
			);
			expect(chatPanelRect.width).toBeGreaterThanOrEqual(
				AGENTS_MAIN_PANEL_MIN_WIDTH - 1,
			);
			expect(sendButtonRect.right).toBeLessThanOrEqual(composerRect.right);
			expect(composerRect.right).toBeLessThanOrEqual(layoutRect.right + 1);
		});
	},
};

export const ResizableSidebarKeyboard: Story = {
	args: {
		chatList: [
			buildChat({
				id: "chat-resize-keyboard",
				title: "Keyboard resizable sidebar agent",
				updated_at: todayTimestamp,
			}),
		],
	},
	parameters: {
		viewport: { defaultViewport: "ipad" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const sidebar = canvas.getByTestId("agents-sidebar-panel");
		const handle = canvas.getByRole("separator", {
			name: "Resize agents sidebar",
		});
		const sidebarWidth = () =>
			sidebar.style.getPropertyValue("--agents-left-sidebar-width");
		const initialWidth = clampLeftSidebarWidth(LEFT_SIDEBAR_DEFAULT_WIDTH);
		const keyboardExpandedWidth = Math.min(
			getLeftSidebarMaxWidth(),
			initialWidth + LEFT_SIDEBAR_KEYBOARD_RESIZE_STEP,
		);
		const maxWidth = getLeftSidebarMaxWidth();

		handle.focus();
		await expect(handle).toHaveFocus();

		fireEvent.keyDown(handle, { key: "ArrowRight" });
		await waitFor(() => {
			expect(sidebarWidth()).toBe(`${keyboardExpandedWidth}px`);
			expect(handle).toHaveAttribute(
				"aria-valuenow",
				String(keyboardExpandedWidth),
			);
		});

		fireEvent.keyDown(handle, { key: "Home" });
		await waitFor(() => {
			expect(sidebarWidth()).toBe(`${LEFT_SIDEBAR_MIN_WIDTH}px`);
			expect(handle).toHaveAttribute(
				"aria-valuenow",
				String(LEFT_SIDEBAR_MIN_WIDTH),
			);
		});

		fireEvent.keyDown(handle, { key: "ArrowLeft" });
		await waitFor(() => {
			expect(sidebarWidth()).toBe(`${LEFT_SIDEBAR_MIN_WIDTH}px`);
			expect(handle).toHaveAttribute(
				"aria-valuenow",
				String(LEFT_SIDEBAR_MIN_WIDTH),
			);
		});

		fireEvent.keyDown(handle, { key: "End" });
		await waitFor(() => {
			expect(sidebarWidth()).toBe(`${maxWidth}px`);
			expect(handle).toHaveAttribute("aria-valuenow", String(maxWidth));
		});
		await waitFor(() => {
			expect(localStorage.getItem(LEFT_SIDEBAR_STORAGE_KEY)).toBe(
				String(maxWidth),
			);
		});
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
		await expect(canvas.getByRole("link", { name: "New chat" })).toBeVisible();
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
			routing: [agentsRouting, aiSettingsRouting],
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
	await userEvent.click(await canvas.findByRole("link", { name: "Settings" }));
};

export const OpensAnalyticsForAdmins: Story = {
	args: {
		isAgentsAdmin: true,
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents/analytics" },
			routing: [agentsRouting, aiSettingsRouting],
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
			routing: [agentsRouting, aiSettingsRouting],
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
			screen.queryByRole("link", { name: "Manage agents" }),
		).not.toBeInTheDocument();
	},
};

export const OpensAISettingsFromManageAgentsOnMobile: Story = {
	args: {
		isAgentsAdmin: true,
	},
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		reactRouter: reactRouterParameters({
			location: { path: "/agents/settings" },
			routing: [agentsRouting, aiSettingsRouting],
		}),
	},
	play: async () => {
		const manageAgentsLink = await screen.findByRole("link", {
			name: "Manage agents",
		});
		expect(manageAgentsLink).toHaveAttribute(
			"href",
			"/ai/settings/coder-agents",
		);

		await userEvent.click(manageAgentsLink);

		await expect(
			await screen.findByRole("heading", { name: "Coder Agents" }),
		).toBeInTheDocument();
	},
};

export const SettingsViewCoderAgentsLink: Story = {
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

		const manageAgentsLink = await screen.findByRole("link", {
			name: "Manage agents",
		});
		expect(manageAgentsLink).toHaveAttribute(
			"href",
			"/ai/settings/coder-agents",
		);

		await userEvent.click(manageAgentsLink);

		await waitFor(() => {
			expect(
				screen.getByText(
					"Configure deployment-wide defaults for Coder Agents and agent-specific capabilities.",
				),
			).toBeInTheDocument();
		});
	},
};
