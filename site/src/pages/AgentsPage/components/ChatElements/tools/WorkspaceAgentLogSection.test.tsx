import { render } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import { workspaceByIdKey } from "#/api/queries/workspaces";
import type { Workspace } from "#/api/typesGenerated";
import {
	MockStartingWorkspace,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceAgentStarting,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	createMockWebSocket,
	type MockWebSocketServer,
} from "#/testHelpers/websockets";
import { OneWayWebSocket } from "#/utils/OneWayWebSocket";
import { ChatWorkspaceContext } from "../../../context/ChatWorkspaceContext";
import { WorkspaceAgentLogSection } from "./WorkspaceAgentLogSection";

afterEach(() => {
	vi.restoreAllMocks();
});

type SectionProps = { status: "running" | "completed"; buildId?: string };

const renderSection = ({
	props,
	chatBuildId,
	chatAgentId = MockWorkspaceAgent.id,
	workspace,
}: {
	props: SectionProps;
	chatBuildId?: string;
	chatAgentId?: string;
	workspace: Workspace;
}) => {
	let server: MockWebSocketServer | undefined;
	const watchAgentLogs = vi
		.spyOn(apiModule, "watchWorkspaceAgentLogs")
		.mockImplementation(
			(agentId) =>
				new OneWayWebSocket({
					apiRoute: `/api/v2/workspaceagents/${agentId}/logs`,
					websocketInit: (url, protocol) => {
						const [socket, mockServer] = createMockWebSocket(url, protocol);
						server = mockServer;
						return socket;
					},
				}),
		);
	vi.spyOn(API, "getWorkspace").mockResolvedValue(workspace);

	const queryClient = createTestQueryClient();
	queryClient.setQueryData(workspaceByIdKey(workspace.id), workspace);

	const ui = (next: SectionProps) => (
		<QueryClientProvider client={queryClient}>
			<ChatWorkspaceContext
				value={{
					workspaceId: workspace.id,
					buildId: chatBuildId,
					agentId: chatAgentId,
				}}
			>
				<WorkspaceAgentLogSection {...next} />
			</ChatWorkspaceContext>
		</QueryClientProvider>
	);
	const { rerender } = render(ui(props));

	return {
		watchAgentLogs,
		rerender: (next: SectionProps) => rerender(ui(next)),
		socketServer: () => server,
	};
};

const currentBuildId = MockWorkspace.latest_build.id;

describe("WorkspaceAgentLogSection", () => {
	it.each([
		{
			case: "running, bound build is current, agent in build",
			props: { status: "running" },
			chatBuildId: currentBuildId,
			workspace: MockWorkspace,
			streams: true,
		},
		{
			case: "running, bound build still starting",
			props: { status: "running" },
			chatBuildId: MockStartingWorkspace.latest_build.id,
			chatAgentId: MockWorkspaceAgentStarting.id,
			workspace: MockStartingWorkspace,
			streams: false,
		},
		{
			case: "running, bound agent not in current build",
			props: { status: "running" },
			chatBuildId: currentBuildId,
			chatAgentId: "agent-from-a-previous-build",
			workspace: MockWorkspace,
			streams: false,
		},
		{
			case: "running, bound build differs from current build",
			props: { status: "running" },
			chatBuildId: "build-not-yet-reflected-in-workspace",
			workspace: MockWorkspace,
			streams: false,
		},
		{
			case: "completed, result build is current",
			props: { status: "completed", buildId: currentBuildId },
			workspace: MockWorkspace,
			streams: true,
		},
		{
			case: "completed, result build replaced by the bound build",
			props: { status: "completed", buildId: "an-older-build" },
			chatBuildId: currentBuildId,
			workspace: MockWorkspace,
			streams: false,
		},
	] satisfies Array<{
		case: string;
		props: SectionProps;
		chatBuildId?: string;
		chatAgentId?: string;
		workspace: Workspace;
		streams: boolean;
	}>)("$case", ({ streams, ...input }) => {
		const { watchAgentLogs } = renderSection(input);

		if (streams) {
			expect(watchAgentLogs).toHaveBeenCalledWith(
				MockWorkspaceAgent.id,
				expect.anything(),
			);
		} else {
			expect(watchAgentLogs).not.toHaveBeenCalled();
		}
	});

	it("keeps streaming agent logs when the call completes", () => {
		const { watchAgentLogs, rerender, socketServer } = renderSection({
			props: { status: "running" },
			chatBuildId: currentBuildId,
			workspace: MockWorkspace,
		});

		rerender({ status: "completed", buildId: currentBuildId });

		expect(watchAgentLogs).toHaveBeenCalledTimes(1);
		expect(socketServer()?.isConnectionOpen).toBe(true);
	});
});
