import { act, fireEvent, render, screen } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import { workspaceByIdKey } from "#/api/queries/workspaces";
import type { Workspace, WorkspaceAgentLog } from "#/api/typesGenerated";
import {
	MockStartingWorkspace,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceAgentLogs,
	MockWorkspaceAgentStarting,
	MockWorkspaceBuildLogs,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	createMockWebSocket,
	type MockWebSocketServer,
} from "#/testHelpers/websockets";
import { OneWayWebSocket } from "#/utils/OneWayWebSocket";
import { ChatWorkspaceContext } from "../../../context/ChatWorkspaceContext";
import { WorkspaceLogBox } from "./WorkspaceLogBox";

afterEach(() => {
	vi.restoreAllMocks();
});

type BoxProps = React.ComponentProps<typeof WorkspaceLogBox>;

const renderBox = ({
	props,
	chatBuildId,
	chatAgentId = MockWorkspaceAgent.id,
	workspace,
}: {
	props: BoxProps;
	chatBuildId?: string;
	chatAgentId?: string;
	workspace: Workspace;
}) => {
	let buildServer: MockWebSocketServer | undefined;
	let agentServer: MockWebSocketServer | undefined;
	const watchBuildLogs = vi
		.spyOn(apiModule, "watchBuildLogsByBuildId")
		.mockImplementation((_buildId, { onMessage }) => {
			const [socket, server] = createMockWebSocket("ws://test");
			socket.addEventListener("message", (event) =>
				onMessage(JSON.parse(event.data)),
			);
			buildServer = server;
			return socket;
		});
	const watchAgentLogs = vi
		.spyOn(apiModule, "watchWorkspaceAgentLogs")
		.mockImplementation(
			(agentId) =>
				new OneWayWebSocket({
					apiRoute: `/api/v2/workspaceagents/${agentId}/logs`,
					websocketInit: (url, protocol) => {
						const [socket, server] = createMockWebSocket(url, protocol);
						agentServer = server;
						return socket;
					},
				}),
		);
	vi.spyOn(API, "getWorkspace").mockResolvedValue(workspace);

	const queryClient = createTestQueryClient();
	queryClient.setQueryData(workspaceByIdKey(workspace.id), workspace);

	render(
		<QueryClientProvider client={queryClient}>
			<ChatWorkspaceContext
				value={{
					workspaceId: workspace.id,
					buildId: chatBuildId,
					agentId: chatAgentId,
				}}
			>
				<WorkspaceLogBox {...props} />
			</ChatWorkspaceContext>
		</QueryClientProvider>,
	);

	return {
		watchBuildLogs,
		watchAgentLogs,
		buildServer: () => buildServer,
		agentServer: () => agentServer,
	};
};

const currentBuildId = MockWorkspace.latest_build.id;

const agentLogLines = (from: number, count: number): WorkspaceAgentLog[] =>
	Array.from({ length: count }, (_, i) => ({
		...MockWorkspaceAgentLogs[0],
		id: 500000 + from + i,
		created_at: new Date(Date.UTC(2024, 0, 1, 0, 0, from + i)).toISOString(),
		output: `agent line ${from + i}`,
	}));

const publishAgentLogs = (
	server: MockWebSocketServer | undefined,
	logs: WorkspaceAgentLog[],
) => {
	act(() => {
		server?.publishMessage(
			new MessageEvent("message", { data: JSON.stringify(logs) }),
		);
	});
};

describe("WorkspaceLogBox", () => {
	it.each([
		{
			case: "running, bound build is current, agent in build",
			props: { status: "running", action: "start" },
			chatBuildId: currentBuildId,
			workspace: MockWorkspace,
			streams: true,
		},
		{
			case: "running create, bound build is current, agent in build",
			props: { status: "running", action: "create" },
			chatBuildId: currentBuildId,
			workspace: MockWorkspace,
			streams: true,
		},
		{
			case: "running, bound build still starting",
			props: { status: "running", action: "start" },
			chatBuildId: MockStartingWorkspace.latest_build.id,
			chatAgentId: MockWorkspaceAgentStarting.id,
			workspace: MockStartingWorkspace,
			streams: false,
		},
		{
			case: "running, bound agent not in current build",
			props: { status: "running", action: "start" },
			chatBuildId: currentBuildId,
			chatAgentId: "agent-from-a-previous-build",
			workspace: MockWorkspace,
			streams: false,
		},
		{
			case: "running, bound build differs from current build",
			props: { status: "running", action: "start" },
			chatBuildId: "build-not-yet-reflected-in-workspace",
			workspace: MockWorkspace,
			streams: false,
		},
		{
			case: "completed, result build is current",
			props: { status: "completed", action: "start", buildId: currentBuildId },
			workspace: MockWorkspace,
			streams: true,
		},
		{
			case: "completed, result build replaced by the bound build",
			props: {
				status: "completed",
				action: "start",
				buildId: "an-older-build",
			},
			chatBuildId: currentBuildId,
			workspace: MockWorkspace,
			streams: false,
		},
		{
			case: "running stop, bound build is current, agent in build",
			props: { status: "running", action: "stop" },
			chatBuildId: currentBuildId,
			workspace: MockWorkspace,
			streams: false,
		},
		{
			case: "completed stop, result build is current",
			props: { status: "completed", action: "stop", buildId: currentBuildId },
			workspace: MockWorkspace,
			streams: false,
		},
	] satisfies Array<{
		case: string;
		props: BoxProps;
		chatBuildId?: string;
		chatAgentId?: string;
		workspace: Workspace;
		streams: boolean;
	}>)("agent log stream: $case", ({ streams, ...input }) => {
		const { watchAgentLogs } = renderBox(input);

		if (streams) {
			expect(watchAgentLogs).toHaveBeenCalledWith(
				MockWorkspaceAgent.id,
				expect.anything(),
			);
		} else {
			expect(watchAgentLogs).not.toHaveBeenCalled();
		}
	});

	it.each([
		{
			case: "running row streams the bound build",
			props: { status: "running", action: "start", buildId: "result-build" },
			chatBuildId: "bound-build",
			expected: "bound-build",
		},
		{
			case: "finished row streams the result build",
			props: {
				status: "completed",
				action: "start",
				buildId: "result-build",
			},
			chatBuildId: "bound-build",
			expected: "result-build",
		},
		{
			case: "running row without a bound build opens no stream",
			props: { status: "running", action: "stop" },
			expected: undefined,
		},
	] satisfies Array<{
		case: string;
		props: BoxProps;
		chatBuildId?: string;
		expected: string | undefined;
	}>)("build log stream: $case", ({ expected, ...input }) => {
		const { watchBuildLogs } = renderBox({
			...input,
			workspace: MockWorkspace,
		});

		if (expected) {
			expect(watchBuildLogs).toHaveBeenCalledTimes(1);
			expect(watchBuildLogs).toHaveBeenCalledWith(expected, expect.anything());
		} else {
			expect(watchBuildLogs).not.toHaveBeenCalled();
		}
	});

	it("follows new lines only while scrolled to the bottom, without scrollIntoView", () => {
		const scrollIntoView = vi.spyOn(HTMLElement.prototype, "scrollIntoView");
		const scrollHeight = vi.spyOn(HTMLElement.prototype, "scrollHeight", "get");
		vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(100);
		const { buildServer, agentServer } = renderBox({
			props: { status: "running", action: "start" },
			chatBuildId: currentBuildId,
			workspace: MockWorkspace,
		});
		const viewport = screen.getByRole("region", { name: "Workspace log" });

		scrollHeight.mockReturnValue(600);
		act(() => {
			for (const log of MockWorkspaceBuildLogs) {
				buildServer()?.publishMessage(
					new MessageEvent("message", { data: JSON.stringify(log) }),
				);
			}
		});
		expect(viewport.scrollTop).toBe(600);

		scrollHeight.mockReturnValue(1000);
		publishAgentLogs(agentServer(), agentLogLines(0, 10));
		expect(viewport.scrollTop).toBe(1000);

		viewport.scrollTop = 200;
		fireEvent.scroll(viewport);
		scrollHeight.mockReturnValue(1400);
		publishAgentLogs(agentServer(), agentLogLines(10, 10));
		expect(viewport.scrollTop).toBe(200);

		expect(scrollIntoView).not.toHaveBeenCalled();
	});
});
