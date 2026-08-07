import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
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
import { getAuthorizationKey } from "#/api/queries/authCheck";
import {
	chatEntityKey,
	chatMessagesKey,
	chatPromptsKey,
} from "#/api/queries/chats";
import { permittedOrganizations } from "#/api/queries/organizations";
import type * as TypesGen from "#/api/typesGenerated";
import type { Chat } from "#/api/typesGenerated";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import { MockChat, MockMCPServerConfig } from "#/testHelpers/chatEntities";
import {
	MockDefaultOrganization,
	MockNoPermissions,
	MockOrganization2,
	MockPermissions,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withProxyProvider,
	withWebSocket,
} from "#/testHelpers/storybook";
import { CoderAgentsPageView } from "../AISettingsPage/CoderAgentsPage/CoderAgentsPageView";
import AgentChatPage, { RIGHT_PANEL_OPEN_KEY } from "./AgentChatPage";
import AgentCreatePage from "./AgentCreatePage";
import AgentSettingsCompactionPage from "./AgentSettingsCompactionPage";
import AgentSettingsGeneralPage from "./AgentSettingsGeneralPage";
import AgentSettingsLayout from "./AgentSettingsLayout";
import AgentsPageLayout, {
	type AgentsPageOutletContext,
} from "./AgentsPageLayout";
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

const defaultModelConfigID = "model-config-1";

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

const defaultOrganizationMCPServer: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-default-organization",
	display_name: "Default organization MCP",
	slug: "default-organization-mcp",
};

const secondOrganizationMCPServer: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-second-organization",
	display_name: "Second organization MCP",
	slug: "second-organization-mcp",
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
		providerInfoByID={new Map()}
		modelConfigsError={undefined}
		isLoadingModelConfigs={false}
		isFetchingModelConfigs={false}
		onSaveTitleGenerationModel={fn()}
		isSavingTitleGenerationModel={false}
		isSaveTitleGenerationModelError={false}
		onSaveCompactionModel={fn()}
		isSavingCompactionModel={false}
		isSaveCompactionModelError={false}
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
			element: <AgentSettingsLayout />,
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
			],
		},
		{ path: ":agentId", element: <div /> },
		{ index: true, element: <AgentCreatePage /> },
	],
};

const aiSettingsRouting = {
	path: "/ai/settings",
	children: [{ path: "coder-agents", element: <AgentsRouteElement /> }],
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
		useOutletContext<AgentsPageOutletContext>();
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

const meta: Meta<typeof AgentsPageLayout> = {
	title: "pages/AgentsPage/AgentsPageLayout",
	component: AgentsPageLayout,
	decorators: [withAuthProvider, withDashboardProvider, withWebSocket],
	parameters: {
		layout: "fullscreen",
		// The layout opens a chat-watch WebSocket on mount. An empty
		// event list gives an inert socket that never emits.
		webSocket: [],
		user: MockUserOwner,
		permissions: MockPermissions,
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: [agentsRouting, aiSettingsRouting],
		}),
	},
	args: {},
	beforeEach: () => {
		localStorage.removeItem(LEFT_SIDEBAR_STORAGE_KEY);
		// Mocks for the queries AgentsPageLayout runs for the sidebar.
		spyOn(API.experimental, "getChats").mockResolvedValue([]);
		spyOn(
			API.experimental,
			"getUserChatPersonalModelOverrides",
		).mockResolvedValue({
			enabled: false,
			root: {
				context: "root",
				mode: "deployment_default",
				model_config_id: "",
				is_set: false,
				is_malformed: false,
			},
			general: {
				context: "general",
				mode: "deployment_default",
				model_config_id: "",
				is_set: false,
				is_malformed: false,
			},
			explore: {
				context: "explore",
				mode: "deployment_default",
				model_config_id: "",
				is_set: false,
				is_malformed: false,
			},
			deployment_defaults: {
				general: {
					context: "general",
					model_config_id: "",
					is_malformed: false,
				},
				explore: {
					context: "explore",
					model_config_id: "",
					is_malformed: false,
				},
			},
		});
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [],
			count: 0,
		});
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
					icon: "",
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

		spyOn(API, "getGroups").mockResolvedValue([]);
	},
};

export default meta;
type Story = StoryObj<typeof AgentsPageLayout>;

const mockChats = (chats: Chat[]) => {
	spyOn(API.experimental, "getChats").mockResolvedValue(chats);
};

export const EmptyState: Story = {
	play: async () => {
		await waitFor(() => {
			expect(API.experimental.getMCPServerConfigs).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
			);
		});
	},
};

export const OrganizationScopedMCPServers: Story = {
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
		queries: [
			{
				key: permittedOrganizations({
					object: { resource_type: "chat" },
					action: "create",
				}).queryKey,
				data: [MockDefaultOrganization, MockOrganization2],
			},
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfigs").mockImplementation(
			async (organization) =>
				organization === MockDefaultOrganization.id
					? [defaultOrganizationMCPServer]
					: [secondOrganizationMCPServer],
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await waitFor(() => {
			expect(API.experimental.getMCPServerConfigs).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
			);
		});
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		expect(
			(await body.findAllByText("Default organization MCP")).length,
		).toBeGreaterThan(0);
		await userEvent.keyboard("{Escape}");

		await userEvent.click(
			canvas.getByRole("button", {
				name: `Organization: ${MockDefaultOrganization.display_name}`,
			}),
		);
		await userEvent.click(
			body.getByRole("option", { name: MockOrganization2.display_name }),
		);
		await waitFor(() => {
			expect(API.experimental.getMCPServerConfigs).toHaveBeenCalledWith(
				MockOrganization2.id,
			);
		});
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		expect(
			(await body.findAllByText("Second organization MCP")).length,
		).toBeGreaterThan(0);
		expect(
			body.queryByText("Default organization MCP"),
		).not.toBeInTheDocument();
	},
};

export const WithChatList: Story = {
	beforeEach: () => {
		mockChats([
			buildChat({
				id: "chat-1",
				title: "Refactor authentication module",
				status: "waiting",
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
				status: "requires_action",
				updated_at: todayTimestamp,
			}),
			buildChat({
				id: "chat-6",
				title: "Debug memory leak in worker",
				status: "interrupting",
				updated_at: todayTimestamp,
			}),
		]);
	},
};

export const ResizableSidebar: Story = {
	beforeEach: () => {
		mockChats([
			buildChat({
				id: "chat-resize",
				title: "Resizable sidebar agent",
				updated_at: todayTimestamp,
			}),
		]);
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
	beforeEach: () => {
		mockChats([
			buildChat({
				id: "chat-resize-persisted",
				title: "Persisted sidebar width agent",
				updated_at: todayTimestamp,
			}),
		]);
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
	beforeEach: () => {
		mockChats([
			buildChat({
				id: "chat-wide-sidebar",
				title: "Wide sidebar agent",
				updated_at: todayTimestamp,
			}),
		]);
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
		// CLEANUP: this desktop-at-200%-zoom snapshot still uses the Chromatic
		// viewport param; migrate it to a pixel viewport.
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
	beforeEach: () => {
		mockChats([
			buildChat({
				id: "chat-resize-keyboard",
				title: "Keyboard resizable sidebar agent",
				updated_at: todayTimestamp,
			}),
		]);
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

// The layout never resolves the chats query, so the sidebar stays
// in its loading state.
export const LoadingChats: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChats").mockReturnValue(new Promise(() => {}));
	},
};

export const ChatsLoadError: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChats").mockRejectedValue(
			new Error("Failed to fetch chats"),
		);
	},
};

// The collapsed state is internal to the layout. Drive it through
// the UI, then assert the collapse took effect.
export const SidebarCollapsed: Story = {
	beforeEach: () => {
		mockChats([
			buildChat({
				id: "chat-1",
				title: "Collapsed sidebar agent",
				updated_at: todayTimestamp,
			}),
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Collapse sidebar" }),
		);
		await expect(
			await canvas.findByRole("button", { name: "Expand sidebar" }),
		).toBeVisible();
	},
};

export const EmptyStateZoom200Desktop: Story = {
	parameters: {
		viewport: { defaultViewport: "desktopZoom200" },
		// CLEANUP: this desktop-at-200%-zoom snapshot still uses the Chromatic
		// viewport param; migrate it to a pixel viewport.
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
	parameters: {
		viewport: { defaultViewport: "desktopZoom200" },
		// CLEANUP: this desktop-at-200%-zoom snapshot still uses the Chromatic
		// viewport param; migrate it to a pixel viewport.
		chromatic: { viewports: [720] },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Collapse sidebar" }),
		);
		const expandButton = await canvas.findByRole("button", {
			name: "Expand sidebar",
		});

		await expect(expandButton).toBeVisible();
	},
};

export const CollapsedSidebarZoom200DesktopWithAgent: Story = {
	beforeEach: () => {
		mockChats([
			buildChat({
				id: "chat-1",
				title: "Collapsed sidebar agent",
				updated_at: todayTimestamp,
			}),
		]);
	},
	parameters: {
		viewport: { defaultViewport: "desktopZoom200" },
		// CLEANUP: this desktop-at-200%-zoom snapshot still uses the Chromatic
		// viewport param; migrate it to a pixel viewport.
		chromatic: { viewports: [720] },
		reactRouter: reactRouterParameters({
			location: { path: "/agents/chat-1" },
			routing: agentsWithChatTopBarRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Collapse sidebar" }),
		);
		const expandButton = await canvas.findByRole("button", {
			name: "Expand sidebar",
		});

		await expect(expandButton).toBeVisible();
	},
};

/**
 * Standalone story for the delete-confirmation dialog with
 * agents-specific copy (title, verb, info). The dialog now lives in
 * AgentsPageLayout (the container), so we render it directly here to
 * preserve interaction-test coverage.
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
	beforeEach: () => {
		mockChats([
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
		]);
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

// ---------------------------------------------------------------------------
// Watch-event archive semantics: these stories mount the real AgentChatPage
// under the layout's :agentId route so the layout's chat-watch socket drives
// the page through the production watch path.
// ---------------------------------------------------------------------------

const agentsWithAgentChatPageRouting = {
	...agentsRouting,
	children: agentsRouting.children.map((route) =>
		"path" in route && route.path === ":agentId"
			? { ...route, element: <AgentChatPage /> }
			: route,
	),
};

const WATCHED_CHAT_ID = "chat-watched";

// MockChat is owned by MockUserOwner, so the page renders the owner view
// (composer enabled unless archived) instead of the other-user banner.
const watchedChat = (overrides: Partial<Chat> = {}): Chat => ({
	...MockChat,
	id: WATCHED_CHAT_ID,
	title: "Watched agent",
	last_model_config_id: defaultModelConfigID,
	created_at: oneWeekAgo,
	updated_at: oneWeekAgo,
	...overrides,
});

const watchedChatQueries = (chat: Chat) => [
	{ key: chatEntityKey(chat.id), data: chat },
	{
		key: chatMessagesKey(chat.id),
		data: {
			pages: [{ messages: [], queued_messages: [], has_more: false }],
			pageParams: [undefined],
		},
	},
	{ key: chatPromptsKey(chat.id), data: { prompts: [] } },
	{
		key: getAuthorizationKey({
			checks: {
				canShareChat: {
					object: {
						resource_type: "chat",
						owner_id: chat.owner_id,
						organization_id: chat.organization_id,
					},
					action: "share",
				},
			},
		}),
		data: { canShareChat: true },
	},
];

const chatWatchEvent = (kind: TypesGen.ChatWatchEventKind, chat: Chat) => ({
	event: "message" as const,
	data: JSON.stringify({ kind, chat } satisfies TypesGen.ChatWatchEvent),
});

const watchedChatPageParameters = (
	chat: Chat,
	watchEvents: readonly ReturnType<typeof chatWatchEvent>[],
) => ({
	queries: watchedChatQueries(chat),
	webSocket: {
		"/chats/watch": [...watchEvents],
	},
	reactRouter: reactRouterParameters({
		location: {
			path: `/agents/${WATCHED_CHAT_ID}`,
			pathParams: { agentId: WATCHED_CHAT_ID },
		},
		routing: [agentsWithAgentChatPageRouting, aiSettingsRouting],
	}),
});

const mockAgentChatPageAPIs = () => {
	localStorage.removeItem(RIGHT_PANEL_OPEN_KEY);
	spyOn(API, "getApiKey").mockRejectedValue(new Error("missing API key"));
	spyOn(API.experimental, "updateChat").mockResolvedValue();
	return () => localStorage.removeItem(RIGHT_PANEL_OPEN_KEY);
};

export const ArchiveWatchEventKeepsOpenChatMounted: Story = {
	decorators: [withProxyProvider()],
	beforeEach: () => {
		mockChats([watchedChat()]);
		return mockAgentChatPageAPIs();
	},
	parameters: watchedChatPageParameters(watchedChat(), [
		chatWatchEvent("deleted", watchedChat({ archived: true })),
	]),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("This agent has been archived and is read-only."),
		).toBeVisible();
		await waitFor(() => {
			expect(canvas.getByRole("textbox")).toHaveAttribute(
				"aria-disabled",
				"true",
			);
		});
		expect(canvas.queryByText("Chat not found")).not.toBeInTheDocument();
	},
};

export const UnarchiveWatchEventRecoversArchivedChat: Story = {
	decorators: [withProxyProvider()],
	beforeEach: () => {
		mockChats([watchedChat({ archived: true })]);
		return mockAgentChatPageAPIs();
	},
	parameters: watchedChatPageParameters(watchedChat({ archived: true }), [
		chatWatchEvent("created", watchedChat({ archived: false })),
	]),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByRole("textbox")).not.toHaveAttribute(
				"aria-disabled",
				"true",
			);
		});
		expect(
			canvas.queryByText("This agent has been archived and is read-only."),
		).not.toBeInTheDocument();
		expect(canvas.queryByText("Chat not found")).not.toBeInTheDocument();
	},
};

// Error reasons surface via each chat's last_error, which the
// layout turns into sidebar error badges.
export const WithErrorReasons: Story = {
	beforeEach: () => {
		mockChats([
			buildChat({
				id: "chat-1",
				title: "Rate limited agent",
				status: "error",
				last_error: {
					kind: "generic",
					message: "Model rate limited",
					retryable: false,
				},
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
				last_error: {
					kind: "generic",
					message: "Context window exceeded",
					retryable: false,
				},
				updated_at: todayTimestamp,
			}),
		]);
	},
};

const openSettingsView = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	await userEvent.click(await canvas.findByRole("link", { name: "Settings" }));
};

export const OpensSettingsForAdmins: Story = {
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
	play: async ({ canvasElement }) => {
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
