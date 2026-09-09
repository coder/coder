import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "react-query";
import { MockChat, MockChatMCPApp } from "#/testHelpers/chatEntities";
import {
	MockBuildInfo,
	MockEntitlements,
	MockUserOwner,
} from "#/testHelpers/entities";
import themes from "#/theme";
import { ThemeContextProvider } from "#/theme/context";
import { AgentChatPageView } from "./AgentChatPageView";
import { createChatStore } from "./components/ChatConversation/chatStore";
import { MCPAppTool } from "./components/MCPApps/MCPAppTool";
import {
	getPersistedRightPanelTabs,
	savePersistedRightPanelTabs,
} from "./utils/rightPanelTabStorage";
import type { UserRightPanelTab } from "./utils/rightPanelTabs";
import { getPersistedSidebarTabId } from "./utils/sidebarTabStorage";

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserOwner,
		permissions: { viewAllLicenses: false },
	}),
}));
vi.mock("#/contexts/ProxyContext", () => ({
	useProxy: () => ({ proxy: { preferredWildcardHostname: "" } }),
}));
vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		entitlements: MockEntitlements,
		experiments: ["chat-mcp-apps"],
		buildInfo: MockBuildInfo,
	}),
}));
vi.mock("./components/ChatTopBar", () => ({ ChatTopBar: () => null }));
vi.mock("./components/RightPanel/RightPanel", () => ({
	RightPanel: () => null,
}));
vi.mock("./components/ChatPageContent", () => ({
	ChatPageInput: () => null,
	ChatPageTimeline: () => (
		<MCPAppTool
			app={MockChatMCPApp}
			toolCallId="tool-1"
			name="Sales"
			args={{}}
			fallback={null}
		/>
	),
}));

afterEach(() => localStorage.clear());

it.each([false, true])(
	"opens, activates, and persists one app tab (restored: %s)",
	async (restored) => {
		const onSetShowSidebarPanel = vi.fn();
		const tab: Extract<UserRightPanelTab, { kind: "mcp_app" }> = {
			id: restored ? "restored-app" : "mcp-app-tool-1",
			kind: "mcp_app",
			label: "Sales",
			toolCallId: "tool-1",
			serverName: MockChatMCPApp.server_name,
			resourceUri: MockChatMCPApp.resource_uri,
		};
		if (restored) savePersistedRightPanelTabs(MockChat.id, [tab]);
		render(
			<QueryClientProvider client={new QueryClient()}>
				<ThemeContextProvider theme={themes.dark}>
					<AgentChatPageView
						chat={MockChat}
						persistedError={undefined}
						store={createChatStore()}
						initialMessages={[]}
						editing={{
							chatInputRef: { current: null },
							editorInitialValue: "",
							initialEditorState: undefined,
							remountKey: 0,
							editingMessageId: null,
							editingFileBlocks: [],
							handleEditUserMessage: vi.fn(),
							handleCancelHistoryEdit: vi.fn(),
							handleSendFromInput: vi.fn(),
							handleContentChange: vi.fn(),
						}}
						effectiveSelectedModel=""
						setSelectedModel={vi.fn()}
						modelOptions={[]}
						models={[]}
						modelSelectorPlaceholder="Select a model"
						hasModelOptions={false}
						isInputDisabled={false}
						isSubmissionPending={false}
						isInterruptPending={false}
						showSidebarPanel={false}
						onSetShowSidebarPanel={onSetShowSidebarPanel}
						debugLoggingEnabled={false}
						gitWatcher={{
							repositories: new Map(),
							everDirty: new Set(),
							hasReceivedChanges: true,
							refresh: vi.fn(),
						}}
						sshCommand={undefined}
						handleCommit={vi.fn()}
						handleInterrupt={vi.fn()}
						handleDeleteQueuedMessage={vi.fn()}
						handlePromoteQueuedMessage={vi.fn()}
						hasMoreMessages={false}
						isFetchingMoreMessages={false}
						isHydratingMessages={false}
						hasFetchMoreError={false}
						onFetchMoreMessages={async () => {}}
						mcpServers={[]}
						selectedMCPServerIds={[]}
						onMCPSelectionChange={vi.fn()}
						onMCPAuthComplete={vi.fn()}
						canShareChat={false}
						canConfigureAgentSetup={false}
						providerCount={0}
						modelCount={0}
					/>
				</ThemeContextProvider>
			</QueryClientProvider>,
		);
		const user = userEvent.setup();
		await user.click(screen.getByRole("button", { name: "Open in panel" }));
		await user.click(screen.getByRole("button", { name: "Open in panel" }));
		expect(onSetShowSidebarPanel).toHaveBeenCalledWith(true);
		expect(getPersistedRightPanelTabs(MockChat.id)).toEqual([tab]);
		expect(getPersistedSidebarTabId(MockChat.id)).toBe(tab.id);
	},
);
