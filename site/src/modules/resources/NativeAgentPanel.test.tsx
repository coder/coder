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
	type SetSessionModeRequest,
} from "@agentclientprotocol/sdk";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StrictMode } from "react";
import { QueryClientProvider } from "react-query";
import { meKey } from "#/api/queries/users";
import type { WorkspaceAgent } from "#/api/typesGenerated";
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
	MockBlockedNativeAgentSessionInfo,
	MockFaultedNativeAgentSessionInfo,
	MockNativeAgentInfo,
	MockNativeAgentModes,
	MockNativeAgentSessionInfo,
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
import { NativeAgentSession } from "./NativeAgentSession";

function setup() {
	const initialize = vi.fn(
		async (_params: InitializeRequest): Promise<InitializeResponse> => ({
			protocolVersion: PROTOCOL_VERSION,
			agentInfo: MockNativeAgentInfo,
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

it("discovers automatically and creates only on request in an edited absolute directory", async () => {
	const mock = setup();
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
	await user.click(screen.getByRole("button", { name: "Create session" }));
	await waitFor(() =>
		expect(mock.mode).toHaveBeenCalledWith({
			sessionId: MockNativeAgentSessionInfo.sessionId,
			modeId: "automation",
		}),
	);
	expect(mock.create).toHaveBeenCalledWith({
		cwd: "/home/coder/other",
		mcpServers: [],
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
});

it("sends a text prompt and cancel notification without replaying it", async () => {
	const mock = setup();
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
	await user.click(
		screen.getByRole("button", {
			name: /Investigate the failing tests/,
			expanded: true,
		}),
	);
	await user.click(
		screen.getByRole("button", {
			name: /Investigate the failing tests/,
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
			name: /Investigate the failing tests/,
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

it("loads paginated sessions and switches without creating or reloading replacements", async () => {
	const mock = setup();
	mock.list.mockImplementation(async ({ cursor }) =>
		cursor
			? { sessions: [MockOtherNativeAgentSessionInfo] }
			: { sessions: [MockNativeAgentSessionInfo], nextCursor: "next" },
	);
	const view = renderControls();
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("button", {
			name: /Investigate the failing tests.*00c083d0-b72a-4a12-853b-270bf30de904/,
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
			name: /Investigate the failing tests/,
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
	await user.click(
		await screen.findByRole("button", { name: /Inspect the next failure/ }),
	);
	await waitFor(() =>
		expect(mock.load).toHaveBeenLastCalledWith({
			sessionId: MockOtherNativeAgentSessionInfo.sessionId,
			cwd: MockOtherNativeAgentSessionInfo.cwd,
			mcpServers: [],
		}),
	);
	await user.click(
		screen.getByRole("button", { name: /Investigate the failing tests/ }),
	);
	await user.click(screen.getByRole("button", { name: "Send" }));
	await waitFor(() =>
		expect(mock.prompt).toHaveBeenCalledWith({
			sessionId: MockNativeAgentSessionInfo.sessionId,
			prompt: [{ type: "text", text: "Continue the first session" }],
		}),
	);
	expect(mock.load).toHaveBeenCalledTimes(2);
	expect(mock.mode).not.toHaveBeenCalled();
	expect(mock.create).not.toHaveBeenCalled();
	view.unmount();
});

it("preserves the open panel, draft, selected session, and directory across workspace agent outages", async () => {
	const mock = setup();
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
			name: /Investigate the failing tests/,
		}),
	);
	await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(1));
	await user.click(screen.getByRole("button", { name: "Add session" }));
	await user.clear(screen.getByRole("textbox", { name: "Working directory" }));
	await user.type(
		screen.getByRole("textbox", { name: "Working directory" }),
		"/home/coder/unsaved-directory",
	);
	await user.type(
		screen.getByRole("textbox", { name: "Prompt" }),
		"Unsent draft survives an outage",
	);

	view.rerender(panel(mockOfflineAgent));
	expect(mock.server.socket.readyState).toBe(3);
	await user.click(screen.getByRole("button", { name: "Reconnect" }));
	await user.click(screen.getByRole("button", { name: "Send" }));
	await user.click(screen.getByRole("button", { name: "Create session" }));
	expect(mock.server.sockets).toHaveLength(1);
	expect(mock.prompt).not.toHaveBeenCalled();
	expect(mock.create).not.toHaveBeenCalled();

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
	await user.click(screen.getByRole("button", { name: "Create session" }));
	await waitFor(() =>
		expect(mock.create).toHaveBeenCalledWith({
			cwd: "/home/coder/unsaved-directory",
			mcpServers: [],
		}),
	);
	view.unmount();
});

it("waits for human mode acknowledgement before navigating to a safely quoted join command", async () => {
	const mock = setup();
	const unsafeID = "session'; touch /tmp/unsafe; echo '";
	mock.create.mockResolvedValue({
		sessionId: unsafeID,
		modes: MockNativeAgentModes,
	});
	const popup = {
		location: { href: "about:blank" },
		opener: window,
		close: vi.fn(),
	};
	vi.stubGlobal(
		"open",
		vi.fn(() => popup),
	);
	const view = renderControls();
	const user = await connectAndCreate(mock);
	const ack = createDeferred<Record<string, never>>();
	mock.mode.mockReturnValueOnce(ack.promise);
	await user.click(screen.getByRole("button", { name: "Join" }));
	await waitFor(() =>
		expect(mock.mode).toHaveBeenLastCalledWith({
			sessionId: unsafeID,
			modeId: "human",
		}),
	);
	expect(popup.location.href).toBe("about:blank");
	ack.resolve({});
	await waitFor(() => expect(popup.location.href).not.toBe("about:blank"));
	const url = new URL(popup.location.href, location.origin);
	expect(url.pathname).toBe(
		`/@${MockWorkspace.owner_name}/${MockWorkspace.name}.${MockWorkspaceAgent.name}/terminal`,
	);
	expect(url.searchParams.get("command")).toBe(
		"coder-acp attach 'session'\\''; touch /tmp/unsafe; echo '\\'''; exec \"${SHELL:-/bin/sh}\" -l",
	);
	expect(mock.server.socket.readyState).toBe(1);
	expect(mock.cancel).not.toHaveBeenCalled();
	await user.click(await screen.findByRole("button", { name: "Observe" }));
	await waitFor(() =>
		expect(mock.mode).toHaveBeenLastCalledWith({
			sessionId: unsafeID,
			modeId: "automation",
		}),
	);
	expect(window.open).toHaveBeenCalledTimes(1);
	expect(popup.close).not.toHaveBeenCalled();
	expect(mock.prompt).not.toHaveBeenCalled();
	view.unmount();
});

it("does not navigate when switching to human mode fails", async () => {
	const mock = setup();
	const popup = {
		location: { href: "about:blank" },
		opener: window,
		close: vi.fn(),
	};
	vi.stubGlobal(
		"open",
		vi.fn(() => popup),
	);
	const view = renderControls();
	const user = await connectAndCreate(mock);
	mock.mode.mockRejectedValueOnce(
		new Error("Native runtime did not acknowledge human mode."),
	);
	await user.click(screen.getByRole("button", { name: "Join" }));
	await waitFor(() => expect(popup.close).toHaveBeenCalled());
	expect(popup.location.href).toBe("about:blank");
	view.unmount();
});

it.each(
	[
		MockBlockedNativeAgentSessionInfo,
		MockResumableNativeAgentSessionInfo,
	].flatMap((info) =>
		["unselected", "selected", "collapsed"].map((selection) => ({
			info,
			selection,
		})),
	),
)(
	"joins a $selection session ($info.title) only after load and human-mode acknowledgement",
	async ({ info, selection }) => {
		const mock = setup();
		mock.list.mockResolvedValue({ sessions: [info] });
		mock.load.mockResolvedValue({ modes: MockPendingNativeAgentModes });
		const popup = {
			location: { href: "about:blank" },
			opener: window,
			close: vi.fn(),
		};
		vi.stubGlobal(
			"open",
			vi.fn(() => popup),
		);
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
		const loaded = createDeferred<LoadSessionResponse>();
		if (selection === "unselected")
			mock.load.mockReturnValueOnce(loaded.promise);
		const acknowledged = createDeferred<Record<string, never>>();
		mock.mode.mockReturnValueOnce(acknowledged.promise);
		const joinButton = await screen.findByRole("button", {
			name: selection === "selected" ? "Join" : `Join ${info.title}`,
		});
		if (selection === "unselected") {
			joinButton.focus();
			await user.keyboard("{Enter}");
		} else {
			await user.click(joinButton);
		}
		expect(window.open).toHaveBeenCalledWith("about:blank", "_blank");
		expect(popup.opener).toBeNull();
		await waitFor(() =>
			expect(mock.load).toHaveBeenCalledWith({
				sessionId: info.sessionId,
				cwd: info.cwd,
				mcpServers: [],
			}),
		);
		if (selection === "unselected") {
			expect(mock.mode).not.toHaveBeenCalled();
			expect(popup.location.href).toBe("about:blank");
			await act(async () =>
				loaded.resolve({ modes: MockPendingNativeAgentModes }),
			);
		}
		await waitFor(() =>
			expect(mock.mode).toHaveBeenCalledWith({
				sessionId: info.sessionId,
				modeId: "human",
			}),
		);
		expect(popup.location.href).toBe("about:blank");
		await act(async () => acknowledged.resolve({}));
		await waitFor(() => expect(popup.location.href).not.toBe("about:blank"));
		expect(
			new URL(popup.location.href, location.origin).searchParams.get("command"),
		).toBe(
			`coder-acp attach '${info.sessionId}'; exec "\${SHELL:-/bin/sh}" -l`,
		);
		expect(mock.load).toHaveBeenCalledTimes(1);
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(mock.create).not.toHaveBeenCalled();
		expect(mock.cancel).not.toHaveBeenCalled();
		view.unmount();
	},
);

it.each(["popup blocked", "load fails", "mode fails"])(
	"does not navigate from a setup card when %s",
	async (failure) => {
		const mock = setup();
		mock.list.mockResolvedValue({
			sessions: [MockBlockedNativeAgentSessionInfo],
		});
		mock.load.mockResolvedValue({ modes: MockPendingNativeAgentModes });
		if (failure === "load fails")
			mock.load.mockRejectedValueOnce(
				new RequestError(-32000, "Native session could not be loaded."),
			);
		if (failure === "mode fails")
			mock.mode.mockRejectedValueOnce(
				new RequestError(-32000, "Human mode was not acknowledged."),
			);
		const popup = {
			location: { href: "about:blank" },
			opener: window,
			close: vi.fn(),
		};
		vi.stubGlobal(
			"open",
			vi.fn(() => (failure === "popup blocked" ? null : popup)),
		);
		const view = renderControls();
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", {
				name: "Join Set up the project agent",
			}),
		);
		if (failure === "popup blocked") {
			expect(mock.load).not.toHaveBeenCalled();
		} else {
			await waitFor(() => expect(popup.close).toHaveBeenCalledTimes(1));
		}
		if (failure !== "mode fails") expect(mock.mode).not.toHaveBeenCalled();
		expect(popup.location.href).toBe("about:blank");
		expect(mock.prompt).not.toHaveBeenCalled();
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
	"resumes saved history only on Join, preserves drafts and collapse through %s, and waits for Observe before sending",
	async (source) => {
		const mock = setup();
		mock.list.mockResolvedValue({
			sessions: [MockResumableNativeAgentSessionInfo],
		});
		mock.load.mockResolvedValue({ modes: MockPendingNativeAgentModes });
		const popup = {
			location: { href: "about:blank" },
			opener: window,
			close: vi.fn(),
		};
		vi.stubGlobal(
			"open",
			vi.fn(() => popup),
		);
		const view = renderControls();
		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", {
				name: /Investigate the failing tests.*00c083d0/,
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

		mock.mode.mockImplementationOnce(async () => {
			mock.list.mockResolvedValue({
				sessions: [MockResumePendingNativeAgentSessionInfo],
			});
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
			return {};
		});
		await user.click(screen.getByRole("button", { name: "Join" }));
		await waitFor(() => expect(popup.location.href).not.toBe("about:blank"));
		expect(mock.mode).toHaveBeenCalledWith({
			sessionId: MockResumableNativeAgentSessionInfo.sessionId,
			modeId: "human",
		});
		await user.click(screen.getByRole("button", { name: "Send" }));
		expect(mock.prompt).not.toHaveBeenCalled();
		await user.click(
			screen.getByRole("button", {
				name: /Investigate the failing tests.*00c083d0/,
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
				name: /Investigate the failing tests.*00c083d0/,
				expanded: false,
			}),
		);
		await user.click(screen.getByRole("button", { name: "Send" }));
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(mock.mode).toHaveBeenCalledTimes(1);
		await user.click(await screen.findByRole("button", { name: "Observe" }));
		await waitFor(() => expect(mock.mode).toHaveBeenCalledTimes(2));
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
	"only explicit resumable true permits human control of unavailable history: %s",
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
		if (resumable === true) {
			await session.setMode("human");
			expect(mock.mode).toHaveBeenCalledExactlyOnceWith({
				sessionId: MockResumableNativeAgentSessionInfo.sessionId,
				modeId: "human",
			});
		} else {
			await expect(session.setMode("human")).rejects.toThrow("unavailable");
			expect(mock.mode).not.toHaveBeenCalled();
		}
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(session.getSnapshot().session?.entries).toEqual([]);
		session.close();
	},
);

it("promotes setup metadata without replacing identity, history, or human control", async () => {
	const mock = setup();
	mock.load.mockResolvedValue({ modes: MockPendingNativeAgentModes });
	const session = new NativeAgentSession(MockWorkspaceAgent.id);
	await session.connect();
	await session.load(MockBlockedNativeAgentSessionInfo);
	const entries = session.getSnapshot().session?.entries;
	await mock.server.socket.connection.client.notify(
		methods.client.session.update,
		{
			sessionId: MockBlockedNativeAgentSessionInfo.sessionId,
			update: {
				sessionUpdate: "session_info_update",
				_meta: { "claude-acp-bridge": { pendingInput: true } },
			},
		},
	);
	await waitFor(() =>
		expect(session.getSnapshot().session?.needsInput).toBe(true),
	);
	await mock.server.socket.connection.client.notify(
		methods.client.session.update,
		{
			sessionId: MockBlockedNativeAgentSessionInfo.sessionId,
			update: {
				sessionUpdate: "session_info_update",
				_meta: { "claude-acp-bridge": { registrationPending: false } },
			},
		},
	);
	await waitFor(() =>
		expect(session.getSnapshot().session).toMatchObject({
			info: {
				sessionId: MockBlockedNativeAgentSessionInfo.sessionId,
				cwd: MockBlockedNativeAgentSessionInfo.cwd,
				_meta: {
					"claude-acp-bridge": {
						registrationPending: false,
						startedAt: 1_792_756_800_000,
					},
				},
			},
			needsInput: false,
			modes: { currentModeId: "human" },
		}),
	);
	expect(session.getSnapshot().session?.entries).toBe(entries);
	expect(mock.prompt).not.toHaveBeenCalled();
	expect(mock.mode).not.toHaveBeenCalled();
	expect(mock.load).toHaveBeenCalledTimes(1);
	session.close();
});

it("honors native mode updates and continues after switching back to automation", async () => {
	const mock = setup();
	const view = renderControls();
	const user = await connectAndCreate(mock);
	await user.type(screen.getByRole("textbox", { name: "Prompt" }), "Continue");
	await user.click(
		screen.getByRole("button", {
			name: /Investigate the failing tests/,
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
			name: /Investigate the failing tests/,
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

it("retains messages and mode updates streamed before create and load responses", async () => {
	const mock = setup();
	const replay = async () => {
		await mock.server.socket.connection.client.notify(
			methods.client.session.update,
			{
				sessionId: MockNativeAgentSessionInfo.sessionId,
				update: {
					sessionUpdate: "user_message_chunk",
					content: { type: "text", text: "Retained question" },
				},
			},
		);
		await mock.server.socket.connection.client.notify(
			methods.client.session.update,
			{
				sessionId: MockNativeAgentSessionInfo.sessionId,
				update: {
					sessionUpdate: "agent_thought_chunk",
					content: { type: "text", text: "Retained thinking" },
				},
			},
		);
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
		return {
			sessionId: MockNativeAgentSessionInfo.sessionId,
			modes: MockNativeAgentModes,
		};
	};
	mock.create.mockImplementation(replay);
	mock.load.mockImplementation(replay);
	const session = new NativeAgentSession(MockWorkspaceAgent.id);
	await session.connect();
	await session.create(MockNativeAgentSessionInfo.cwd);
	expect(session.getSnapshot().session?.entries).toMatchObject([
		{ kind: "user", text: "Retained question" },
		{ kind: "thought", text: "Retained thinking" },
	]);
	await session.connect();
	expect(session.getSnapshot().session?.entries).toMatchObject([
		{ kind: "user", text: "Retained question" },
		{ kind: "thought", text: "Retained thinking" },
	]);
	expect(session.getSnapshot().session?.modes?.currentModeId).toBe("human");
	expect(mock.create).toHaveBeenCalledTimes(1);
	expect(mock.load).toHaveBeenCalledTimes(1);
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

it("retains metadata from create, load, and discovery without losing transcript state", async () => {
	const mock = setup();
	mock.create.mockResolvedValue({
		sessionId: MockNativeAgentSessionInfo.sessionId,
		modes: MockNativeAgentModes,
		_meta: MockNativeAgentSessionInfo._meta,
	});
	const session = new NativeAgentSession(MockWorkspaceAgent.id);
	await session.connect();
	await session.create(MockNativeAgentSessionInfo.cwd);
	expect(session.getSnapshot().session?.info._meta).toEqual(
		MockNativeAgentSessionInfo._meta,
	);
	mock.load.mockResolvedValue({
		modes: MockNativeAgentModes,
		_meta: MockRecoveredNativeAgentSessionInfo._meta,
	});
	await session.connect();
	expect(session.getSnapshot().session?.info._meta).toEqual(
		MockRecoveredNativeAgentSessionInfo._meta,
	);
	await session.prompt("Retain this transcript");
	const entries = session.getSnapshot().session?.entries;
	mock.list.mockResolvedValue({
		sessions: [
			{
				...MockNativeAgentSessionInfo,
				_meta: MockFaultedNativeAgentSessionInfo._meta,
			},
		],
	});
	await session.list();
	expect(session.getSnapshot().session?.info._meta).toEqual(
		MockFaultedNativeAgentSessionInfo._meta,
	);
	expect(session.getSnapshot().session?.entries).toBe(entries);
	await expect(session.prompt("Do not send into a fault")).rejects.toThrow();
	for (const info of [
		MockResumableNativeAgentSessionInfo,
		MockResumePendingNativeAgentSessionInfo,
		MockRegisteredNativeAgentSessionInfo,
	]) {
		mock.list.mockResolvedValue({ sessions: [info] });
		await session.list();
		expect(session.getSnapshot().session?.info._meta).toEqual(info._meta);
		expect(session.getSnapshot().session?.entries).toBe(entries);
	}
	expect(mock.prompt).toHaveBeenCalledTimes(1);
	session.close();
});

it.each([
	"load fails",
	"load succeeds with a fault",
	"load succeeds after ending",
	"saved without resume capability",
	"setup faults before registration",
])(
	"keeps unavailable sessions selectable but blocks commands when %s",
	async (state) => {
		const mock = setup();
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
				name: /Inspect an interrupted session/,
			}),
		);
		await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(1));
		await user.type(
			screen.getByRole("textbox", { name: "Prompt" }),
			"Continue after inspection",
		);
		await user.click(screen.getByRole("button", { name: "Send" }));
		await user.click(screen.getByRole("button", { name: "Join" }));
		expect(mock.prompt).not.toHaveBeenCalled();
		expect(mock.mode).not.toHaveBeenCalled();
		await user.click(
			screen.getByRole("button", { name: /Continue after a tool error/ }),
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

it("blocks commands for stale availability until refresh confirms recovery without discarding the draft", async () => {
	const mock = setup();
	mock.list.mockResolvedValue({ sessions: [MockStaleNativeAgentSessionInfo] });
	const view = renderControls();
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("button", { name: /Check runtime availability/ }),
	);
	await waitFor(() => expect(mock.load).toHaveBeenCalledTimes(1));
	await user.type(
		screen.getByRole("textbox", { name: "Prompt" }),
		"Keep this draft",
	);
	await user.click(screen.getByRole("button", { name: "Send" }));
	await user.click(screen.getByRole("button", { name: "Join" }));
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
