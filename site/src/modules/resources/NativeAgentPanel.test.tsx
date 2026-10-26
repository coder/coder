import {
	agent,
	type InitializeRequest,
	type InitializeResponse,
	type ListSessionsRequest,
	type ListSessionsResponse,
	type LoadSessionRequest,
	type LoadSessionResponse,
	methods,
	type NewSessionRequest,
	type NewSessionResponse,
	PROTOCOL_VERSION,
	type PromptRequest,
	type PromptResponse,
	RequestError,
	type SessionInfo,
	type SetSessionModeRequest,
} from "@agentclientprotocol/sdk";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StrictMode } from "react";
import { QueryClientProvider } from "react-query";
import { meKey } from "#/api/queries/users";
import type { Workspace, WorkspaceAgent } from "#/api/typesGenerated";
import { AuthProvider } from "#/contexts/auth/AuthProvider";
import {
	DashboardContext,
	type DashboardValue,
} from "#/modules/dashboard/DashboardProvider";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import {
	MockArchivedOpenCodeNativeAgentSessionInfo,
	MockBlockedCodexNativeAgentSessionInfo,
	MockBlockedNativeAgentSessionInfo,
	MockCodexNativeAgentSessionInfo,
	MockFaultedNativeAgentSessionInfo,
	MockNativeAgentHarnesses,
	MockNativeAgentInfo,
	MockNativeAgentModes,
	MockNativeAgentMultiplexerInfo,
	MockNativeAgentSessionInfo,
	MockOpenCodeNativeAgentSessionInfo,
	MockOtherNativeAgentSessionInfo,
	MockPendingNativeAgentModes,
	MockRecoveredNativeAgentSessionInfo,
	MockRegisteredNativeAgentSessionInfo,
	MockResumableNativeAgentSessionInfo,
	MockResumePendingNativeAgentSessionInfo,
	MockStaleNativeAgentSessionInfo,
} from "#/testHelpers/nativeAgent";
import { mockNativeAgentServer } from "#/testHelpers/nativeAgentServer";
import {
	createTestQueryClient,
	renderComponent as render,
} from "#/testHelpers/renderHelpers";
import { NativeAgentControls, NativeAgentPanel } from "./NativeAgentPanel";
import {
	nativeAgentPanelStorageKey,
	readNativeAgentPanelState,
	writeNativeAgentPanelState,
} from "./NativeAgentPanelStorage";
import { NativeAgentSession } from "./NativeAgentSession";
import {
	type NativeAgentHarness,
	nativeAgentMetadata,
} from "./nativeAgentMetadata";

function setup(harnesses?: readonly NativeAgentHarness[]) {
	const initialize = vi.fn(
		async (_params: InitializeRequest): Promise<InitializeResponse> => ({
			protocolVersion: PROTOCOL_VERSION,
			agentInfo: harnesses
				? MockNativeAgentMultiplexerInfo
				: MockNativeAgentInfo,
			...(harnesses
				? { _meta: { "coder-acp": { harnesses: [...harnesses] } } }
				: {}),
			agentCapabilities: {
				loadSession: true,
				sessionCapabilities: { list: {} },
			},
		}),
	);
	const list = vi.fn(
		async (_params: ListSessionsRequest): Promise<ListSessionsResponse> => ({
			sessions: [MockNativeAgentSessionInfo],
		}),
	);
	const create = vi.fn(
		async (_params: NewSessionRequest): Promise<NewSessionResponse> => ({
			sessionId: MockNativeAgentSessionInfo.sessionId,
			modes: MockNativeAgentModes,
		}),
	);
	const load = vi.fn(
		async (_params: LoadSessionRequest): Promise<LoadSessionResponse> => ({
			modes: MockNativeAgentModes,
		}),
	);
	const mode = vi.fn(async (_params: SetSessionModeRequest) => ({}));
	const prompt = vi.fn(
		async (_params: PromptRequest): Promise<PromptResponse> => ({
			stopReason: "end_turn",
		}),
	);
	const cancel = vi.fn();
	const server = mockNativeAgentServer(
		agent()
			.onRequest(methods.agent.initialize, ({ params }) => initialize(params))
			.onRequest(methods.agent.session.list, ({ params }) => list(params))
			.onRequest(methods.agent.session.new, ({ params }) => create(params))
			.onRequest(methods.agent.session.load, ({ params }) => load(params))
			.onRequest(methods.agent.session.setMode, ({ params }) => mode(params))
			.onRequest(methods.agent.session.prompt, ({ params }) => prompt(params))
			.onNotification(methods.agent.session.cancel, ({ params }) => {
				cancel(params);
			}),
	);
	return { server, initialize, list, create, load, mode, prompt, cancel };
}

const mockWorkspaceAgent: WorkspaceAgent = {
	...MockWorkspaceAgent,
	expanded_directory: MockNativeAgentSessionInfo.cwd,
};

const mockDashboard: DashboardValue = {
	entitlements: MockEntitlements,
	experiments: ["native-agents"],
	appearance: MockAppearanceConfig,
	buildInfo: MockBuildInfo,
	organizations: [MockDefaultOrganization],
	showOrganizations: false,
	canViewOrganizationSettings: false,
};

function renderControls() {
	return render(
		<StrictMode>
			<QueryClientProvider client={createTestQueryClient()}>
				<NativeAgentControls
					agent={mockWorkspaceAgent}
					workspace={MockWorkspace}
					storageKey={nativeAgentPanelStorageKey(
						MockUserOwner.id,
						MockWorkspace.id,
						mockWorkspaceAgent.name,
					)}
				/>
			</QueryClientProvider>
		</StrictMode>,
	);
}

async function connectAndCreate(mock: ReturnType<typeof setup>) {
	const user = userEvent.setup();
	await waitFor(() => expect(mock.list).toHaveBeenCalled());
	await user.click(screen.getByRole("button", { name: "Add session" }));
	await user.click(screen.getByRole("button", { name: "Create session" }));
	await waitFor(() => expect(mock.mode).toHaveBeenCalled());
	return user;
}

beforeEach(() => {
	sessionStorage.clear();
});

afterEach(() => {
	vi.unstubAllGlobals();
	vi.useRealTimers();
});

it.each([
	"experiment disabled",
	"other owner",
	"browser only",
	"unsupported OS",
	"offline",
])("does not connect when %s", async (constraint) => {
	const mock = setup();
	const queryClient = createTestQueryClient();
	queryClient.setQueryData(meKey, MockUserOwner);
	const dashboard: DashboardValue = {
		...mockDashboard,
		experiments: constraint === "experiment disabled" ? [] : ["native-agents"],
		entitlements: {
			...MockEntitlements,
			has_license: true,
			features: {
				...MockEntitlements.features,
				browser_only: {
					entitlement: "entitled",
					enabled: constraint === "browser only",
				},
			},
		},
	};
	const view = render(
		<QueryClientProvider client={queryClient}>
			<AuthProvider>
				<DashboardContext.Provider value={dashboard}>
					<NativeAgentPanel
						agent={{
							...mockWorkspaceAgent,
							operating_system:
								constraint === "unsupported OS" ? "darwin" : "linux",
							status: constraint === "offline" ? "disconnected" : "connected",
						}}
						workspace={{
							...MockWorkspace,
							owner_id:
								constraint === "other owner"
									? "another-owner"
									: MockWorkspace.owner_id,
						}}
					/>
				</DashboardContext.Provider>
			</AuthProvider>
		</QueryClientProvider>,
	);
	await act(async () => {});
	expect(mock.server.sockets).toHaveLength(0);
	view.unmount();
});

it.each([
	{ harnesses: undefined },
	{ harnesses: [] },
	{ harnesses: MockNativeAgentHarnesses },
	{ harnesses: MockNativeAgentHarnesses.filter((item) => item.id === "codex") },
	{
		harnesses: MockNativeAgentHarnesses.filter(
			(item) => item.id === "opencode",
		),
	},
	{ harnesses: MockNativeAgentHarnesses, selectedHarness: "codex" },
	{ harnesses: MockNativeAgentHarnesses, selectedHarness: "opencode" },
])(
	"discovers automatically and creates only on request in an edited absolute directory: %j",
	async ({ harnesses, selectedHarness }) => {
		const mock = setup(harnesses);
		const harness = selectedHarness ?? harnesses?.[0]?.id;
		const mockCreated =
			harness === "opencode"
				? MockOpenCodeNativeAgentSessionInfo
				: harness === "codex"
					? MockCodexNativeAgentSessionInfo
					: MockNativeAgentSessionInfo;
		mock.create.mockResolvedValue({
			sessionId: mockCreated.sessionId,
			modes: MockNativeAgentModes,
			_meta: mockCreated._meta,
		});
		const view = renderControls();
		const user = userEvent.setup();
		await waitFor(() => expect(mock.list).toHaveBeenCalled());
		expect(mock.server.sockets).toHaveLength(1);
		expect(mock.create).not.toHaveBeenCalled();
		await user.click(screen.getByRole("button", { name: "Add session" }));
		await user.clear(
			await screen.findByRole("textbox", { name: "Working directory" }),
		);
		await user.type(
			screen.getByRole("textbox", { name: "Working directory" }),
			"/home/coder/other",
		);
		if (selectedHarness) {
			await user.click(screen.getByRole("combobox", { name: "Harness" }));
			await user.click(
				screen.getByRole("option", {
					name: selectedHarness === "opencode" ? "OpenCode" : "Codex",
				}),
			);
		}
		if (harnesses && harnesses.length > 1) {
			act(() => mock.server.socket.close(1000));
			await user.click(
				await screen.findByRole("button", { name: "Reconnect" }),
			);
			await waitFor(() =>
				expect(mock.list.mock.calls.length).toBeGreaterThan(1),
			);
		}
		await user.click(screen.getByRole("button", { name: "Create session" }));
		await waitFor(() =>
			expect(mock.mode).toHaveBeenCalledWith({
				sessionId: mockCreated.sessionId,
				modeId: "automation",
			}),
		);
		expect(mock.create).toHaveBeenCalledWith({
			cwd: "/home/coder/other",
			mcpServers: [],
			...(harness ? { _meta: { "coder-acp": { harness } } } : {}),
		});
		expect(mock.initialize).toHaveBeenCalledWith(
			expect.objectContaining({
				protocolVersion: PROTOCOL_VERSION,
				clientCapabilities: {
					auth: { terminal: false },
					terminal: false,
					fs: { readTextFile: false, writeTextFile: false },
				},
			}),
		);
		expect(mock.server.socket.binaryType).toBe("arraybuffer");
		expect(new URL(mock.server.socket.url).pathname).toBe(
			`/api/v2/workspaceagents/${MockWorkspaceAgent.id}/experimental/acp`,
		);
		view.unmount();
		expect(mock.server.socket.readyState).toBe(3);
		expect(mock.cancel).not.toHaveBeenCalled();
	},
);

it("sends a text prompt and cancel notification without replaying it or letting Open interrupt it", async () => {
	const mock = setup();
	mock.list.mockResolvedValue({
		sessions: [MockNativeAgentSessionInfo, MockOtherNativeAgentSessionInfo],
	});
	const open = vi.fn();
	vi.stubGlobal("open", open);
	const completion = createDeferred<PromptResponse>();
	mock.prompt.mockReturnValue(completion.promise);
	const view = renderControls();
	const user = await connectAndCreate(mock);
	await user.type(
		screen.getByRole("textbox", { name: "Prompt" }),
		"Inspect the parser 🚀",
	);
	await user.click(screen.getByRole("button", { name: "Send" }));
	await waitFor(() =>
		expect(mock.prompt).toHaveBeenCalledWith({
			sessionId: MockNativeAgentSessionInfo.sessionId,
			prompt: [{ type: "text", text: "Inspect the parser 🚀" }],
		}),
	);
	await user.click(screen.getByRole("button", { name: "Open" }));
	await user.click(
		screen.getByRole("button", { name: "Open Inspect the next failure" }),
	);
	expect(open).toHaveBeenCalledTimes(2);
	await user.click(
		screen.getByRole("button", {
			name: /^Investigate the failing tests/,
			expanded: true,
		}),
	);
	await user.click(
		screen.getByRole("button", {
			name: /^Investigate the failing tests/,
			expanded: false,
		}),
	);
	expect(mock.server.sockets).toHaveLength(1);
	expect(mock.load).not.toHaveBeenCalled();
	expect(mock.prompt).toHaveBeenCalledTimes(1);
	expect(mock.cancel).not.toHaveBeenCalled();
	expect(mock.mode).toHaveBeenCalledTimes(1);
	await user.click(screen.getByRole("button", { name: "Cancel prompt" }));
	await waitFor(() =>
		expect(mock.cancel).toHaveBeenCalledWith({
			sessionId: MockNativeAgentSessionInfo.sessionId,
		}),
	);
	completion.resolve({ stopReason: "cancelled" });
	act(() => mock.server.socket.close(1000));
	await user.click(await screen.findByRole("button", { name: "Reconnect" }));
	await waitFor(() =>
		expect(mock.load).toHaveBeenCalledWith({
			sessionId: MockNativeAgentSessionInfo.sessionId,
			cwd: MockNativeAgentSessionInfo.cwd,
			mcpServers: [],
		}),
	);
	expect(mock.prompt).toHaveBeenCalledTimes(1);
	expect(mock.server.sockets).toHaveLength(2);
	view.unmount();
});

it.each([
	{
		description: "clears an unchanged submitted prompt",
		editDuringTurn: false,
	},
	{
		description: "preserves a draft edited during a turn",
		editDuringTurn: true,
	},
])("$description before sending the next turn", async ({ editDuringTurn }) => {
	const mock = setup();
	const completion = createDeferred<PromptResponse>();
	mock.prompt.mockReturnValueOnce(completion.promise);
	const view = renderControls();
	const user = await connectAndCreate(mock);
	await user.type(
		screen.getByRole("textbox", { name: "Prompt" }),
		"First prompt",
	);
	await user.click(screen.getByRole("button", { name: "Send" }));
	await waitFor(() => expect(mock.prompt).toHaveBeenCalledTimes(1));
	if (editDuringTurn) {
		await user.clear(screen.getByRole("textbox", { name: "Prompt" }));
		await user.type(
			screen.getByRole("textbox", { name: "Prompt" }),
			"Second prompt",
		);
	}
	const previousLists = mock.list.mock.calls.length;
	await act(async () => completion.resolve({ stopReason: "end_turn" }));
	await waitFor(() =>
		expect(mock.list.mock.calls.length).toBeGreaterThan(previousLists),
	);
	await waitFor(() =>
		expect(
			readNativeAgentPanelState(
				nativeAgentPanelStorageKey(
					MockUserOwner.id,
					MockWorkspace.id,
					mockWorkspaceAgent.name,
				),
			)?.prompt,
		).toBe(editDuringTurn ? "Second prompt" : ""),
	);
	if (!editDuringTurn) {
		await user.type(
			screen.getByRole("textbox", { name: "Prompt" }),
			"Second prompt",
		);
	}
	await user.click(screen.getByRole("button", { name: "Send" }));
	await waitFor(() =>
		expect(mock.prompt).toHaveBeenLastCalledWith({
			sessionId: MockNativeAgentSessionInfo.sessionId,
			prompt: [{ type: "text", text: "Second prompt" }],
		}),
	);
	view.unmount();
});

it("discovers startup sessions on the existing connection and pauses polling after an error", async () => {
	vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
	const mock = setup();
	mock.list.mockResolvedValueOnce({ sessions: [] });
	const view = renderControls();
	const user = userEvent.setup();
	await waitFor(() => expect(mock.list).toHaveBeenCalledTimes(1));
	await act(async () => vi.advanceTimersByTimeAsync(10_000));
	await user.click(
		await screen.findByRole("button", {
			name: /^Investigate the failing tests/,
		}),
	);
	await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(1));
	await user.type(
		screen.getByRole("textbox", { name: "Prompt" }),
		"Keep my draft",
	);
	mock.list.mockRejectedValueOnce(
		new RequestError(-32000, "Broker unavailable"),
	);
	await act(async () => vi.advanceTimersByTimeAsync(10_000));
	await waitFor(() => expect(mock.list).toHaveBeenCalledTimes(3));
	await act(async () => vi.advanceTimersByTimeAsync(60_000));
	expect(mock.list).toHaveBeenCalledTimes(3);
	await user.click(screen.getByRole("button", { name: "Refresh sessions" }));
	await waitFor(() => expect(mock.list).toHaveBeenCalledTimes(4));
	await user.click(screen.getByRole("button", { name: "Send" }));
	await waitFor(() =>
		expect(mock.prompt).toHaveBeenCalledWith({
			sessionId: MockNativeAgentSessionInfo.sessionId,
			prompt: [{ type: "text", text: "Keep my draft" }],
		}),
	);
	expect(mock.load).toHaveBeenCalledTimes(1);
	expect(mock.server.sockets).toHaveLength(1);
	expect(mock.create).not.toHaveBeenCalled();
	view.unmount();
});

it("does not reopen failed ACP on timers and recovers only on request", async () => {
	vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
	const mock = setup();
	mock.initialize.mockRejectedValueOnce(
		new RequestError(-32000, "Sign in to Claude in the terminal"),
	);
	const view = renderControls();
	const user = userEvent.setup();
	await waitFor(() => expect(mock.server.socket.readyState).toBe(3));
	await act(async () => vi.advanceTimersByTimeAsync(60_000));
	expect(mock.initialize).toHaveBeenCalledTimes(1);
	expect(mock.server.sockets).toHaveLength(1);
	await user.click(await screen.findByRole("button", { name: "Reconnect" }));
	await waitFor(() => expect(mock.list).toHaveBeenCalled());
	expect(mock.server.sockets).toHaveLength(2);
	expect(mock.create).not.toHaveBeenCalled();
	view.unmount();
});

it("does not start a deferred connection after unmount", async () => {
	const mock = setup();
	const view = renderControls();
	view.unmount();
	await act(async () => {});
	expect(mock.server.sockets).toHaveLength(0);
});

it("loads three harnesses across pages and switches without creating or reloading replacements", async () => {
	const mock = setup(MockNativeAgentHarnesses);
	mock.list.mockImplementation(async ({ cursor }) =>
		cursor
			? {
					sessions: [
						MockCodexNativeAgentSessionInfo,
						MockOpenCodeNativeAgentSessionInfo,
					],
				}
			: { sessions: [MockNativeAgentSessionInfo], nextCursor: "next" },
	);
	const view = renderControls();
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("button", {
			name: /^Investigate the failing tests.*00c083d0-b72a-4a12-853b-270bf30de904/,
		}),
	);
	await waitFor(() =>
		expect(mock.load).toHaveBeenCalledWith({
			sessionId: MockNativeAgentSessionInfo.sessionId,
			cwd: MockNativeAgentSessionInfo.cwd,
			mcpServers: [],
		}),
	);
	await user.type(
		screen.getByRole("textbox", { name: "Prompt" }),
		"Continue the first session",
	);
	await user.click(
		screen.getByRole("button", {
			name: /^Investigate the failing tests/,
			expanded: true,
		}),
	);
	await user.keyboard("{Enter}");
	expect(mock.load).toHaveBeenCalledTimes(1);
	expect(mock.cancel).not.toHaveBeenCalled();
	expect(mock.mode).not.toHaveBeenCalled();
	expect(mock.prompt).not.toHaveBeenCalled();
	expect(mock.server.sockets).toHaveLength(1);
	expect(mock.server.socket.readyState).toBe(1);
	await user.click(screen.getByRole("button", { name: "More sessions" }));
	for (const info of [
		MockCodexNativeAgentSessionInfo,
		MockOpenCodeNativeAgentSessionInfo,
	]) {
		await user.click(
			await screen.findByRole("button", { name: new RegExp(info.sessionId) }),
		);
		await waitFor(() =>
			expect(mock.load).toHaveBeenLastCalledWith({
				sessionId: info.sessionId,
				cwd: info.cwd,
				mcpServers: [],
			}),
		);
	}
	await user.click(
		screen.getByRole("button", { name: /^Investigate the failing tests/ }),
	);
	await user.click(screen.getByRole("button", { name: "Send" }));
	await waitFor(() =>
		expect(mock.prompt).toHaveBeenCalledWith({
			sessionId: MockNativeAgentSessionInfo.sessionId,
			prompt: [{ type: "text", text: "Continue the first session" }],
		}),
	);
	expect(mock.load).toHaveBeenCalledTimes(3);
	expect(mock.mode).not.toHaveBeenCalled();
	expect(mock.create).not.toHaveBeenCalled();
	view.unmount();
});

const mockClaudeOnlyInitialization: InitializeResponse = {
	protocolVersion: PROTOCOL_VERSION,
	agentInfo: MockNativeAgentInfo,
	agentCapabilities: {
		loadSession: true,
		sessionCapabilities: { list: {} },
	},
	_meta: {
		"coder-acp": {
			harnesses: MockNativeAgentHarnesses.filter(
				(item) => item.id === "claude",
			),
		},
	},
};

it.each([
	{
		name: "legacy adapter",
		harnesses: undefined,
		removesCodex: false,
		losesCapabilities: false,
	},
	{
		name: "unchanged harnesses",
		harnesses: MockNativeAgentHarnesses,
		removesCodex: false,
		losesCapabilities: false,
	},
	{
		name: "Codex removed",
		harnesses: MockNativeAgentHarnesses,
		removesCodex: true,
		losesCapabilities: false,
	},
	{
		name: "capabilities removed",
		harnesses: MockNativeAgentHarnesses,
		removesCodex: true,
		losesCapabilities: true,
	},
])(
	"preserves the open panel, draft, selected session, directory, and harness across workspace agent outages: $name",
	async ({ harnesses, removesCodex, losesCapabilities }) => {
		const mock = setup(harnesses);
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(meKey, MockUserOwner);
		const mockOfflineAgent: WorkspaceAgent = {
			...mockWorkspaceAgent,
			status: "disconnected",
		};
		const mockReconnectedAgent: WorkspaceAgent = {
			...mockWorkspaceAgent,
			expanded_directory: "/home/coder/server-default-changed",
		};
		const panel = (agent: WorkspaceAgent) => (
			<QueryClientProvider client={queryClient}>
				<AuthProvider>
					<DashboardContext.Provider value={mockDashboard}>
						<NativeAgentPanel agent={agent} workspace={MockWorkspace} />
					</DashboardContext.Provider>
				</AuthProvider>
			</QueryClientProvider>
		);
		const view = render(
			panel({ ...mockWorkspaceAgent, lifecycle_state: "starting" }),
		);
		await act(async () => {});
		expect(mock.server.sockets).toHaveLength(0);
		view.rerender(panel(mockWorkspaceAgent));
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", {
				name: /^Investigate the failing tests/,
			}),
		);
		await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(1));
		await user.click(screen.getByRole("button", { name: "Add session" }));
		await user.clear(
			screen.getByRole("textbox", { name: "Working directory" }),
		);
		await user.type(
			screen.getByRole("textbox", { name: "Working directory" }),
			"/home/coder/unsaved-directory",
		);
		await user.type(
			screen.getByRole("textbox", { name: "Prompt" }),
			"Unsent draft survives an outage",
		);
		if (harnesses) {
			await user.click(screen.getByRole("combobox", { name: "Harness" }));
			await user.click(screen.getByRole("option", { name: "Codex" }));
		}
		const previousLists = mock.list.mock.calls.length;
		await user.click(screen.getByRole("button", { name: "Refresh sessions" }));
		await waitFor(() =>
			expect(mock.list.mock.calls.length).toBeGreaterThan(previousLists),
		);
		expect(mock.server.sockets).toHaveLength(1);

		view.rerender(panel(mockOfflineAgent));
		expect(mock.server.socket.readyState).toBe(3);
		await user.click(screen.getByRole("button", { name: "Reconnect" }));
		await user.click(screen.getByRole("button", { name: "Send" }));
		await user.click(screen.getByRole("button", { name: "Create session" }));
		expect(mock.server.sockets).toHaveLength(1);
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(mock.create).not.toHaveBeenCalled();

		if (removesCodex) {
			mock.initialize.mockResolvedValue({
				...mockClaudeOnlyInitialization,
				_meta: losesCapabilities
					? undefined
					: mockClaudeOnlyInitialization._meta,
			});
		}
		view.rerender(panel(mockReconnectedAgent));
		await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(2));
		expect(mock.server.sockets).toHaveLength(2);
		expect(mock.load).toHaveBeenLastCalledWith({
			sessionId: MockNativeAgentSessionInfo.sessionId,
			cwd: MockNativeAgentSessionInfo.cwd,
			mcpServers: [],
		});
		expect(mock.prompt).not.toHaveBeenCalled();
		await user.click(screen.getByRole("button", { name: "Send" }));
		await waitFor(() =>
			expect(mock.prompt).toHaveBeenCalledWith({
				sessionId: MockNativeAgentSessionInfo.sessionId,
				prompt: [{ type: "text", text: "Unsent draft survives an outage" }],
			}),
		);
		if (removesCodex) {
			await user.click(screen.getByRole("button", { name: "Create session" }));
			expect(mock.create).not.toHaveBeenCalled();
			if (losesCapabilities) {
				mock.initialize.mockResolvedValue(mockClaudeOnlyInitialization);
				await user.click(screen.getByRole("button", { name: "Reconnect" }));
				await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(3));
				await user.click(
					screen.getByRole("button", { name: "Create session" }),
				);
				expect(mock.create).not.toHaveBeenCalled();
			}
			await user.click(screen.getByRole("combobox", { name: "Harness" }));
			await user.keyboard("{Escape}{Enter}");
			await user.click(screen.getByRole("option", { name: "Claude Code" }));
			await user.keyboard("{End}-reselected");
		}
		await user.click(screen.getByRole("button", { name: "Create session" }));
		await waitFor(() =>
			expect(mock.create).toHaveBeenCalledWith({
				cwd: removesCodex
					? "/home/coder/unsaved-directory-reselected"
					: "/home/coder/unsaved-directory",
				mcpServers: [],
				...(harnesses
					? {
							_meta: {
								"coder-acp": { harness: removesCodex ? "claude" : "codex" },
							},
						}
					: {}),
			}),
		);
		view.unmount();
	},
);

it.each([
	{ agentID: "rebuilt-agent-id", remount: false },
	{ agentID: "rebuilt-agent-id", remount: true },
	{ agentID: mockWorkspaceAgent.id, remount: true },
])(
	"restores native UI state across rebuilds and panel remounts: %j",
	async ({ agentID, remount }) => {
		const mock = setup(MockNativeAgentHarnesses);
		mock.list.mockResolvedValue({
			sessions: [MockOpenCodeNativeAgentSessionInfo],
		});
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(meKey, MockUserOwner);
		const panel = (agent: WorkspaceAgent) => (
			<QueryClientProvider client={queryClient}>
				<AuthProvider>
					<DashboardContext.Provider value={mockDashboard}>
						<NativeAgentPanel agent={agent} workspace={MockWorkspace} />
					</DashboardContext.Provider>
				</AuthProvider>
			</QueryClientProvider>
		);
		let view = render(panel(mockWorkspaceAgent));
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", {
				name: /^Inspect the OpenCode test run/,
			}),
		);
		await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(1));
		await user.click(screen.getByRole("button", { name: "Add session" }));
		await user.clear(
			screen.getByRole("textbox", { name: "Working directory" }),
		);
		await user.type(
			screen.getByRole("textbox", { name: "Working directory" }),
			"/home/coder/edited",
		);
		await user.click(screen.getByRole("combobox", { name: "Harness" }));
		await user.click(screen.getByRole("option", { name: "OpenCode" }));
		await user.type(
			screen.getByRole("textbox", { name: "Prompt" }),
			"Only send this draft when I ask",
		);
		await user.click(
			screen.getByRole("button", {
				name: /^Inspect the OpenCode test run/,
				expanded: true,
			}),
		);

		expect(
			readNativeAgentPanelState(
				nativeAgentPanelStorageKey(
					MockUserOwner.id,
					MockWorkspace.id,
					mockWorkspaceAgent.name,
				),
			),
		).toEqual({
			cwd: "/home/coder/edited",
			harness: "opencode",
			prompt: "Only send this draft when I ask",
			collapsedSessionID: MockOpenCodeNativeAgentSessionInfo.sessionId,
			selectedSession: {
				sessionId: MockOpenCodeNativeAgentSessionInfo.sessionId,
				cwd: MockOpenCodeNativeAgentSessionInfo.cwd,
			},
		});
		const mockRebuiltAgent = {
			...mockWorkspaceAgent,
			id: agentID,
			expanded_directory: "/home/coder/new-default",
		};
		if (remount) {
			view.unmount();
			view = render(panel(mockRebuiltAgent));
		} else {
			view.rerender(panel(mockRebuiltAgent));
		}
		await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(2));
		expect(mock.server.socket.url).toContain(mockRebuiltAgent.id);
		expect(mock.load).toHaveBeenLastCalledWith({
			sessionId: MockOpenCodeNativeAgentSessionInfo.sessionId,
			cwd: MockOpenCodeNativeAgentSessionInfo.cwd,
			mcpServers: [],
		});
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(mock.create).not.toHaveBeenCalled();
		expect(mock.mode).not.toHaveBeenCalled();
		expect(mock.cancel).not.toHaveBeenCalled();
		await user.click(
			screen.getByRole("button", {
				name: /^Inspect the OpenCode test run/,
				expanded: false,
			}),
		);
		await user.click(screen.getByRole("button", { name: "Send" }));
		await waitFor(() =>
			expect(mock.prompt).toHaveBeenCalledExactlyOnceWith({
				sessionId: MockOpenCodeNativeAgentSessionInfo.sessionId,
				prompt: [{ type: "text", text: "Only send this draft when I ask" }],
			}),
		);
		await user.click(screen.getByRole("button", { name: "Add session" }));
		await user.click(screen.getByRole("button", { name: "Create session" }));
		await waitFor(() =>
			expect(mock.create).toHaveBeenCalledExactlyOnceWith({
				cwd: "/home/coder/edited",
				mcpServers: [],
				_meta: { "coder-acp": { harness: "opencode" } },
			}),
		);
		view.unmount();
	},
);

it.each(["workspace", "user", "agent"])(
	"isolates saved native UI state by %s",
	async (scope) => {
		const mock = setup(MockNativeAgentHarnesses);
		mock.list.mockResolvedValue({
			sessions: [MockOpenCodeNativeAgentSessionInfo],
		});
		const key = nativeAgentPanelStorageKey(
			MockUserOwner.id,
			MockWorkspace.id,
			mockWorkspaceAgent.name,
		);
		const mockSavedState = {
			cwd: "/home/coder/private-directory",
			harness: "opencode",
			prompt: "Private unsent draft",
			selectedSession: {
				sessionId: MockOpenCodeNativeAgentSessionInfo.sessionId,
				cwd: MockOpenCodeNativeAgentSessionInfo.cwd,
			},
			collapsedSessionID: MockOpenCodeNativeAgentSessionInfo.sessionId,
		};
		writeNativeAgentPanelState(key, mockSavedState);
		const mockUser = {
			...MockUserOwner,
			id: scope === "user" ? "other-user" : MockUserOwner.id,
		};
		const mockWorkspace = {
			...MockWorkspace,
			id: scope === "workspace" ? "other-workspace" : MockWorkspace.id,
			owner_id: mockUser.id,
		};
		const mockAgent = {
			...mockWorkspaceAgent,
			name: scope === "agent" ? "other-agent" : mockWorkspaceAgent.name,
		};
		const queryClient = createTestQueryClient();
		queryClient.setQueryDefaults(meKey, {
			staleTime: Number.POSITIVE_INFINITY,
		});
		queryClient.setQueryData(meKey, MockUserOwner);
		const panel = (agent: WorkspaceAgent, workspace: Workspace) => (
			<QueryClientProvider client={queryClient}>
				<AuthProvider>
					<DashboardContext.Provider value={mockDashboard}>
						<NativeAgentPanel agent={agent} workspace={workspace} />
					</DashboardContext.Provider>
				</AuthProvider>
			</QueryClientProvider>
		);
		const view = render(panel(mockWorkspaceAgent, MockWorkspace));
		await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(1));
		const previousLists = mock.list.mock.calls.length;
		act(() => queryClient.setQueryData(meKey, mockUser));
		view.rerender(panel(mockAgent, mockWorkspace));
		const user = userEvent.setup();
		await waitFor(() =>
			expect(mock.list.mock.calls.length).toBeGreaterThan(previousLists),
		);
		expect(mock.load).toHaveBeenCalledTimes(1);
		await user.click(
			await screen.findByRole("button", {
				name: /^Inspect the OpenCode test run/,
			}),
		);
		await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(2));
		await user.type(
			screen.getByRole("textbox", { name: "Prompt" }),
			"New scope draft",
		);
		await user.click(screen.getByRole("button", { name: "Send" }));
		await waitFor(() =>
			expect(mock.prompt).toHaveBeenCalledExactlyOnceWith({
				sessionId: MockOpenCodeNativeAgentSessionInfo.sessionId,
				prompt: [{ type: "text", text: "New scope draft" }],
			}),
		);
		await user.click(screen.getByRole("button", { name: "Add session" }));
		await user.click(screen.getByRole("button", { name: "Create session" }));
		await waitFor(() =>
			expect(mock.create).toHaveBeenCalledExactlyOnceWith({
				cwd: mockWorkspaceAgent.expanded_directory,
				mcpServers: [],
				_meta: { "coder-acp": { harness: "claude" } },
			}),
		);
		expect(readNativeAgentPanelState(key)).toEqual(mockSavedState);
		view.unmount();
	},
);

it("keeps live input usable when session storage is denied", async () => {
	vi.stubGlobal("sessionStorage", {
		getItem: () => {
			throw new DOMException("Storage denied", "SecurityError");
		},
		setItem: () => {
			throw new DOMException("Storage full", "QuotaExceededError");
		},
	});
	const mock = setup();
	const view = renderControls();
	const user = await connectAndCreate(mock);
	await user.type(
		screen.getByRole("textbox", { name: "Prompt" }),
		"Keep editing without storage",
	);
	await user.click(screen.getByRole("button", { name: "Send" }));
	await waitFor(() =>
		expect(mock.prompt).toHaveBeenCalledExactlyOnceWith({
			sessionId: MockNativeAgentSessionInfo.sessionId,
			prompt: [{ type: "text", text: "Keep editing without storage" }],
		}),
	);
	view.unmount();
});

it("ignores stale stored values while the user is editing", async () => {
	const mock = setup();
	const view = renderControls();
	const user = await connectAndCreate(mock);
	await user.type(
		screen.getByRole("textbox", { name: "Prompt" }),
		"Current draft",
	);
	writeNativeAgentPanelState(
		nativeAgentPanelStorageKey(
			MockUserOwner.id,
			MockWorkspace.id,
			mockWorkspaceAgent.name,
		),
		{ cwd: "/stale", harness: "stale", prompt: "Stale draft" },
	);
	const previousLists = mock.list.mock.calls.length;
	await user.click(screen.getByRole("button", { name: "Refresh sessions" }));
	await waitFor(() =>
		expect(mock.list.mock.calls.length).toBeGreaterThan(previousLists),
	);
	await user.type(
		screen.getByRole("textbox", { name: "Prompt" }),
		" keeps growing",
	);
	await user.click(screen.getByRole("button", { name: "Send" }));
	await waitFor(() =>
		expect(mock.prompt).toHaveBeenCalledExactlyOnceWith({
			sessionId: MockNativeAgentSessionInfo.sessionId,
			prompt: [{ type: "text", text: "Current draft keeps growing" }],
		}),
	);
	view.unmount();
});

it.each([
	{
		mockInfo: {
			...MockNativeAgentSessionInfo,
			sessionId: "session'; touch /tmp/unsafe; echo '",
		},
		quotedID: "'session'\\''; touch /tmp/unsafe; echo '\\'''",
	},
	{
		mockInfo: MockCodexNativeAgentSessionInfo,
		quotedID: "'codex:4a7ebd77-9156-489c-ae77-a82080a15a00'",
	},
	{
		mockInfo: MockOpenCodeNativeAgentSessionInfo,
		quotedID: "'opencode:ses_01JOpenCodeAbC123'",
	},
])(
	"opens a safely quoted terminal command without ACP side effects and can reopen under human control: $mockInfo.sessionId",
	async ({ mockInfo, quotedID }) => {
		const mock = setup(MockNativeAgentHarnesses);
		const { sessionId } = mockInfo;
		mock.list.mockResolvedValue({ sessions: [mockInfo] });
		const popup = { close: vi.fn() };
		const open = vi.fn<
			(...args: Parameters<typeof window.open>) => typeof popup
		>(() => popup);
		vi.stubGlobal("open", open);
		const view = renderControls();
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", {
				name: new RegExp(mockInfo.title ?? sessionId),
				expanded: false,
			}),
		);
		await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(1));
		await user.type(
			screen.getByRole("textbox", { name: "Prompt" }),
			"Keep my draft",
		);
		const previousLists = mock.list.mock.calls.length;
		await user.click(screen.getByRole("button", { name: "Open" }));
		const href = open.mock.calls[0]?.[0];
		if (!href) throw new Error("Expected a terminal URL");
		const url = new URL(href, location.origin);
		expect(url.pathname).toBe(
			`/@${MockWorkspace.owner_name}/${MockWorkspace.name}.${MockWorkspaceAgent.name}/terminal`,
		);
		expect(url.searchParams.get("command")).toBe(
			`coder-acp attach ${quotedID}; exec "\${SHELL:-/bin/sh}" -l`,
		);
		expect(open).toHaveBeenCalledWith(href, "_blank", "noopener,noreferrer");
		expect(mock.load).toHaveBeenCalledTimes(1);
		expect(mock.mode).not.toHaveBeenCalled();
		expect(mock.list).toHaveBeenCalledTimes(previousLists);
		await act(async () => {
			await mock.server.socket.connection.client.notify(
				methods.client.session.update,
				{
					sessionId,
					update: {
						sessionUpdate: "current_mode_update",
						currentModeId: "human",
					},
				},
			);
		});
		await screen.findByRole("button", { name: "Observe" });
		await user.click(screen.getByRole("button", { name: "Open" }));
		await user.click(screen.getByRole("button", { name: "Open" }));
		expect(open).toHaveBeenCalledTimes(3);
		expect(open).toHaveBeenLastCalledWith(
			href,
			"_blank",
			"noopener,noreferrer",
		);
		expect(mock.mode).not.toHaveBeenCalled();
		expect(mock.load).toHaveBeenCalledTimes(1);
		expect(mock.create).not.toHaveBeenCalled();
		expect(mock.prompt).not.toHaveBeenCalled();
		await user.click(screen.getByRole("button", { name: "Observe" }));
		await waitFor(() =>
			expect(mock.mode).toHaveBeenCalledExactlyOnceWith({
				sessionId,
				modeId: "automation",
			}),
		);
		expect(open).toHaveBeenCalledTimes(3);
		expect(popup.close).not.toHaveBeenCalled();
		expect(mock.cancel).not.toHaveBeenCalled();
		await user.click(screen.getByRole("button", { name: "Send" }));
		await waitFor(() =>
			expect(mock.prompt).toHaveBeenCalledWith({
				sessionId,
				prompt: [{ type: "text", text: "Keep my draft" }],
			}),
		);
		view.unmount();
	},
);

it.each([false, true])(
	"opens the terminal while ACP is disconnected without reconnecting or changing control (selected: %s)",
	async (selected) => {
		const mock = setup();
		const open = vi.fn();
		vi.stubGlobal("open", open);
		const view = renderControls();
		const user = userEvent.setup();
		const card = await screen.findByRole("button", {
			name: /^Investigate the failing tests/,
		});
		if (selected) {
			await user.click(card);
			await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(1));
		}
		act(() => mock.server.socket.close(1000));
		await screen.findByRole("button", { name: "Reconnect" });
		await user.click(
			screen.getByRole("button", {
				name: selected ? "Open" : "Open Investigate the failing tests",
			}),
		);
		expect(open).toHaveBeenCalledExactlyOnceWith(
			expect.stringContaining("/terminal?"),
			"_blank",
			"noopener,noreferrer",
		);
		expect(mock.server.sockets).toHaveLength(1);
		expect(mock.load).toHaveBeenCalledTimes(selected ? 1 : 0);
		expect(mock.mode).not.toHaveBeenCalled();
		expect(mock.create).not.toHaveBeenCalled();
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(mock.cancel).not.toHaveBeenCalled();
		view.unmount();
	},
);

it.each(
	[
		MockNativeAgentSessionInfo,
		MockBlockedNativeAgentSessionInfo,
		MockResumableNativeAgentSessionInfo,
		MockResumePendingNativeAgentSessionInfo,
		MockFaultedNativeAgentSessionInfo,
		MockOpenCodeNativeAgentSessionInfo,
		MockArchivedOpenCodeNativeAgentSessionInfo,
	].flatMap((info) =>
		["unselected", "selected", "collapsed"].map((selection) => ({
			info,
			selection,
		})),
	),
)(
	"opens a $selection session ($info.title) without loading, changing mode, or restarting it",
	async ({ info, selection }) => {
		const mock = setup();
		mock.list.mockResolvedValue({ sessions: [info] });
		mock.load.mockResolvedValue({ modes: MockPendingNativeAgentModes });
		const open = vi.fn<typeof window.open>(() => null);
		vi.stubGlobal("open", open);
		const view = renderControls();
		const user = userEvent.setup();
		if (selection !== "unselected") {
			await user.click(
				await screen.findByRole("button", { name: new RegExp(info.sessionId) }),
			);
			await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(1));
			if (selection === "collapsed") {
				await user.click(
					screen.getByRole("button", {
						name: new RegExp(info.sessionId),
						expanded: true,
					}),
				);
			}
		}
		const openButton = await screen.findByRole("button", {
			name:
				selection === "selected" ? "Open" : `Open ${info.title ?? "OpenCode"}`,
		});
		const previousLists = mock.list.mock.calls.length;
		openButton.focus();
		await user.keyboard("{Enter}{Enter}");
		expect(open).toHaveBeenCalledTimes(2);
		const href = open.mock.calls[0]?.[0];
		if (!href) throw new Error("Expected a terminal URL");
		expect(new URL(href, location.origin).searchParams.get("command")).toBe(
			`coder-acp attach '${info.sessionId}'; exec "\${SHELL:-/bin/sh}" -l`,
		);
		expect(open).toHaveBeenLastCalledWith(
			href,
			"_blank",
			"noopener,noreferrer",
		);
		expect(mock.list).toHaveBeenCalledTimes(previousLists);
		expect(mock.load).toHaveBeenCalledTimes(selection === "unselected" ? 0 : 1);
		expect(mock.mode).not.toHaveBeenCalled();
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(mock.create).not.toHaveBeenCalled();
		expect(mock.cancel).not.toHaveBeenCalled();
		view.unmount();
	},
);

it.each(["load pending", "mode pending", "modes unavailable"])(
	"opens a terminal independently when %s",
	async (state) => {
		const mock = setup();
		mock.list.mockResolvedValue({ sessions: [MockNativeAgentSessionInfo] });
		const loaded = createDeferred<LoadSessionResponse>();
		const acknowledged = createDeferred<Record<string, never>>();
		mock.load.mockReturnValue(
			state === "load pending"
				? loaded.promise
				: Promise.resolve({
						modes:
							state === "modes unavailable"
								? undefined
								: { ...MockNativeAgentModes, currentModeId: "human" },
					}),
		);
		mock.mode.mockReturnValue(acknowledged.promise);
		const open = vi.fn();
		vi.stubGlobal("open", open);
		const view = renderControls();
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", {
				name: /^Investigate the failing tests/,
			}),
		);
		await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(1));
		if (state === "mode pending") {
			await user.click(await screen.findByRole("button", { name: "Observe" }));
			await waitFor(() => expect(mock.mode).toHaveBeenCalledTimes(1));
		}
		await user.click(screen.getByRole("button", { name: "Open" }));
		expect(open).toHaveBeenCalledExactlyOnceWith(
			expect.stringContaining("/terminal?"),
			"_blank",
			"noopener,noreferrer",
		);
		expect(mock.load).toHaveBeenCalledTimes(1);
		expect(mock.mode).toHaveBeenCalledTimes(state === "mode pending" ? 1 : 0);
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(mock.create).not.toHaveBeenCalled();
		expect(mock.cancel).not.toHaveBeenCalled();
		await act(async () => {
			loaded.resolve({ modes: MockNativeAgentModes });
			acknowledged.resolve({});
		});
		view.unmount();
	},
);

it.each(["poll", "notification"])(
	"recovers the same setup session via %s without reopening, changing control, or resending a prompt",
	async (source) => {
		if (source === "poll")
			vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
		const mock = setup();
		mock.list.mockResolvedValue({
			sessions: [MockBlockedNativeAgentSessionInfo],
		});
		mock.load.mockResolvedValue({ modes: MockPendingNativeAgentModes });
		const view = renderControls();
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", {
				name: /Set up the project agent.*00c083d0/,
			}),
		);
		await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(1));
		await user.type(
			screen.getByRole("textbox", { name: "Prompt" }),
			"Submit this draft only when I ask",
		);
		await user.click(screen.getByRole("button", { name: "Send" }));
		expect(mock.prompt).not.toHaveBeenCalled();
		await user.click(
			screen.getByRole("button", {
				name: /Set up the project agent.*00c083d0/,
				expanded: true,
			}),
		);
		mock.list.mockResolvedValue({
			sessions: [MockRegisteredNativeAgentSessionInfo],
		});
		await act(async () => {
			if (source === "poll") {
				await vi.advanceTimersByTimeAsync(10_000);
			} else {
				await mock.server.socket.connection.client.notify(
					methods.client.session.update,
					{
						sessionId: MockRegisteredNativeAgentSessionInfo.sessionId,
						update: {
							sessionUpdate: "session_info_update",
							_meta: { "claude-acp-bridge": { registrationPending: false } },
						},
					},
				);
			}
		});
		await user.click(
			screen.getByRole("button", {
				name: /Set up the project agent.*00c083d0/,
				expanded: false,
			}),
		);
		await user.click(screen.getByRole("button", { name: "Send" }));
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(mock.mode).not.toHaveBeenCalled();
		await user.click(await screen.findByRole("button", { name: "Observe" }));
		await waitFor(() =>
			expect(mock.mode).toHaveBeenCalledWith({
				sessionId: MockRegisteredNativeAgentSessionInfo.sessionId,
				modeId: "automation",
			}),
		);
		await user.click(screen.getByRole("button", { name: "Send" }));
		await waitFor(() =>
			expect(mock.prompt).toHaveBeenCalledWith({
				sessionId: MockRegisteredNativeAgentSessionInfo.sessionId,
				prompt: [{ type: "text", text: "Submit this draft only when I ask" }],
			}),
		);
		expect(mock.prompt).toHaveBeenCalledTimes(1);
		expect(mock.load).toHaveBeenCalledTimes(1);
		expect(mock.create).not.toHaveBeenCalled();
		expect(mock.server.sockets).toHaveLength(1);
		view.unmount();
	},
);

it.each(["discovery refresh", "notification"])(
	"preserves drafts and collapse through startup restore via %s and waits for Observe before sending",
	async (source) => {
		const mock = setup();
		mock.list.mockResolvedValue({
			sessions: [MockResumableNativeAgentSessionInfo],
		});
		mock.load.mockResolvedValue({ modes: MockPendingNativeAgentModes });
		vi.stubGlobal("open", vi.fn());
		const view = renderControls();
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", {
				name: /^Investigate the failing tests.*00c083d0/,
			}),
		);
		await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(1));
		await user.type(
			screen.getByRole("textbox", { name: "Prompt" }),
			"Continue from the saved conversation",
		);
		await user.click(screen.getByRole("button", { name: "Add session" }));
		await user.clear(
			screen.getByRole("textbox", { name: "Working directory" }),
		);
		await user.type(
			screen.getByRole("textbox", { name: "Working directory" }),
			"/home/coder/edited",
		);
		await user.click(screen.getByRole("button", { name: "Send" }));
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(mock.mode).not.toHaveBeenCalled();

		await user.click(screen.getByRole("button", { name: "Open" }));
		expect(window.open).toHaveBeenCalledExactlyOnceWith(
			expect.stringContaining("/terminal?"),
			"_blank",
			"noopener,noreferrer",
		);
		expect(mock.mode).not.toHaveBeenCalled();
		mock.list.mockResolvedValue({
			sessions: [MockResumePendingNativeAgentSessionInfo],
		});
		await act(async () => {
			await mock.server.socket.connection.client.notify(
				methods.client.session.update,
				{
					sessionId: MockResumableNativeAgentSessionInfo.sessionId,
					update: {
						sessionUpdate: "session_info_update",
						_meta: MockResumePendingNativeAgentSessionInfo._meta,
					},
				},
			);
		});
		await user.click(screen.getByRole("button", { name: "Send" }));
		expect(mock.prompt).not.toHaveBeenCalled();
		await user.click(
			screen.getByRole("button", {
				name: /^Investigate the failing tests.*00c083d0/,
				expanded: true,
			}),
		);

		const mockRegisteredInfo = {
			...MockResumePendingNativeAgentSessionInfo,
			_meta: MockRegisteredNativeAgentSessionInfo._meta,
		};
		mock.list.mockResolvedValue({ sessions: [mockRegisteredInfo] });
		if (source === "discovery refresh") {
			await user.click(
				screen.getByRole("button", { name: "Refresh sessions" }),
			);
		} else {
			await act(async () => {
				await mock.server.socket.connection.client.notify(
					methods.client.session.update,
					{
						sessionId: mockRegisteredInfo.sessionId,
						update: {
							sessionUpdate: "session_info_update",
							_meta: {
								"claude-acp-bridge": {
									registrationPending: false,
									resumePending: false,
								},
							},
						},
					},
				);
			});
		}
		await user.click(
			screen.getByRole("button", {
				name: /^Investigate the failing tests.*00c083d0/,
				expanded: false,
			}),
		);
		await user.click(screen.getByRole("button", { name: "Send" }));
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(mock.mode).not.toHaveBeenCalled();
		await user.click(await screen.findByRole("button", { name: "Observe" }));
		await waitFor(() =>
			expect(mock.mode).toHaveBeenCalledExactlyOnceWith({
				sessionId: mockRegisteredInfo.sessionId,
				modeId: "automation",
			}),
		);
		await user.click(screen.getByRole("button", { name: "Send" }));
		await waitFor(() =>
			expect(mock.prompt).toHaveBeenCalledWith({
				sessionId: mockRegisteredInfo.sessionId,
				prompt: [
					{ type: "text", text: "Continue from the saved conversation" },
				],
			}),
		);
		expect(mock.prompt).toHaveBeenCalledTimes(1);
		expect(mock.load).toHaveBeenCalledTimes(1);
		expect(mock.create).not.toHaveBeenCalled();
		expect(mock.cancel).not.toHaveBeenCalled();
		expect(mock.server.sockets).toHaveLength(1);
		await user.click(screen.getByRole("button", { name: "Create session" }));
		await waitFor(() =>
			expect(mock.create).toHaveBeenCalledWith({
				cwd: "/home/coder/edited",
				mcpServers: [],
			}),
		);
		view.unmount();
	},
);

it("creates a pending native session without requesting automation or submitting a prompt", async () => {
	const mock = setup();
	mock.create.mockResolvedValue({
		sessionId: MockBlockedNativeAgentSessionInfo.sessionId,
		modes: MockPendingNativeAgentModes,
		_meta: MockBlockedNativeAgentSessionInfo._meta,
	});
	mock.list.mockResolvedValue({
		sessions: [MockBlockedNativeAgentSessionInfo],
	});
	const view = renderControls();
	const user = userEvent.setup();
	await waitFor(() => expect(mock.list).toHaveBeenCalled());
	await user.click(screen.getByRole("button", { name: "Add session" }));
	await user.click(screen.getByRole("button", { name: "Create session" }));
	await user.type(
		await screen.findByRole("textbox", { name: "Prompt" }),
		"Wait for setup",
	);
	await user.click(screen.getByRole("button", { name: "Send" }));
	expect(mock.mode).not.toHaveBeenCalled();
	expect(mock.prompt).not.toHaveBeenCalled();
	expect(mock.create).toHaveBeenCalledTimes(1);
	view.unmount();
});

it.each([
	MockBlockedNativeAgentSessionInfo,
	MockResumePendingNativeAgentSessionInfo,
	{
		...MockResumePendingNativeAgentSessionInfo,
		_meta: {
			"claude-acp-bridge": { resumePending: true, registrationPending: false },
		},
	},
])(
	"rejects automation and prompts while setup or resume is pending despite reported automation: $_meta",
	async (info) => {
		const mock = setup();
		const session = new NativeAgentSession(MockWorkspaceAgent.id);
		await session.connect();
		await session.load(info);
		await expect(session.setMode("automation")).rejects.toThrow(
			"complete setup",
		);
		await expect(session.prompt("Do not send")).rejects.toThrow();
		expect(mock.mode).not.toHaveBeenCalled();
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(session.getSnapshot().session?.entries).toEqual([]);
		session.close();
	},
);

it.each([undefined, false, "true", true])(
	"never changes control of unavailable history, regardless of resume capability: %s",
	async (resumable) => {
		const mock = setup();
		const session = new NativeAgentSession(MockWorkspaceAgent.id);
		await session.connect();
		await session.load({
			...MockResumableNativeAgentSessionInfo,
			_meta: {
				"claude-acp-bridge": {
					archived: true,
					ended: true,
					fault: true,
					resumable,
				},
			},
		});
		await expect(session.setMode("automation")).rejects.toThrow("unavailable");
		await expect(session.prompt("Do not replay")).rejects.toThrow();
		await expect(session.setMode("human")).rejects.toThrow("unavailable");
		expect(mock.mode).not.toHaveBeenCalled();
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(session.getSnapshot().session?.entries).toEqual([]);
		session.close();
	},
);

it.each([
	{
		info: MockBlockedNativeAgentSessionInfo,
		namespace: "claude-acp-bridge",
		harness: "claude",
	},
	{
		info: MockBlockedCodexNativeAgentSessionInfo,
		namespace: "coder-acp",
		harness: "codex",
	},
])(
	"promotes $namespace setup metadata without replacing identity, history, or human control",
	async ({ info, namespace, harness }) => {
		const mock = setup();
		mock.load.mockResolvedValue({ modes: MockPendingNativeAgentModes });
		const session = new NativeAgentSession(MockWorkspaceAgent.id);
		await session.connect();
		await session.load(info);
		const entries = session.getSnapshot().session?.entries;
		await mock.server.socket.connection.client.notify(
			methods.client.session.update,
			{
				sessionId: info.sessionId,
				update: {
					sessionUpdate: "session_info_update",
					_meta: { [namespace]: { pendingInput: true } },
				},
			},
		);
		await waitFor(() =>
			expect(session.getSnapshot().session?.needsInput).toBe(true),
		);
		await mock.server.socket.connection.client.notify(
			methods.client.session.update,
			{
				sessionId: info.sessionId,
				update: {
					sessionUpdate: "session_info_update",
					_meta: {
						[namespace]: { blockedReason: "Waiting for native approval." },
					},
				},
			},
		);
		await waitFor(() =>
			expect(session.getSnapshot().session).toMatchObject({
				info: {
					_meta: {
						[namespace]: { blockedReason: "Waiting for native approval." },
					},
				},
				needsInput: true,
			}),
		);
		for (const pendingInput of [null, true]) {
			await mock.server.socket.connection.client.notify(
				methods.client.session.update,
				{
					sessionId: info.sessionId,
					update: {
						sessionUpdate: "session_info_update",
						_meta: { [namespace]: { pendingInput } },
					},
				},
			);
			await waitFor(() =>
				expect(session.getSnapshot().session?.needsInput).toBe(
					Boolean(pendingInput),
				),
			);
		}
		await mock.server.socket.connection.client.notify(
			methods.client.session.update,
			{
				sessionId: info.sessionId,
				update: {
					sessionUpdate: "session_info_update",
					_meta: { [namespace]: { registrationPending: false } },
				},
			},
		);
		await waitFor(() =>
			expect(session.getSnapshot().session).toMatchObject({
				info: {
					sessionId: info.sessionId,
					cwd: info.cwd,
					_meta: {
						[namespace]: {
							registrationPending: false,
							startedAt: 1_792_756_800_000,
						},
					},
				},
				needsInput: false,
				modes: { currentModeId: "human" },
			}),
		);
		expect(
			nativeAgentMetadata(session.getSnapshot().session?.info).harness,
		).toBe(harness);
		expect(session.getSnapshot().session?.entries).toBe(entries);
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(mock.mode).not.toHaveBeenCalled();
		expect(mock.load).toHaveBeenCalledTimes(1);
		session.close();
	},
);

it("honors native mode updates and continues after switching back to automation", async () => {
	const mock = setup();
	const view = renderControls();
	const user = await connectAndCreate(mock);
	await user.type(screen.getByRole("textbox", { name: "Prompt" }), "Continue");
	await user.click(
		screen.getByRole("button", {
			name: /^Investigate the failing tests/,
			expanded: true,
		}),
	);
	await act(async () => {
		await mock.server.socket.connection.client.notify(
			methods.client.session.update,
			{
				sessionId: MockNativeAgentSessionInfo.sessionId,
				update: {
					sessionUpdate: "current_mode_update",
					currentModeId: "human",
				},
			},
		);
	});
	await user.click(
		screen.getByRole("button", {
			name: /^Investigate the failing tests/,
			expanded: false,
		}),
	);
	expect(mock.server.sockets).toHaveLength(1);
	expect(mock.server.socket.readyState).toBe(1);
	expect(mock.load).not.toHaveBeenCalled();
	expect(mock.cancel).not.toHaveBeenCalled();
	expect(mock.mode).toHaveBeenCalledTimes(1);
	await user.click(screen.getByRole("button", { name: "Send" }));
	expect(mock.prompt).not.toHaveBeenCalled();
	await user.click(await screen.findByRole("button", { name: "Observe" }));
	await waitFor(() => expect(mock.mode).toHaveBeenCalledTimes(2));
	await user.click(screen.getByRole("button", { name: "Send" }));
	await waitFor(() => expect(mock.prompt).toHaveBeenCalledTimes(1));
	view.unmount();
});

it("separates thinking from messages, decodes fragmented Unicode, bounds retained display, and cancels remote permission requests", async () => {
	const mock = setup();
	const session = new NativeAgentSession(MockWorkspaceAgent.id);
	await session.connect();
	await session.load(MockNativeAgentSessionInfo);
	const send = mock.server.socket.connection.client;
	for (const [messageId, text] of [
		["thought-1", "Check "],
		["thought-1", "the parser 🚀"],
		["thought-2", "Check the fixtures."],
	]) {
		await send.notify(methods.client.session.update, {
			sessionId: MockNativeAgentSessionInfo.sessionId,
			update: {
				sessionUpdate: "agent_thought_chunk",
				messageId,
				content: { type: "text", text },
			},
		});
	}
	await send.notify(methods.client.session.update, {
		sessionId: MockNativeAgentSessionInfo.sessionId,
		update: {
			sessionUpdate: "agent_message_chunk",
			messageId: "thought-2",
			content: { type: "text", text: "Hello 🚀" },
		},
	});
	await waitFor(() =>
		expect(session.getSnapshot().session?.entries).toMatchObject([
			{ kind: "thought", messageID: "thought-1", text: "Check the parser 🚀" },
			{ kind: "thought", messageID: "thought-2", text: "Check the fixtures." },
			{ kind: "agent", messageID: "thought-2", text: "Hello 🚀" },
		]),
	);
	await send.notify(methods.client.session.update, {
		sessionId: MockNativeAgentSessionInfo.sessionId,
		update: {
			sessionUpdate: "tool_call",
			toolCallId: "t1",
			title: "Run tests",
			status: "pending",
		},
	});
	await send.notify(methods.client.session.update, {
		sessionId: MockNativeAgentSessionInfo.sessionId,
		update: {
			sessionUpdate: "tool_call_update",
			toolCallId: "t1",
			status: "completed",
		},
	});
	await waitFor(() =>
		expect(session.getSnapshot().session?.entries.at(-1)).toMatchObject({
			kind: "tool",
			status: "completed",
		}),
	);
	const permission = await send.request(
		methods.client.session.requestPermission,
		{
			sessionId: MockNativeAgentSessionInfo.sessionId,
			toolCall: { toolCallId: "permission" },
			options: [{ optionId: "yes", name: "Allow", kind: "allow_once" }],
		},
	);
	expect(permission).toEqual({ outcome: { outcome: "cancelled" } });
	await waitFor(() =>
		expect(session.getSnapshot().session?.needsInput).toBe(true),
	);
	for (let i = 0; i < 96; i++) {
		await send.notify(methods.client.session.update, {
			sessionId: MockNativeAgentSessionInfo.sessionId,
			update: {
				sessionUpdate: "agent_thought_chunk",
				messageId: String(i),
				content: { type: "text", text: "x".repeat(100) },
			},
		});
	}
	await waitFor(() =>
		expect(session.getSnapshot().session?.entries).toHaveLength(100),
	);
	expect(session.getSnapshot().session?.truncated).toBe(false);
	await session.prompt("Continue");
	expect(session.getSnapshot().session?.entries).toHaveLength(100);
	expect(session.getSnapshot().session?.truncated).toBe(true);
	for (const text of ["x".repeat(8_001), "y"]) {
		await send.notify(methods.client.session.update, {
			sessionId: MockNativeAgentSessionInfo.sessionId,
			update: {
				sessionUpdate: "agent_thought_chunk",
				content: { type: "text", text },
			},
		});
	}
	await waitFor(() =>
		expect(session.getSnapshot().session?.entries.at(-1)).toMatchObject({
			kind: "thought",
			text: `${"x".repeat(7_999)}y`,
		}),
	);
	expect(session.getSnapshot().session?.entries).toHaveLength(100);
	for (const namespace of ["another-adapter", "claude-acp-bridge"]) {
		await send.notify(methods.client.session.update, {
			sessionId: MockNativeAgentSessionInfo.sessionId,
			update: {
				sessionUpdate: "session_info_update",
				_meta: { [namespace]: { error: "Stream integrity failure" } },
			},
		});
		await session.list();
		expect(session.getSnapshot().session?.error).toBe(
			namespace === "claude-acp-bridge"
				? "Stream integrity failure"
				: undefined,
		);
	}
	session.close();
});

it.each([
	MockNativeAgentSessionInfo,
	MockCodexNativeAgentSessionInfo,
	MockOpenCodeNativeAgentSessionInfo,
])(
	"retains native identity, messages, and mode updates streamed before create and load responses: $sessionId",
	async (info) => {
		const mock = setup(MockNativeAgentHarnesses);
		const replay = async () => {
			await mock.server.socket.connection.client.notify(
				methods.client.session.update,
				{
					sessionId: info.sessionId,
					update: {
						sessionUpdate: "user_message_chunk",
						content: { type: "text", text: "Retained question" },
					},
				},
			);
			await mock.server.socket.connection.client.notify(
				methods.client.session.update,
				{
					sessionId: info.sessionId,
					update: {
						sessionUpdate: "agent_thought_chunk",
						content: { type: "text", text: "Retained thinking" },
					},
				},
			);
			await mock.server.socket.connection.client.notify(
				methods.client.session.update,
				{
					sessionId: info.sessionId,
					update: {
						sessionUpdate: "current_mode_update",
						currentModeId: "human",
					},
				},
			);
			return {
				sessionId: info.sessionId,
				modes: MockNativeAgentModes,
				_meta: info._meta,
			};
		};
		mock.create.mockImplementation(replay);
		mock.load.mockImplementation(replay);
		const session = new NativeAgentSession(MockWorkspaceAgent.id);
		await session.connect();
		await session.create(info.cwd, nativeAgentMetadata(info).harness);
		expect(session.getSnapshot().session?.info._meta).toEqual(info._meta);
		expect(session.getSnapshot().session?.entries).toMatchObject([
			{ kind: "user", text: "Retained question" },
			{ kind: "thought", text: "Retained thinking" },
		]);
		await session.connect();
		expect(session.getSnapshot().session?.info._meta).toEqual(info._meta);
		expect(session.getSnapshot().session?.entries).toMatchObject([
			{ kind: "user", text: "Retained question" },
			{ kind: "thought", text: "Retained thinking" },
		]);
		expect(session.getSnapshot().session?.modes?.currentModeId).toBe("human");
		const entries = session.getSnapshot().session?.entries;
		await session.setMode("automation");
		mock.load.mockResolvedValueOnce({ modes: MockNativeAgentModes });
		await session.load(MockOtherNativeAgentSessionInfo);
		await session.load(info);
		expect(session.getSnapshot().session?.entries).toBe(entries);
		expect(session.getSnapshot().session?.info._meta).toEqual(info._meta);
		expect(mock.create).toHaveBeenCalledTimes(1);
		expect(mock.load).toHaveBeenCalledTimes(2);
		expect(mock.cancel).not.toHaveBeenCalled();
		session.close();
	},
);

it.each([
	MockOpenCodeNativeAgentSessionInfo,
	MockArchivedOpenCodeNativeAgentSessionInfo,
])(
	"restores only an exact discovered selection without native mutations: $sessionId ($title)",
	async (info) => {
		const mock = setup(MockNativeAgentHarnesses);
		const session = new NativeAgentSession(MockWorkspaceAgent.id, {
			sessionId: info.sessionId,
			cwd: info.cwd,
		});
		await session.connect();
		mock.list.mockResolvedValueOnce({
			sessions: [
				MockOtherNativeAgentSessionInfo,
				{ ...info, cwd: "/different-directory" },
			],
			nextCursor: "next",
		});
		await session.list();
		expect(mock.load).not.toHaveBeenCalled();
		mock.list.mockResolvedValue({ sessions: [info] });
		await session.list("next");
		await waitFor(() =>
			expect(mock.load).toHaveBeenCalledExactlyOnceWith({
				sessionId: info.sessionId,
				cwd: info.cwd,
				mcpServers: [],
			}),
		);
		expect(session.getSnapshot().session?.info).toEqual(info);
		await session.list();
		expect(mock.load).toHaveBeenCalledTimes(1);
		expect(mock.create).not.toHaveBeenCalled();
		expect(mock.mode).not.toHaveBeenCalled();
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(mock.cancel).not.toHaveBeenCalled();
		session.close();
	},
);

it("keeps discovery and pagination usable while saved history is loading", async () => {
	const mock = setup(MockNativeAgentHarnesses);
	const restored = createDeferred<LoadSessionResponse>();
	mock.load.mockImplementation(async ({ sessionId }) =>
		sessionId === MockOpenCodeNativeAgentSessionInfo.sessionId
			? restored.promise
			: { modes: MockNativeAgentModes },
	);
	mock.list.mockImplementation(async ({ cursor }) =>
		cursor
			? { sessions: [MockOtherNativeAgentSessionInfo] }
			: {
					sessions: [
						MockOpenCodeNativeAgentSessionInfo,
						MockNativeAgentSessionInfo,
					],
					nextCursor: "next",
				},
	);
	const key = nativeAgentPanelStorageKey(
		MockUserOwner.id,
		MockWorkspace.id,
		mockWorkspaceAgent.name,
	);
	writeNativeAgentPanelState(key, {
		cwd: MockOpenCodeNativeAgentSessionInfo.cwd,
		harness: "opencode",
		prompt: "Send to the healthy session only",
		selectedSession: {
			sessionId: MockOpenCodeNativeAgentSessionInfo.sessionId,
			cwd: MockOpenCodeNativeAgentSessionInfo.cwd,
		},
	});
	const view = renderControls();
	const user = userEvent.setup();
	await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(1));
	await user.click(screen.getByRole("button", { name: "Send" }));
	expect(mock.prompt).not.toHaveBeenCalled();
	await user.click(
		await screen.findByRole("button", { name: "More sessions" }),
	);
	await waitFor(() =>
		expect(mock.list).toHaveBeenCalledWith({ cursor: "next" }),
	);
	await user.click(
		await screen.findByRole("button", {
			name: /^Investigate the failing tests/,
		}),
	);
	await waitFor(() =>
		expect(mock.load).toHaveBeenCalledWith({
			sessionId: MockNativeAgentSessionInfo.sessionId,
			cwd: MockNativeAgentSessionInfo.cwd,
			mcpServers: [],
		}),
	);
	await user.click(screen.getByRole("button", { name: "Send" }));
	await waitFor(() =>
		expect(mock.prompt).toHaveBeenCalledExactlyOnceWith({
			sessionId: MockNativeAgentSessionInfo.sessionId,
			prompt: [{ type: "text", text: "Send to the healthy session only" }],
		}),
	);
	await act(async () => restored.resolve({ modes: MockNativeAgentModes }));
	await user.type(
		screen.getByRole("textbox", { name: "Prompt" }),
		"Keep the healthy selection",
	);
	await user.click(screen.getByRole("button", { name: "Send" }));
	await waitFor(() =>
		expect(mock.prompt).toHaveBeenLastCalledWith({
			sessionId: MockNativeAgentSessionInfo.sessionId,
			prompt: [{ type: "text", text: "Keep the healthy selection" }],
		}),
	);
	expect(readNativeAgentPanelState(key)?.selectedSession?.sessionId).toBe(
		MockNativeAgentSessionInfo.sessionId,
	);
	expect(mock.create).not.toHaveBeenCalled();
	expect(mock.mode).not.toHaveBeenCalled();
	expect(mock.cancel).not.toHaveBeenCalled();
	view.unmount();
});

it.each(["before discovery", "during restored load"])(
	"keeps explicit selection instead of restoring stale selection: %s",
	async (timing) => {
		const mock = setup(MockNativeAgentHarnesses);
		const restored = createDeferred<LoadSessionResponse>();
		mock.list.mockResolvedValue({
			sessions: [MockOpenCodeNativeAgentSessionInfo],
		});
		mock.load.mockImplementation(async ({ sessionId }) =>
			sessionId === MockOpenCodeNativeAgentSessionInfo.sessionId
				? restored.promise
				: { modes: MockNativeAgentModes },
		);
		const session = new NativeAgentSession(
			MockWorkspaceAgent.id,
			MockOpenCodeNativeAgentSessionInfo,
		);
		await session.connect();
		const mockLoad = vi.spyOn(session, "load");
		let restoration: Promise<SessionInfo> | undefined;
		if (timing === "during restored load") {
			await session.list();
			restoration = mockLoad.mock.results[0]?.value;
			expect(restoration).toBeDefined();
			expect(session.getSnapshot().session?.loading).toBe(true);
		}
		await session.load(MockOtherNativeAgentSessionInfo);
		restored.resolve({ modes: MockNativeAgentModes });
		await restoration;
		await session.list();
		expect(session.getSnapshot().session?.info).toEqual(
			MockOtherNativeAgentSessionInfo,
		);
		expect(session.getSnapshot().session?.loading).toBe(false);
		expect(mock.load).toHaveBeenCalledTimes(
			timing === "before discovery" ? 1 : 2,
		);
		mockLoad.mockRestore();
		expect(mock.mode).not.toHaveBeenCalled();
		expect(mock.create).not.toHaveBeenCalled();
		session.close();
	},
);

it("keeps discovery usable after a restored selection fails to load", async () => {
	const mock = setup(MockNativeAgentHarnesses);
	mock.list.mockResolvedValue({
		sessions: [MockOpenCodeNativeAgentSessionInfo],
	});
	mock.load.mockRejectedValue(
		new RequestError(-32000, "History is unavailable"),
	);
	const session = new NativeAgentSession(
		MockWorkspaceAgent.id,
		MockOpenCodeNativeAgentSessionInfo,
	);
	await session.connect();
	await expect(session.list()).resolves.toEqual({
		sessions: [MockOpenCodeNativeAgentSessionInfo],
	});
	await waitFor(() =>
		expect(session.getSnapshot().session).toMatchObject({
			error: "History is unavailable",
			loading: false,
		}),
	);
	await session.list();
	expect(mock.load).toHaveBeenCalledTimes(1);
	expect(mock.create).not.toHaveBeenCalled();
	expect(mock.mode).not.toHaveBeenCalled();
	expect(mock.prompt).not.toHaveBeenCalled();
	session.close();
});

it("disconnects an in-flight prompt without cancelling native work or resending it", async () => {
	const mock = setup();
	const completion = createDeferred<PromptResponse>();
	mock.prompt.mockReturnValue(completion.promise);
	const session = new NativeAgentSession(MockWorkspaceAgent.id);
	await session.connect();
	await session.load(MockNativeAgentSessionInfo);
	const prompt = session.prompt("Keep running in the workspace");
	const rejected = expect(prompt).rejects.toThrow();
	await waitFor(() => expect(mock.prompt).toHaveBeenCalledTimes(1));
	session.close();
	await rejected;
	await session.connect();
	expect(mock.cancel).not.toHaveBeenCalled();
	expect(mock.prompt).toHaveBeenCalledTimes(1);
	expect(mock.load).toHaveBeenCalledTimes(2);
	completion.resolve({ stopReason: "end_turn" });
	session.close();
});

it("preserves retained history and keeps listing available when reconnect cannot reload the selected session", async () => {
	const mock = setup();
	mock.load.mockImplementationOnce(async () => {
		await mock.server.socket.connection.client.notify(
			methods.client.session.update,
			{
				sessionId: MockNativeAgentSessionInfo.sessionId,
				update: {
					sessionUpdate: "agent_message_chunk",
					content: {
						type: "text",
						text: "Retained answer from the selected session",
					},
				},
			},
		);
		return { modes: MockNativeAgentModes };
	});
	const session = new NativeAgentSession(MockWorkspaceAgent.id);
	await session.connect();
	await session.load(MockNativeAgentSessionInfo);
	const retainedEntries = session.getSnapshot().session?.entries;
	expect(retainedEntries).toMatchObject([
		{ kind: "agent", text: "Retained answer from the selected session" },
	]);
	mock.load.mockRejectedValueOnce(
		new RequestError(-32000, "Native session ended"),
	);
	await expect(session.connect()).rejects.toThrow("Native session ended");
	expect(session.getSnapshot()).toMatchObject({
		status: "connected",
		session: { info: MockNativeAgentSessionInfo, entries: retainedEntries },
	});
	expect(session.getSnapshot().session?.error).toBeTruthy();
	await expect(session.list()).resolves.toMatchObject({
		sessions: [MockNativeAgentSessionInfo],
	});
	session.close();
});

const mockRecoveredCodexInfo: SessionInfo = {
	...MockCodexNativeAgentSessionInfo,
	_meta: {
		"coder-acp": {
			...MockCodexNativeAgentSessionInfo._meta["coder-acp"],
			lastError: {
				message: "Shell command exited with code 1: missing input fixture.",
				at: 1_792_757_100_000,
			},
		},
	},
};
const mockFaultedCodexInfo: SessionInfo = {
	...MockCodexNativeAgentSessionInfo,
	_meta: {
		"coder-acp": {
			...MockCodexNativeAgentSessionInfo._meta["coder-acp"],
			ended: true,
			fault: true,
		},
	},
};
const mockResumableCodexInfo: SessionInfo = {
	...MockCodexNativeAgentSessionInfo,
	_meta: {
		"coder-acp": {
			...MockCodexNativeAgentSessionInfo._meta["coder-acp"],
			archived: true,
			resumable: true,
			resumePending: false,
			ended: true,
		},
	},
};
const mockResumePendingCodexInfo: SessionInfo = {
	...MockCodexNativeAgentSessionInfo,
	_meta: {
		"coder-acp": {
			...MockCodexNativeAgentSessionInfo._meta["coder-acp"],
			archived: false,
			resumable: false,
			resumePending: true,
			registrationPending: true,
			mode: "human",
		},
	},
};
const mockRegisteredCodexInfo: SessionInfo = {
	...MockCodexNativeAgentSessionInfo,
	_meta: {
		"coder-acp": {
			...MockCodexNativeAgentSessionInfo._meta["coder-acp"],
			mode: "human",
		},
	},
};

it.each([
	{
		namespace: "claude-acp-bridge",
		harness: "claude",
		mockCreated: MockNativeAgentSessionInfo,
		mockRecovered: MockRecoveredNativeAgentSessionInfo,
		mockFaulted: MockFaultedNativeAgentSessionInfo,
		mockStatuses: [
			MockResumableNativeAgentSessionInfo,
			MockResumePendingNativeAgentSessionInfo,
			MockRegisteredNativeAgentSessionInfo,
		],
	},
	{
		namespace: "coder-acp",
		harness: "codex",
		mockCreated: MockCodexNativeAgentSessionInfo,
		mockRecovered: mockRecoveredCodexInfo,
		mockFaulted: mockFaultedCodexInfo,
		mockStatuses: [
			mockResumableCodexInfo,
			mockResumePendingCodexInfo,
			mockRegisteredCodexInfo,
		],
	},
])(
	"retains $namespace metadata from create, load, and discovery without losing transcript state",
	async ({
		harness,
		mockCreated,
		mockRecovered,
		mockFaulted,
		mockStatuses,
	}) => {
		const mock = setup();
		mock.create.mockResolvedValue({
			sessionId: mockCreated.sessionId,
			modes: MockNativeAgentModes,
			_meta: mockCreated._meta,
		});
		const session = new NativeAgentSession(MockWorkspaceAgent.id);
		await session.connect();
		await session.create(mockCreated.cwd);
		expect(session.getSnapshot().session?.info._meta).toEqual(
			mockCreated._meta,
		);
		expect(
			nativeAgentMetadata(session.getSnapshot().session?.info).harness,
		).toBe(harness);
		mock.load.mockResolvedValue({
			modes: MockNativeAgentModes,
			_meta: mockRecovered._meta,
		});
		await session.connect();
		expect(session.getSnapshot().session?.info._meta).toEqual(
			mockRecovered._meta,
		);
		await session.prompt("Retain this transcript");
		const entries = session.getSnapshot().session?.entries;
		mock.list.mockResolvedValue({
			sessions: [{ ...mockCreated, _meta: mockFaulted._meta }],
		});
		await session.list();
		expect(session.getSnapshot().session?.info._meta).toEqual(
			mockFaulted._meta,
		);
		expect(session.getSnapshot().session?.entries).toBe(entries);
		await expect(session.prompt("Do not send into a fault")).rejects.toThrow();
		for (const mockStatus of mockStatuses) {
			mock.list.mockResolvedValue({
				sessions: [{ ...mockCreated, _meta: mockStatus._meta }],
			});
			await session.list();
			expect(session.getSnapshot().session?.info._meta).toEqual(
				mockStatus._meta,
			);
			expect(session.getSnapshot().session?.entries).toBe(entries);
		}
		expect(mock.prompt).toHaveBeenCalledTimes(1);
		session.close();
	},
);

it.each([
	"load fails",
	"load succeeds with a fault",
	"load succeeds after ending",
	"saved without resume capability",
	"setup faults before registration",
])(
	"keeps unavailable sessions selectable and opens diagnostics without ACP mutations when %s",
	async (state) => {
		const mock = setup();
		const open = vi.fn();
		vi.stubGlobal("open", open);
		const mockUnavailableInfo = { ...MockFaultedNativeAgentSessionInfo };
		if (state === "load succeeds after ending") {
			mockUnavailableInfo._meta = { "claude-acp-bridge": { ended: true } };
		} else if (state === "saved without resume capability") {
			mockUnavailableInfo._meta = { "claude-acp-bridge": { archived: true } };
		} else if (state === "setup faults before registration") {
			mockUnavailableInfo._meta = {
				"claude-acp-bridge": { registrationPending: true, fault: true },
			};
			mock.load.mockResolvedValueOnce({ modes: MockPendingNativeAgentModes });
		}
		mock.list.mockResolvedValue({
			sessions: [mockUnavailableInfo, MockRecoveredNativeAgentSessionInfo],
		});
		if (state === "load fails") {
			mock.load.mockRejectedValueOnce(
				new RequestError(-32000, "Session integrity fault"),
			);
		}
		const view = renderControls();
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", {
				name: /^Inspect an interrupted session/,
			}),
		);
		await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(1));
		await user.type(
			screen.getByRole("textbox", { name: "Prompt" }),
			"Continue after inspection",
		);
		await user.click(screen.getByRole("button", { name: "Send" }));
		await user.click(screen.getByRole("button", { name: "Open" }));
		expect(open).toHaveBeenCalledExactlyOnceWith(
			expect.stringContaining("/terminal?"),
			"_blank",
			"noopener,noreferrer",
		);
		expect(mock.load).toHaveBeenCalledTimes(1);
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(mock.mode).not.toHaveBeenCalled();
		await user.click(
			screen.getByRole("button", { name: /^Continue after a tool error/ }),
		);
		await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(2));
		await user.click(screen.getByRole("button", { name: "Send" }));
		await waitFor(() =>
			expect(mock.prompt).toHaveBeenCalledWith({
				sessionId: MockRecoveredNativeAgentSessionInfo.sessionId,
				prompt: [{ type: "text", text: "Continue after inspection" }],
			}),
		);
		expect(mock.create).not.toHaveBeenCalled();
		view.unmount();
	},
);

it("opens stale sessions but blocks prompts until refresh confirms recovery without discarding the draft", async () => {
	const mock = setup();
	const open = vi.fn();
	vi.stubGlobal("open", open);
	mock.list.mockResolvedValue({ sessions: [MockStaleNativeAgentSessionInfo] });
	const view = renderControls();
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("button", { name: /^Check runtime availability/ }),
	);
	await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(1));
	await user.type(
		screen.getByRole("textbox", { name: "Prompt" }),
		"Keep this draft",
	);
	await user.click(screen.getByRole("button", { name: "Send" }));
	await user.click(screen.getByRole("button", { name: "Open" }));
	expect(open).toHaveBeenCalledExactlyOnceWith(
		expect.stringContaining("/terminal?"),
		"_blank",
		"noopener,noreferrer",
	);
	expect(mock.load).toHaveBeenCalledTimes(1);
	expect(mock.prompt).not.toHaveBeenCalled();
	expect(mock.mode).not.toHaveBeenCalled();
	mock.list.mockResolvedValue({
		sessions: [
			{
				...MockStaleNativeAgentSessionInfo,
				_meta: MockRecoveredNativeAgentSessionInfo._meta,
			},
		],
	});
	await user.click(screen.getByRole("button", { name: "Refresh sessions" }));
	await waitFor(() => expect(mock.list).toHaveBeenCalledTimes(2));
	await user.click(screen.getByRole("button", { name: "Send" }));
	await waitFor(() =>
		expect(mock.prompt).toHaveBeenCalledWith({
			sessionId: MockStaleNativeAgentSessionInfo.sessionId,
			prompt: [{ type: "text", text: "Keep this draft" }],
		}),
	);
	expect(mock.server.sockets).toHaveLength(1);
	expect(mock.load).toHaveBeenCalledTimes(1);
	view.unmount();
});

it("retains failed load identity and actionable error without claiming a loaded session", async () => {
	const mock = setup();
	mock.load.mockRejectedValueOnce(
		new RequestError(-32000, "Session integrity fault"),
	);
	const session = new NativeAgentSession(MockWorkspaceAgent.id);
	await session.connect();
	await expect(session.load(MockFaultedNativeAgentSessionInfo)).rejects.toThrow(
		"Session integrity fault",
	);
	await waitFor(() =>
		expect(session.getSnapshot().session).toMatchObject({
			info: MockFaultedNativeAgentSessionInfo,
			error: "Session integrity fault",
		}),
	);
	expect(session.getSnapshot().session?.modes).toBeUndefined();
	expect(mock.load).toHaveBeenCalledTimes(1);
	session.close();
});

it("rejects adapters without session loading support", async () => {
	const mock = setup();
	mock.initialize.mockResolvedValue({
		protocolVersion: PROTOCOL_VERSION,
		agentCapabilities: {},
	});
	const session = new NativeAgentSession(MockWorkspaceAgent.id);
	await expect(session.connect()).rejects.toThrow(
		"must support session listing and loading",
	);
	expect(session.getSnapshot().status).toBe("disconnected");
	expect(mock.server.socket.readyState).toBe(3);
	expect(mock.create).not.toHaveBeenCalled();
});

it("reports the setup failure close code and never reconnects automatically", async () => {
	const mock = setup();
	const session = new NativeAgentSession(MockWorkspaceAgent.id);
	await session.connect();
	mock.server.socket.close(1011);
	await waitFor(() =>
		expect(session.getSnapshot()).toMatchObject({
			status: "disconnected",
			error:
				"Workspace ACP adapter exited. Check coder-acp setup in the workspace.",
		}),
	);
	expect(mock.server.sockets).toHaveLength(1);
	session.close();
});
